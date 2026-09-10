package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/pulsar-stellar/pulsar-app/indexer/internal/apierror"
)

// Routes builds the HTTP handler for the read API: a chi router carrying the
// business routes, a not-found handler so an unmatched route still answers with
// the ADR-017 envelope rather than chi's bare 404, and a method-not-allowed
// handler so a known path hit with the wrong method answers the same way, all
// wrapped by the request-logging and panic-recovery middleware.
func (s *Server) Routes() http.Handler {
	r := chi.NewRouter()

	r.NotFound(s.handleNotFound)
	r.MethodNotAllowed(s.handleMethodNotAllowed)

	r.Get("/health", s.handleHealth)

	// RequestLogger and Recoverer wrap the whole mux, not chi's Use stack: chi
	// skips its Use middleware for the not-found path until a route is
	// registered, so wrapping the mux is what applies them to every request,
	// matched or not. RequestLogger is outermost so the status a recovered panic
	// produces is the status it logs.
	return RequestLogger(s.log)(Recoverer(s.log)(r))
}

// handleNotFound answers a request that matched no route with a 404 not_found
// envelope carrying the NOT_FOUND_ROUTE catalog code.
func (s *Server) handleNotFound(w http.ResponseWriter, _ *http.Request) {
	writeError(w, apierror.New(apierror.CodeNotFoundRoute, "No endpoint matches the request path."))
}
