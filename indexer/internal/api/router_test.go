package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pulsar-stellar/pulsar-app/indexer/internal/apierror"
)

func TestRouterUnknownRouteReturnsNotFoundEnvelope(t *testing.T) {
	t.Parallel()

	srv := NewServer(nil) // discard logger; this test asserts the HTTP contract only
	rr := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/does-not-exist", nil))

	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rr.Code)
	}
	assertJSONContentType(t, rr)

	var env decodedError
	if err := json.Unmarshal(rr.Body.Bytes(), &env); err != nil {
		t.Fatalf("body is not JSON: %v; body=%s", err, rr.Body.String())
	}
	if env.Error.Code != "not_found" {
		t.Errorf("error.code = %q, want not_found", env.Error.Code)
	}
	if env.Error.Details.Code != string(apierror.CodeNotFoundRoute) {
		t.Errorf("error.details.code = %q, want %q", env.Error.Details.Code, apierror.CodeNotFoundRoute)
	}
	if env.Error.Message == "" {
		t.Error("error.message is empty")
	}
}

// The middleware stack must wrap the not-found path too, so even a 404 is
// logged and carries a request id. This pins chi's behaviour of applying Use
// middleware around its route matching.
func TestRouterAppliesMiddlewareToNotFound(t *testing.T) {
	t.Parallel()

	log, buf := newTestLogger()
	srv := &Server{log: log}
	rr := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/nope", nil))

	if rr.Header().Get("X-Request-Id") == "" {
		t.Error("no X-Request-Id on a 404; middleware did not wrap the not-found handler")
	}

	line := findLine(t, buf, "http_request")
	if line["status"].(float64) != float64(http.StatusNotFound) {
		t.Errorf("logged status = %v, want 404", line["status"])
	}
	if line["path"] != "/nope" {
		t.Errorf("logged path = %v, want /nope", line["path"])
	}
}
