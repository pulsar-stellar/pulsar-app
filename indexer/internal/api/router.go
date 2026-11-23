package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/pulsar-stellar/pulsar-app/indexer/internal/apierror"
)

// Routes builds the HTTP handler for the read API: a chi router carrying the
// business routes, a not-found handler so an unmatched route still answers with
// the ADR-017 envelope rather than chi's bare 404, and a method-not-allowed
// handler so a known path hit with the wrong method answers the same way, all
// wrapped by the surface-wide middleware chain.
func (s *Server) Routes() http.Handler {
	r := s.router()

	// The surface-wide chain wraps the whole mux, not chi's Use stack: chi skips
	// its Use middleware for the not-found path until a route is registered, so
	// wrapping the mux is what applies these to every request, matched or not.
	// RequestLogger is outermost so the status a recovered panic produces is the
	// status it logs. SecurityHeaders rides on every response; ReadDeadline bounds
	// the time any request may spend downstream (a no-op when unset); Recoverer
	// sits innermost so a handler panic becomes an envelope before the outer
	// layers see it.
	return RequestLogger(s.log)(SecurityHeaders(ReadDeadline(s.readTimeout)(Recoverer(s.log)(r))))
}

// router builds the chi mux with every route registered and no surface-wide
// middleware, so tests can walk the registered routes (chi.Walk).
func (s *Server) router() *chi.Mux {
	r := chi.NewRouter()

	r.NotFound(s.handleNotFound)
	r.MethodNotAllowed(s.handleMethodNotAllowed)

	// HEAD on a GET route runs the GET handler, so uptime monitors that probe
	// with HEAD see the real status (ADR-057, #41). net/http drops the body of a
	// HEAD response. A route with no GET, such as POST /graphql, still answers
	// HEAD with 404 NOT_FOUND_METHOD. chi requires Use before any route.
	r.Use(middleware.GetHead)

	r.Get("/health", s.handleHealth)

	// The write routes sit behind the auth gate, and behind the rate limiter
	// when one is configured (ADR-044). RateLimit is listed before RequireAuth so
	// it wraps it: an unauthenticated flood is capped before the auth comparison
	// runs. writeMiddleware collects the guards so both write routes carry the
	// same stack.
	writeMiddleware := make([]func(http.Handler) http.Handler, 0, 2)
	if s.writeLimiter != nil {
		writeMiddleware = append(writeMiddleware, RateLimit(s.writeLimiter, "write"))
	}
	writeMiddleware = append(writeMiddleware, RequireAuth(s.authToken))

	// The public reads and /graphql spend from their own bucket when one is
	// configured (ADR-055), separate from the writes' so a read flood cannot
	// lock the operator out. /health above is deliberately outside it, so a
	// spent bucket never fails the platform's health check.
	var readMiddleware []func(http.Handler) http.Handler
	if s.readLimiter != nil {
		readMiddleware = append(readMiddleware, RateLimit(s.readLimiter, "read"))
	}

	// The contract routes are registered flat rather than under a chi subrouter
	// so /contracts matches the SDK's paths exactly, with no trailing slash and
	// no redirect. List and get are public reads; register and delete are the
	// write surface and carry the auth (and rate-limit) middleware inline, so the
	// GET on the same path stays open.
	// The root index is a public read, so it spends from the read bucket like
	// the routes it lists (ADR-055, ADR-057).
	r.With(readMiddleware...).Get("/", s.handleRoot)

	r.With(readMiddleware...).Get("/contracts", s.handleListContracts)
	r.With(writeMiddleware...).Post("/contracts", s.handleRegisterContract)
	r.With(readMiddleware...).Get("/contracts/{id}", s.handleGetContract)
	r.With(writeMiddleware...).Delete("/contracts/{id}", s.handleDeleteContract)

	// The events routes are registered flat for the same reason as the contract
	// routes: the paths match the SDK's exactly. The list route is nested under a
	// contract; the single-event route is top-level, keyed by the event's own id.
	r.With(readMiddleware...).Get("/contracts/{id}/events", s.handleListEvents)
	r.With(readMiddleware...).Get("/events/{id}", s.handleGetEvent)

	// The GraphQL read surface is a single POST endpoint over the same data, with
	// nesting the REST routes cannot express (ADR-043). It is registered after the
	// REST routes so it too is wrapped by the middleware below, inheriting request
	// logging, panic recovery, and the request id.
	r.With(readMiddleware...).Post("/graphql", s.handleGraphQL)

	return r
}

// handleNotFound answers a request that matched no route with a 404 not_found
// envelope carrying the NOT_FOUND_ROUTE catalog code.
func (s *Server) handleNotFound(w http.ResponseWriter, _ *http.Request) {
	writeError(w, apierror.New(apierror.CodeNotFoundRoute, "No endpoint matches the request path."))
}
