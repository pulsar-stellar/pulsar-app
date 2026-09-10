package api

import (
	"log/slog"

	"github.com/pulsar-stellar/pulsar-app/indexer/internal/logger"
)

// Server holds the dependencies the HTTP handlers share and is the composition
// root for the read API. It gains a field as each handler that needs one lands.
type Server struct {
	// log is already scoped to the api component. Middleware and handlers log
	// through it and must not attach a component of their own, or the line
	// carries the component twice.
	log *slog.Logger

	// contracts backs the health endpoint's status read. It is the narrow
	// contractStatsReader rather than the whole store, so a handler test can
	// substitute a fake and the api package depends on the store only through
	// the slice of it that it uses.
	contracts contractStatsReader

	// version is the build identifier /health reports. NewServer guarantees it
	// is non-empty, since the SDK's health schema rejects an empty string.
	version string
}

// NewServer builds a Server. log is the base logger; NewServer scopes it to the
// api component once, per the logger package's contract. A nil logger becomes a
// discarding one so a handler never nil-panics, matching the poller's
// constructor. An empty version becomes "dev", so an unstamped build still
// reports a value the health schema accepts.
func NewServer(log *slog.Logger, contracts contractStatsReader, version string) *Server {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	if version == "" {
		version = "dev"
	}
	return &Server{
		log:       logger.Component(log, logger.ComponentAPI),
		contracts: contracts,
		version:   version,
	}
}
