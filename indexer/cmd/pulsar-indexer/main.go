// Command pulsar-indexer runs the Pulsar event indexer daemon.
//
// Startup is a fixed pipeline, each stage gated on the last: load and validate
// the environment, build the logger, resolve and open the database, apply
// migrations, register the bootstrap contracts, and launch one polling
// goroutine per contract. A failure in any stage before the poll loops start
// is fatal and names the stage, because a daemon that came up half-configured
// is worse than one that refused to start.
//
// Shutdown is signal-driven: SIGINT or SIGTERM cancels the root context, the
// HTTP server drains in flight requests, every poll loop returns ctx.Err(), and
// main waits for all of them before exiting so no goroutine is torn down
// mid-transaction.
//
// The HTTP surface is wired alongside the poller (step 72): the read API and
// the guarded write routes (ADR-044) serve on cfg.ListenAddr while the poll
// loops run, and both stop on the same signal. See .agent/context.md.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/pulsar-stellar/pulsar-app/indexer/internal/api"
	"github.com/pulsar-stellar/pulsar-app/indexer/internal/config"
	"github.com/pulsar-stellar/pulsar-app/indexer/internal/db"
	"github.com/pulsar-stellar/pulsar-app/indexer/internal/decoder"
	"github.com/pulsar-stellar/pulsar-app/indexer/internal/logger"
	"github.com/pulsar-stellar/pulsar-app/indexer/internal/rpc"
	"github.com/pulsar-stellar/pulsar-app/indexer/internal/store"
	"github.com/pulsar-stellar/pulsar-app/indexer/internal/version"
)

// HTTP transport timeouts, distinct from the surface-wide read deadline the API
// applies per request (ADR-044): these bound the connection itself.
// readHeaderTimeout caps the time a client may take to send request headers, so
// a slow-loris client cannot pin a connection open (gosec G112). idleTimeout
// bounds how long a kept-alive connection may sit unused. shutdownGrace bounds
// the graceful drain before in flight requests are cut.
const (
	readHeaderTimeout = 10 * time.Second
	idleTimeout       = 120 * time.Second
	shutdownGrace     = 10 * time.Second
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "pulsar-indexer: %v\n", err)
		os.Exit(1)
	}
}

// run holds the startup pipeline and blocks until a signal cancels it. It is
// separate from main so every failure path returns an error rather than calling
// os.Exit, which keeps the stages testable and the deferred cleanup honoured.
func run() error {
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return fmt.Errorf("loading configuration: %w", err)
	}

	log, err := logger.New(os.Stderr, logger.Options{Level: cfg.LogLevel, Format: cfg.LogFormat})
	if err != nil {
		return fmt.Errorf("building logger: %w", err)
	}

	driver, err := db.Resolve(db.Options{
		DriverName: cfg.DBDriver,
		PoolMax:    cfg.DBPoolMax,
		PoolMin:    cfg.DBPoolMin,
	})
	if err != nil {
		return fmt.Errorf("resolving database driver: %w", err)
	}

	dialect, err := dialectFor(driver.Kind)
	if err != nil {
		return fmt.Errorf("resolving store dialect: %w", err)
	}

	handle, err := db.Open(driver, db.ConnOptions{
		DSN:              cfg.DBURL,
		AllowInsecureTLS: cfg.DBAllowInsecureTLS,
	})
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}
	defer func() { _ = handle.Close() }()

	// The root context is cancelled by the first SIGINT or SIGTERM. Migrations
	// and the bootstrap registration run under it too, so a signal during
	// startup aborts cleanly rather than being ignored until the poll loops
	// begin. cancel is derived on top so a fatal serve error can tear the poll
	// loops down as well, since stop alone unregisters the signal without
	// cancelling the context.
	sigCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithCancel(sigCtx)
	defer cancel()

	applied, err := db.Up(ctx, handle, driver)
	if err != nil {
		return fmt.Errorf("applying migrations: %w", err)
	}
	log.Info("startup",
		"component", "main",
		"db_driver", cfg.DBDriver,
		"network", cfg.Network,
		"migrations_applied", len(applied),
		"bootstrap_contracts", len(cfg.BootstrapContracts))

	contracts := store.NewContracts(handle)
	for _, id := range cfg.BootstrapContracts {
		if _, err := contracts.Register(ctx, id); err != nil {
			return fmt.Errorf("registering bootstrap contract %s: %w", id, err)
		}
		log.Info("contract_registered", "component", "main", "contract_id", id)
	}

	client := rpc.NewClient(cfg.RPCURL, http.DefaultClient)
	defer func() { _ = client.Close() }()

	poller := rpc.NewPoller(
		client,
		decoder.New(),
		handle,
		contracts,
		dialect,
		cfg.PollInterval,
		cfg.BatchSize,
		log,
	)

	// The read API and the guarded write routes serve over the same stores the
	// poller writes through. The write surface carries auth and rate limiting,
	// and the whole surface a read deadline (ADR-044); the config layer has
	// already validated the token length and the positive rate bounds.
	events := store.NewEvents(handle, dialect)
	srv := api.NewServer(log, contracts, events, version.Version,
		api.WithAuthToken(cfg.AdminToken),
		api.WithWriteRateLimit(cfg.WriteRatePerSec, cfg.WriteRateBurst),
		api.WithReadTimeout(cfg.ReadTimeout),
	)
	httpServer := newHTTPServer(cfg, srv.Routes())

	// Bind the listener before launching any goroutine so a port already in use
	// is a fatal startup error naming the stage, not a failure buried in a
	// background loop after the poll loops have started.
	listener, err := net.Listen("tcp", cfg.ListenAddr)
	if err != nil {
		return fmt.Errorf("listening on %s: %w", cfg.ListenAddr, err)
	}

	// One goroutine per contract. Each Run blocks until ctx is cancelled and
	// then returns ctx.Err(); a non-cancellation return would signal a bug, so
	// it is logged rather than swallowed. The WaitGroup lets main hold the
	// process open until every loop has stopped.
	var wg sync.WaitGroup
	for _, id := range cfg.BootstrapContracts {
		wg.Add(1)
		go func(contractID string) {
			defer wg.Done()
			if err := poller.Run(ctx, contractID); err != nil &&
				!errorIsCancellation(err) {
				log.Error("poller_stopped",
					"component", "main",
					"contract_id", contractID,
					"err", err)
			}
		}(id)
	}

	if len(cfg.BootstrapContracts) == 0 {
		log.Warn("no_bootstrap_contracts",
			"component", "main",
			"detail", "PULSAR_INDEXER_BOOTSTRAP_CONTRACTS is empty; the indexer will idle until a contract is registered")
	}

	log.Info("running",
		"component", "main",
		"poll_interval", cfg.PollInterval.String(),
		"listen_addr", cfg.ListenAddr)

	// serveUntilShutdown blocks until ctx is cancelled or the server fails to
	// serve. A serve failure is fatal: it cancels the poll loops through the
	// shared context on the way out, so the daemon does not linger with a dead
	// HTTP surface. A clean signal-driven stop returns nil.
	serveErr := serveUntilShutdown(ctx, httpServer, listener, log)

	log.Info("shutdown_signal", "component", "main", "detail", "cancelling poll loops and waiting for them to stop")
	cancel()
	wg.Wait()
	log.Info("stopped", "component", "main")
	return serveErr
}

// newHTTPServer builds the daemon's HTTP server from the config and the API
// handler. The transport timeouts are fixed here rather than configured: they
// guard the connection (a positive ReadHeaderTimeout clears gosec G112), while
// the per-request read deadline the handler already applies is the tunable one.
func newHTTPServer(cfg config.Config, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           handler,
		ReadHeaderTimeout: readHeaderTimeout,
		IdleTimeout:       idleTimeout,
	}
}

// serveUntilShutdown serves on ln until ctx is cancelled, then drains in flight
// requests within shutdownGrace and returns nil. A serve failure other than the
// expected ErrServerClosed is returned so the caller can treat it as fatal. The
// listener is passed in already bound so a port conflict surfaces at startup
// rather than here.
func serveUntilShutdown(ctx context.Context, srv *http.Server, ln net.Listener, log *slog.Logger) error {
	serveErrs := make(chan error, 1)
	go func() {
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErrs <- fmt.Errorf("serving http: %w", err)
			return
		}
		serveErrs <- nil
	}()

	select {
	case err := <-serveErrs:
		return err
	case <-ctx.Done():
		log.Info("http_shutdown", "component", "main", "detail", "draining in flight requests")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownGrace)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shutting down http server: %w", err)
		}
		return nil
	}
}

// errorIsCancellation reports whether err is the clean shutdown signal, so the
// poller-stopped log fires only on an unexpected exit.
func errorIsCancellation(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

// dialectFor maps a resolved driver to the store dialect that the events store
// needs for the one query the engines spell differently. The switch is
// exhaustive over the supported kinds; an unknown kind is a programmer error,
// since Resolve already rejected every other name, so it fails closed rather
// than defaulting to an engine.
func dialectFor(kind db.Kind) (store.Dialect, error) {
	switch kind {
	case db.KindSQLite:
		return store.DialectSQLite, nil
	case db.KindPostgres:
		return store.DialectPostgres, nil
	default:
		return 0, fmt.Errorf("no store dialect for driver %q", kind)
	}
}
