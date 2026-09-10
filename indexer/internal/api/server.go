package api

import (
	"log/slog"

	"github.com/pulsar-stellar/pulsar-app/indexer/internal/logger"
)

// Server holds the dependencies the HTTP handlers share and is the composition
// root for the read API. It gains a field as each handler that needs one lands;
// step 63 wires the router and its middleware, so a logger is all it needs yet.
type Server struct {
	// log is already scoped to the api component. Middleware and handlers log
	// through it and must not attach a component of their own, or the line
	// carries the component twice.
	log *slog.Logger
}

// NewServer builds a Server. log is the base logger; NewServer scopes it to the
// api component once, per the logger package's contract. A nil logger becomes a
// discarding one so a handler never nil-panics, matching the poller's
// constructor.
func NewServer(log *slog.Logger) *Server {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	return &Server{log: logger.Component(log, logger.ComponentAPI)}
}
