package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/pulsar-stellar/pulsar-app/indexer/internal/apierror"
)

// A write route admits a request that carries the configured bearer token: the
// gate is transparent to a caller that presents the right credential, so the
// handler runs and the store sees the call.
func TestRequireAuthAdmitsTheConfiguredToken(t *testing.T) {
	t.Parallel()

	reached := false
	srv := NewServer(nil, fakeContracts{deleteFn: func(context.Context, string) error {
		reached = true
		return nil
	}}, fakeEvents{}, "1.2.3", WithAuthToken(testAuthToken))

	rr := serveAuthed(srv, http.MethodDelete, "/contracts/"+validContractID, "")

	if rr.Code != http.StatusNoContent {
		t.Errorf("status = %d, want 204 for a correctly authed write", rr.Code)
	}
	if !reached {
		t.Error("the handler did not run for a correctly authed write")
	}
}

// A write route with no Authorization header is refused with the 401
// UNAUTHORIZED envelope, a WWW-Authenticate challenge, and never reaches the
// store. This is the base case ADR-044's gate exists for.
func TestRequireAuthRejectsAMissingHeader(t *testing.T) {
	t.Parallel()

	srv := NewServer(nil, fakeContracts{deleteFn: func(context.Context, string) error {
		t.Error("Delete reached the store despite a missing bearer token")
		return nil
	}}, fakeEvents{}, "1.2.3", WithAuthToken(testAuthToken))

	rr := serve(srv, http.MethodDelete, "/contracts/"+validContractID, "")

	assertUnauthorized(t, rr)
	if got := rr.Header().Get("WWW-Authenticate"); got != "Bearer" {
		t.Errorf("WWW-Authenticate = %q, want Bearer", got)
	}
}

// A wrong token fails exactly as a missing one does, so the response is not a
// credential oracle a caller could probe.
func TestRequireAuthRejectsAWrongToken(t *testing.T) {
	t.Parallel()

	srv := NewServer(nil, fakeContracts{deleteFn: func(context.Context, string) error {
		t.Error("Delete reached the store despite a wrong bearer token")
		return nil
	}}, fakeEvents{}, "1.2.3", WithAuthToken(testAuthToken))

	r := httptest.NewRequest(http.MethodDelete, "/contracts/"+validContractID, nil)
	r.Header.Set("Authorization", "Bearer not-the-configured-token")
	rr := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rr, r)

	assertUnauthorized(t, rr)
}

// A non-bearer scheme is refused: the gate accepts only "Bearer <token>", so a
// Basic credential is an auth failure, not a fallback.
func TestRequireAuthRejectsANonBearerScheme(t *testing.T) {
	t.Parallel()

	srv := NewServer(nil, fakeContracts{}, fakeEvents{}, "1.2.3", WithAuthToken(testAuthToken))

	r := httptest.NewRequest(http.MethodPost, "/contracts", strings.NewReader(`{"contract_id":"`+validContractID+`"}`))
	r.Header.Set("Authorization", "Basic dXNlcjpwYXNz")
	rr := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rr, r)

	assertUnauthorized(t, rr)
}

// A server built without a token fails closed: every write is refused rather
// than left open, so a deployment that forgot to set the token cannot expose
// open writes. Here even a plausible-looking bearer is rejected.
func TestRequireAuthFailsClosedWhenUnconfigured(t *testing.T) {
	t.Parallel()

	srv := NewServer(nil, fakeContracts{deleteFn: func(context.Context, string) error {
		t.Error("Delete reached the store though no token was configured")
		return nil
	}}, fakeEvents{}, "1.2.3")

	r := httptest.NewRequest(http.MethodDelete, "/contracts/"+validContractID, nil)
	r.Header.Set("Authorization", "Bearer anything")
	rr := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rr, r)

	assertUnauthorized(t, rr)
}

// The token a caller presents must not appear in the response, so a rejected
// request cannot confirm a partial guess by echoing it.
func TestRequireAuthDoesNotEchoTheAttemptedToken(t *testing.T) {
	t.Parallel()

	const attempt = "a-secret-guess-value-abc123"
	srv := NewServer(nil, fakeContracts{}, fakeEvents{}, "1.2.3", WithAuthToken(testAuthToken))

	r := httptest.NewRequest(http.MethodDelete, "/contracts/"+validContractID, nil)
	r.Header.Set("Authorization", "Bearer "+attempt)
	rr := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rr, r)

	if strings.Contains(rr.Body.String(), attempt) {
		t.Errorf("the attempted token leaked into the response: %s", rr.Body.String())
	}
}

// The read routes carry no auth: a GET reaches its handler with no
// Authorization header, since ADR-044 scopes the gate to the write surface.
func TestReadRoutesRequireNoAuth(t *testing.T) {
	t.Parallel()

	srv := NewServer(nil, fakeContracts{}, fakeEvents{}, "1.2.3", WithAuthToken(testAuthToken))

	for _, target := range []string{"/health", "/contracts"} {
		rr := serve(srv, http.MethodGet, target, "")
		if rr.Code == http.StatusUnauthorized {
			t.Errorf("GET %s returned 401; reads must not require auth", target)
		}
	}
}

// The rate limiter caps the write surface: with a burst of two, the first two
// authed writes pass and the third is refused with the 429 RATE_LIMITED
// envelope. The bucket is shared across the write routes.
func TestRateLimitRefusesWritesOverBudget(t *testing.T) {
	t.Parallel()

	srv := NewServer(nil, fakeContracts{}, fakeEvents{}, "1.2.3",
		WithAuthToken(testAuthToken), WithWriteRateLimit(1, 2))

	body := `{"contract_id":"` + validContractID + `"}`
	if rr := serveAuthed(srv, http.MethodPost, "/contracts", body); rr.Code == http.StatusTooManyRequests {
		t.Fatal("first write was rate-limited within the burst budget")
	}
	if rr := serveAuthed(srv, http.MethodPost, "/contracts", body); rr.Code == http.StatusTooManyRequests {
		t.Fatal("second write was rate-limited within the burst budget")
	}

	rr := serveAuthed(srv, http.MethodPost, "/contracts", body)
	if rr.Code != http.StatusTooManyRequests {
		t.Fatalf("third write status = %d, want 429 once the burst is spent", rr.Code)
	}
	var env decodedError
	if err := json.Unmarshal(rr.Body.Bytes(), &env); err != nil {
		t.Fatalf("body is not JSON: %v; body=%s", err, rr.Body.String())
	}
	if env.Error.Code != "rate_limited" {
		t.Errorf("error.code = %q, want rate_limited", env.Error.Code)
	}
	if env.Error.Details.Code != string(apierror.CodeRateLimited) {
		t.Errorf("error.details.code = %q, want %q", env.Error.Details.Code, apierror.CodeRateLimited)
	}
}

// The write limiter must not touch reads: a burst of one, already spent by a
// write, still leaves GET responses flowing, since the limiter wraps only the
// write routes.
func TestRateLimitLeavesReadsUnbounded(t *testing.T) {
	t.Parallel()

	srv := NewServer(nil, fakeContracts{}, fakeEvents{}, "1.2.3",
		WithAuthToken(testAuthToken), WithWriteRateLimit(1, 1))

	// Spend the single burst token on a write.
	serveAuthed(srv, http.MethodPost, "/contracts", `{"contract_id":"`+validContractID+`"}`)

	for i := 0; i < 5; i++ {
		if rr := serve(srv, http.MethodGet, "/health", ""); rr.Code == http.StatusTooManyRequests {
			t.Fatalf("GET /health was rate-limited; the write limiter must not bound reads")
		}
	}
}

// ReadDeadline derives a bounded context: a handler that outlives the deadline
// sees its request context cancelled with DeadlineExceeded, which is what lets
// a slow store read surface as an error rather than hang.
func TestReadDeadlineCancelsAnOverrunningHandler(t *testing.T) {
	t.Parallel()

	var sawErr error
	h := ReadDeadline(10 * time.Millisecond)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
			sawErr = r.Context().Err()
		case <-time.After(2 * time.Second):
		}
	}))

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))

	if sawErr != context.DeadlineExceeded {
		t.Errorf("handler saw ctx error %v, want context.DeadlineExceeded", sawErr)
	}
}

// A non-positive duration disables the guard: the handler runs with the
// request's own context, carrying no deadline, so the four-argument NewServer
// is unaffected.
func TestReadDeadlineDisabledLeavesTheContextUnbounded(t *testing.T) {
	t.Parallel()

	var hadDeadline bool
	h := ReadDeadline(0)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		_, hadDeadline = r.Context().Deadline()
	}))

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))

	if hadDeadline {
		t.Error("a disabled ReadDeadline set a context deadline")
	}
}

// Every response carries X-Content-Type-Options: nosniff (ADR-043 L2), on a
// success and on an error alike, since SecurityHeaders wraps the whole surface.
func TestSecurityHeadersSetNosniffOnEveryResponse(t *testing.T) {
	t.Parallel()

	srv := NewServer(nil, fakeContracts{}, fakeEvents{}, "1.2.3", WithAuthToken(testAuthToken))

	// A success (GET /health) and an auth failure (unauthed write) both carry it.
	ok := serve(srv, http.MethodGet, "/health", "")
	if got := ok.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Errorf("X-Content-Type-Options on 200 = %q, want nosniff", got)
	}
	denied := serve(srv, http.MethodDelete, "/contracts/"+validContractID, "")
	if got := denied.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Errorf("X-Content-Type-Options on 401 = %q, want nosniff", got)
	}
}

// assertUnauthorized asserts rr is the 401 UNAUTHORIZED envelope the auth gate
// returns for a missing or wrong bearer token.
func assertUnauthorized(t *testing.T, rr *httptest.ResponseRecorder) {
	t.Helper()

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rr.Code)
	}
	assertJSONContentType(t, rr)

	var env decodedError
	if err := json.Unmarshal(rr.Body.Bytes(), &env); err != nil {
		t.Fatalf("body is not JSON: %v; body=%s", err, rr.Body.String())
	}
	if env.Error.Code != "unauthorized" {
		t.Errorf("error.code = %q, want unauthorized", env.Error.Code)
	}
	if env.Error.Details.Code != string(apierror.CodeUnauthorized) {
		t.Errorf("error.details.code = %q, want %q", env.Error.Details.Code, apierror.CodeUnauthorized)
	}
}
