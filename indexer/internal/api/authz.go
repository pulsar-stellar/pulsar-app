package api

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"net/http"
	"strings"
	"time"

	"golang.org/x/time/rate"

	"github.com/pulsar-stellar/pulsar-app/indexer/internal/apierror"
)

// RequireAuth returns middleware that gates a route behind a static bearer
// token (ADR-044). It reads Authorization: Bearer <t> and admits the request
// only when the presented token equals the configured one. The comparison
// hashes both sides with SHA-256 first and then runs subtle.ConstantTimeCompare,
// so neither the token's bytes nor its length leak through a timing side
// channel. A missing header, a non-bearer scheme, and a wrong token all fail
// the same way: a 401 UNAUTHORIZED envelope with a WWW-Authenticate challenge
// and one fixed message, so the response is not a credential oracle. An empty
// configured token fails closed (every request is refused), so a deployment
// that forgot to set the token cannot expose open writes. The token is never
// logged or echoed.
func RequireAuth(token string) func(http.Handler) http.Handler {
	expected := sha256.Sum256([]byte(token))
	configured := token != ""

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			provided, ok := bearerToken(r.Header.Get("Authorization"))
			presented := sha256.Sum256([]byte(provided))

			// Both comparisons run regardless of outcome so a caller cannot
			// distinguish "no token configured" from "wrong token" by timing.
			match := subtle.ConstantTimeCompare(presented[:], expected[:]) == 1
			if !configured || !ok || !match {
				w.Header().Set("WWW-Authenticate", "Bearer")
				writeError(w, apierror.New(apierror.CodeUnauthorized, "This route requires a valid bearer token."))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// bearerToken extracts the credential from an Authorization header value. It
// returns the token and true only for a well-formed "Bearer <token>" with a
// non-empty token; the scheme match is case-insensitive per RFC 7235. Anything
// else returns false, which the caller treats as an auth failure.
func bearerToken(header string) (string, bool) {
	const prefix = "bearer "
	if len(header) <= len(prefix) || !strings.EqualFold(header[:len(prefix)], prefix) {
		return "", false
	}
	token := strings.TrimSpace(header[len(prefix):])
	if token == "" {
		return "", false
	}
	return token, true
}

// RateLimit returns middleware that admits a request only when the shared token
// bucket has a token to spend, and otherwise refuses it with a 429 RATE_LIMITED
// envelope (ADR-044). The limiter is global across the routes it wraps rather
// than per-client: the write surface has a single operator, so a global bucket
// bounds the write rate without the unbounded per-IP map that would itself be a
// memory-exhaustion vector. It is mounted outside RequireAuth so an
// unauthenticated flood is capped before the auth comparison runs.
func RateLimit(limiter *rate.Limiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !limiter.Allow() {
				writeError(w, apierror.New(apierror.CodeRateLimited, "Too many write requests; retry after a short wait."))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// ReadDeadline returns middleware that bounds the wall-clock time a request may
// spend downstream by deriving a context.WithTimeout(d) from the request
// context and cancelling it on return (ADR-044). It is a distinct guard from
// the GraphQL query-shape limits (MaxDepth, MaxParallelism, MaxQueryLength),
// which cap a query's structure but not the time a shallow query can spend in a
// slow store or RPC read; the deadline bounds that time for any request, REST or
// GraphQL. A handler that overruns sees a cancelled context and surfaces its own
// store-error envelope (500 INTERNAL_STORE, cause logged server-side), which
// keeps the response inside ADR-017's fixed status set rather than adding a 504.
// A non-positive duration disables the guard, so a Server built without a
// timeout (the four-argument NewServer) is unaffected.
func ReadDeadline(d time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if d <= 0 {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, cancel := context.WithTimeout(r.Context(), d)
			defer cancel()
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// SecurityHeaders returns middleware that sets X-Content-Type-Options: nosniff
// on every response (ADR-043 L2, mounted surface-wide by ADR-044), so a browser
// never MIME-sniffs a JSON body into something executable. It sets the header
// before the handler runs so it rides on every response, success or error.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		next.ServeHTTP(w, r)
	})
}
