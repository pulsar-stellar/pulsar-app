// Command pulsar-indexer runs the Pulsar event indexer daemon.
//
// Startup is a fixed pipeline, each stage gated on the last: load and validate
// the environment, build the logger, resolve and open the database, apply
// migrations, register the bootstrap contracts, and launch one polling
// goroutine per contract. A failure in any stage before the poll loops start
// is fatal and names the stage, because a daemon that came up half-configured
// is worse than one that refused to start.
//
// Shutdown is signal-driven: SIGINT or SIGTERM cancels the root context, every
// poll loop returns ctx.Err(), and main waits for all of them before exiting so
// no goroutine is torn down mid-transaction.
//
// The HTTP surface is not wired here yet; this slice of the startup sequence
// (step 72) brings up the poller only, and the API server follows in a later
// block. See .agent/context.md.
package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/pulsar-stellar/pulsar-app/indexer/internal/config"
	"github.com/pulsar-stellar/pulsar-app/indexer/internal/db"
	"github.com/pulsar-stellar/pulsar-app/indexer/internal/decoder"
	"github.com/pulsar-stellar/pulsar-app/indexer/internal/logger"
	"github.com/pulsar-stellar/pulsar-app/indexer/internal/rpc"
	"github.com/pulsar-stellar/pulsar-app/indexer/internal/store"
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
	// begin.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

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
		cfg.PollInterval,
		cfg.BatchSize,
		log,
	)

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

	log.Info("running", "component", "main", "poll_interval", cfg.PollInterval.String())

	<-ctx.Done()
	log.Info("shutdown_signal", "component", "main", "detail", "cancelling poll loops and waiting for them to stop")
	wg.Wait()
	log.Info("stopped", "component", "main")
	return nil
}

// errorIsCancellation reports whether err is the clean shutdown signal, so the
// poller-stopped log fires only on an unexpected exit.
func errorIsCancellation(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}
