package api

import (
	"log/slog"

	graphql "github.com/graph-gophers/graphql-go"

	"github.com/pulsar-stellar/pulsar-app/indexer/internal/logger"
)

// Server holds the dependencies the HTTP handlers share and is the composition
// root for the read API. It gains a field as each handler that needs one lands.
type Server struct {
	// log is already scoped to the api component. Middleware and handlers log
	// through it and must not attach a component of their own, or the line
	// carries the component twice.
	log *slog.Logger

	// contracts backs the contract routes and the health status read. It is the
	// contractStore interface rather than the whole store, so a handler test can
	// substitute a fake and the api package depends on the store only through
	// the methods its handlers call.
	contracts contractStore

	// events backs the events read routes. Like contracts it is an interface
	// defined at its point of use, so a handler test substitutes a fake and the
	// package depends on the events store only through Query and Get.
	events eventStore

	// version is the build identifier /health reports. NewServer guarantees it
	// is non-empty, since the SDK's health schema rejects an empty string.
	version string

	// schema is the parsed GraphQL schema the /graphql handler executes. It is
	// built once in NewServer, closing over this Server so its resolvers reach
	// the same stores as the REST handlers, and is safe for concurrent use.
	schema *graphql.Schema
}

// NewServer builds a Server. log is the base logger; NewServer scopes it to the
// api component once, per the logger package's contract. A nil logger becomes a
// discarding one so a handler never nil-panics, matching the poller's
// constructor. An empty version becomes "dev", so an unstamped build still
// reports a value the health schema accepts.
func NewServer(log *slog.Logger, contracts contractStore, events eventStore, version string) *Server {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	if version == "" {
		version = "dev"
	}
	s := &Server{
		log:       logger.Component(log, logger.ComponentAPI),
		contracts: contracts,
		events:    events,
		version:   version,
	}
	// The GraphQL schema closes over the fully built Server, so it is parsed
	// after the struct is assembled rather than in the literal above.
	s.schema = newGraphQLSchema(s)
	return s
}
