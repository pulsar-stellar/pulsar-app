package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/pulsar-stellar/pulsar-app/indexer/internal/apierror"
)

// contextKey is unexported so no other package can collide with the keys this
// package stores on a request context.
type contextKey int

const requestIDKey contextKey = iota

// RequestIDFromContext returns the request id RequestLogger attached to ctx, or
// the empty string when ctx carries none (a handler exercised without the
// middleware, say). A handler uses it to stamp its own log lines with the same
// request_id as the request and any panic, so one request's lines join up.
func RequestIDFromContext(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey).(string)
	return id
}

// newRequestID returns a fresh 128-bit hex id. The id is server-generated and
// never read from an inbound header: a client-supplied value would be
// untrusted input on a field that lands in every log line, so accepting one
// would open a log-injection and correlation-forgery path for no gain here.
func newRequestID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand does not fail on a supported platform; if it somehow
		// does, fall back to a time-based id so the request stays traceable
		// and the request path never panics.
		return fmt.Sprintf("req-%x", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}

// responseRecorder wraps a ResponseWriter to capture the status code for the
// request log and to record whether the response has begun. The recoverer
// reads wroteHeader so it does not write a second header over one a handler
// already sent; WriteHeader itself guards the double call.
type responseRecorder struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (r *responseRecorder) WriteHeader(status int) {
	if r.wroteHeader {
		return
	}
	r.status = status
	r.wroteHeader = true
	r.ResponseWriter.WriteHeader(status)
}

func (r *responseRecorder) Write(b []byte) (int, error) {
	if !r.wroteHeader {
		r.WriteHeader(http.StatusOK)
	}
	return r.ResponseWriter.Write(b)
}

// RequestLogger returns middleware that assigns each request an id, echoes it
// in the X-Request-Id response header, and logs one line per completed request
// with the section 7.6 request-scoped fields: request_id, method, path, status,
// and duration_ms. log must already be scoped to the api component. Mount it
// outside Recoverer so a status a recovered panic produced is the one logged.
func RequestLogger(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			id := newRequestID()

			r = r.WithContext(context.WithValue(r.Context(), requestIDKey, id))
			w.Header().Set("X-Request-Id", id)

			rec := &responseRecorder{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(rec, r)

			log.InfoContext(r.Context(), "http_request",
				"request_id", id,
				"method", r.Method,
				"path", r.URL.Path,
				"status", rec.status,
				"duration_ms", float64(time.Since(start).Microseconds())/1000.0,
			)
		})
	}
}

// Recoverer returns middleware that turns a panic in a downstream handler into
// a logged event and a clean 500. It logs a "panic" line with the request
// fields, the recovered value, and a stack trace, then returns the ADR-017
// error envelope with class internal and catalog code INTERNAL_PANIC. The
// recovered value is logged server-side only; the client sees a fixed generic
// message, never the panic's contents. log must already be scoped to the api
// component. Mount it inside RequestLogger.
func Recoverer(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				v := recover()
				if v == nil {
					return
				}
				// http uses ErrAbortHandler to abort a response on purpose and
				// suppresses it; re-panic so that contract is preserved.
				if v == http.ErrAbortHandler {
					panic(v)
				}

				log.ErrorContext(r.Context(), "panic",
					"request_id", RequestIDFromContext(r.Context()),
					"method", r.Method,
					"path", r.URL.Path,
					"panic", fmt.Sprintf("%v", v),
					"stack", string(debug.Stack()),
				)

				// If the handler already began the response, its bytes are on
				// the wire and a fresh envelope cannot cleanly replace them.
				if rec, ok := w.(*responseRecorder); ok && rec.wroteHeader {
					return
				}
				writeError(w, apierror.New(apierror.CodeInternalPanic, "Internal server error"))
			}()

			next.ServeHTTP(w, r)
		})
	}
}
