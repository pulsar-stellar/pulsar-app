package main

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/pulsar-stellar/pulsar-app/indexer/internal/config"
)

// discardLogger is a slog logger that throws its output away, so a test can
// exercise the serve loop without noise on stderr.
func discardLogger() *slog.Logger { return slog.New(slog.DiscardHandler) }

// newHTTPServer must set ReadHeaderTimeout to a positive value so a slow-loris
// client cannot hold a connection open sending headers forever (gosec G112),
// and must carry the listen address and handler it was given.
func TestNewHTTPServerSetsSlowlorisGuardAndWiring(t *testing.T) {
	t.Parallel()

	reached := false
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reached = true
		w.WriteHeader(http.StatusOK)
	})

	srv := newHTTPServer(config.Config{ListenAddr: ":9999"}, handler)

	if srv.ReadHeaderTimeout <= 0 {
		t.Errorf("ReadHeaderTimeout = %v, want positive (gosec G112)", srv.ReadHeaderTimeout)
	}
	if srv.IdleTimeout <= 0 {
		t.Errorf("IdleTimeout = %v, want positive", srv.IdleTimeout)
	}
	if srv.Addr != ":9999" {
		t.Errorf("Addr = %q, want :9999", srv.Addr)
	}

	rr := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))
	if !reached {
		t.Error("the configured handler did not run")
	}
}

// serveUntilShutdown runs the server until its context is cancelled, then
// returns nil after a graceful shutdown. This is the loop run() uses to hold
// the process open alongside the poll loops, so a cancelled context must stop
// the listener and return without error.
func TestServeUntilShutdownReturnsOnContextCancel(t *testing.T) {
	t.Parallel()

	srv := newHTTPServer(config.Config{ListenAddr: "127.0.0.1:0"}, http.NewServeMux())
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- serveUntilShutdown(ctx, srv, ln, discardLogger()) }()

	// Give the server a moment to start accepting, then cancel.
	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("serveUntilShutdown returned %v, want nil on clean shutdown", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("serveUntilShutdown did not return after context cancel")
	}
}

// A listener bind failure surfaces as an error rather than a silent no-op, so a
// port already in use fails the daemon loudly instead of leaving it up with no
// HTTP surface.
func TestServeUntilShutdownReportsAServeError(t *testing.T) {
	t.Parallel()

	srv := newHTTPServer(config.Config{}, http.NewServeMux())
	// A closed listener makes Serve return immediately with an error that is
	// not http.ErrServerClosed.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	_ = ln.Close()

	done := make(chan error, 1)
	go func() { done <- serveUntilShutdown(context.Background(), srv, ln, discardLogger()) }()

	select {
	case err := <-done:
		if err == nil {
			t.Error("serveUntilShutdown returned nil, want the serve error from a closed listener")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("serveUntilShutdown did not return on a serve error")
	}
}
