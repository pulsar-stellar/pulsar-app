package api

import (
	"log/slog"
	"time"

	graphql "github.com/graph-gophers/graphql-go"
	"golang.org/x/time/rate"

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

	// authToken is the bearer token the write routes require (ADR-044). It is
	// set by WithAuthToken. Empty means no token was configured, and RequireAuth
	// then fails closed so the write surface is never left open by omission.
	authToken string

	// writeLimiter caps the write routes when set by WithWriteRateLimit. A nil
	// limiter means no rate limit is mounted, which is the read-only default a
	// four-argument NewServer produces.
	writeLimiter *rate.Limiter

	// readTimeout bounds the wall-clock time any request may spend downstream
	// when set by WithReadTimeout. A non-positive value disables the guard, so a
	// four-argument NewServer applies none.
	readTimeout time.Duration
}

// Option configures a Server at construction. Options keep NewServer's required
// arguments to the dependencies every deployment needs, while the write-surface
// guards (ADR-044) are opt-in, so a read-only caller and a handler test build a
// Server without them.
type Option func(*Server)

// WithAuthToken sets the bearer token the write routes require (ADR-044). A
// deployment passes the validated config value; a Server built without it
// refuses every write, since RequireAuth fails closed on an empty token.
func WithAuthToken(token string) Option {
	return func(s *Server) { s.authToken = token }
}

// WithWriteRateLimit mounts a shared token bucket over the write routes sized
// at perSec sustained requests with a burst ceiling (ADR-044). The bucket is
// global rather than per-client, matching the single-operator write surface. A
// non-positive perSec or burst leaves the limiter unset, so misconfiguration
// does not silently wedge writes shut; the config layer validates both as
// positive before they reach here.
func WithWriteRateLimit(perSec, burst int) Option {
	return func(s *Server) {
		if perSec <= 0 || burst <= 0 {
			return
		}
		s.writeLimiter = rate.NewLimiter(rate.Limit(perSec), burst)
	}
}

// WithReadTimeout bounds the wall-clock time any request may spend downstream,
// applied surface-wide (ADR-044). It is a distinct guard from the GraphQL
// query-shape limits: it bounds time in the store or RPC for any request. A
// non-positive duration disables it.
func WithReadTimeout(d time.Duration) Option {
	return func(s *Server) { s.readTimeout = d }
}

// NewServer builds a Server. log is the base logger; NewServer scopes it to the
// api component once, per the logger package's contract. A nil logger becomes a
// discarding one so a handler never nil-panics, matching the poller's
// constructor. An empty version becomes "dev", so an unstamped build still
// reports a value the health schema accepts. The variadic options carry the
// write-surface guards (ADR-044); a caller that passes none gets the read-only
// surface unchanged.
func NewServer(log *slog.Logger, contracts contractStore, events eventStore, version string, opts ...Option) *Server {
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
	for _, opt := range opts {
		opt(s)
	}
	// The GraphQL schema closes over the fully built Server, so it is parsed
	// after the struct is assembled rather than in the literal above.
	s.schema = newGraphQLSchema(s)
	return s
}
