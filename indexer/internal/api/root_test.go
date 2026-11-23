package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/pulsar-stellar/pulsar-app/indexer/internal/apierror"
	"github.com/pulsar-stellar/pulsar-app/indexer/internal/models"
)

// decodedRoot mirrors the ADR-017 success envelope for GET /.
type decodedRoot struct {
	Data struct {
		Name    string            `json:"name"`
		Version string            `json:"version"`
		Links   map[string]string `json:"links"`
	} `json:"data"`
}

// GET / tells a visitor what the indexer serves and where to read about it
// (ADR-057), so the bare URL is not a 404.
func TestRootLinksToTheServedSurface(t *testing.T) {
	t.Parallel()

	srv := NewServer(nil, fakeContracts{}, fakeEvents{}, "1.2.3")
	rr := serve(srv, http.MethodGet, "/", "")

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	assertJSONContentType(t, rr)

	var env decodedRoot
	if err := json.Unmarshal(rr.Body.Bytes(), &env); err != nil {
		t.Fatalf("body is not JSON: %v; body=%s", err, rr.Body.String())
	}
	if env.Data.Name != "pulsar-indexer" {
		t.Errorf("data.name = %q, want pulsar-indexer", env.Data.Name)
	}
	if env.Data.Version != "1.2.3" {
		t.Errorf("data.version = %q, want 1.2.3", env.Data.Version)
	}

	want := map[string]string{
		"health":    "/health",
		"graphql":   "/graphql",
		"docs_site": docsSiteURL,
	}
	if len(env.Data.Links) != len(want) {
		t.Errorf("data.links = %v, want exactly %v", env.Data.Links, want)
	}
	for key, value := range want {
		if env.Data.Links[key] != value {
			t.Errorf("data.links.%s = %q, want %q", key, env.Data.Links[key], value)
		}
	}
	if !strings.HasPrefix(docsSiteURL, "https://") {
		t.Errorf("docsSiteURL = %q, want an https URL", docsSiteURL)
	}
}

// Every relative link the root advertises must answer, so the index never
// points a visitor at a 404. /graphql is POST-only (ADR-043), so it is checked
// with the method it serves.
func TestRootLinksResolve(t *testing.T) {
	t.Parallel()

	srv := NewServer(nil, fakeContracts{}, fakeEvents{}, "1.2.3")
	var env decodedRoot
	if err := json.Unmarshal(serve(srv, http.MethodGet, "/", "").Body.Bytes(), &env); err != nil {
		t.Fatal(err)
	}

	for key, link := range env.Data.Links {
		if !strings.HasPrefix(link, "/") {
			continue // absolute: the docs site, not this server
		}
		method, body := http.MethodGet, ""
		if link == "/graphql" {
			method, body = http.MethodPost, `{"query":"{ health { ok } }"}`
		}
		if rr := serve(srv, method, link, body); rr.Code != http.StatusOK {
			t.Errorf("links.%s %s %s = %d, want 200", key, method, link, rr.Code)
		}
	}
}

// GET / is a public read, so it spends from the read bucket like the others
// (ADR-055).
func TestRootIsReadRateLimited(t *testing.T) {
	t.Parallel()

	srv := NewServer(nil, fakeContracts{}, fakeEvents{}, "1.2.3", WithReadRateLimit(1, 1))
	serve(srv, http.MethodGet, "/", "")
	if rr := serve(srv, http.MethodGet, "/", ""); rr.Code != http.StatusTooManyRequests {
		t.Errorf("second GET / = %d, want 429", rr.Code)
	}
}

// getRouteProbes holds a concrete request for every GET route the router
// registers. TestHeadAnswersEveryGetRoute walks the router and fails on a GET
// route missing here, so a new route cannot ship without HEAD coverage.
var getRouteProbes = map[string]string{
	"/":                      "/",
	"/health":                "/health",
	"/contracts":             "/contracts",
	"/contracts/{id}":        "/contracts/" + validContractID,
	"/contracts/{id}/events": "/contracts/" + validContractID + "/events",
	"/events/{id}":           "/events/1",
}

// probeServer answers every probe with a 200, so the HEAD and GET statuses
// compare like for like.
func probeServer() *Server {
	return NewServer(nil, fakeContracts{}, fakeEvents{
		getFn: func(context.Context, int64) (*models.Event, error) { return sampleEvent(1, 100, "transfer"), nil },
	}, "1.2.3")
}

// HEAD answers every GET route with the GET's status (ADR-057, closes #41).
// Uptime monitors often probe with HEAD, and before this a HEAD on /health
// answered 404 NOT_FOUND_METHOD.
func TestHeadAnswersEveryGetRoute(t *testing.T) {
	t.Parallel()

	srv := probeServer()
	err := chi.Walk(srv.router(), func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		if method != http.MethodGet {
			return nil
		}
		target, ok := getRouteProbes[route]
		if !ok {
			t.Errorf("GET %s has no entry in getRouteProbes; add one so HEAD is tested on it", route)
			return nil
		}
		get := serve(srv, http.MethodGet, target, "")
		head := serve(srv, http.MethodHead, target, "")
		if get.Code != http.StatusOK {
			t.Errorf("GET %s = %d, want 200 (the probe should succeed)", target, get.Code)
		}
		if head.Code != get.Code {
			t.Errorf("HEAD %s = %d, want %d (the GET status)", target, head.Code, get.Code)
		}
		if head.Header().Get("Content-Type") != get.Header().Get("Content-Type") {
			t.Errorf("HEAD %s Content-Type = %q, want %q", target, head.Header().Get("Content-Type"), get.Header().Get("Content-Type"))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// Over a real connection a HEAD response carries the headers and no body.
func TestHeadHealthOverHTTPSendsNoBody(t *testing.T) {
	t.Parallel()

	ts := httptest.NewServer(probeServer().Routes())
	t.Cleanup(ts.Close)

	req, err := http.NewRequest(http.MethodHead, ts.URL+"/health", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("HEAD /health = %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if len(body) != 0 {
		t.Errorf("HEAD body = %q, want empty", body)
	}
}

// HEAD maps only onto GET routes. /graphql serves POST alone (ADR-043), so a
// HEAD there still answers 404 NOT_FOUND_METHOD.
func TestHeadOnAPostOnlyRouteStaysNotFound(t *testing.T) {
	t.Parallel()

	rr := serve(probeServer(), http.MethodHead, "/graphql", "")
	if rr.Code != http.StatusNotFound {
		t.Fatalf("HEAD /graphql = %d, want 404", rr.Code)
	}
	var env decodedError
	if err := json.Unmarshal(rr.Body.Bytes(), &env); err != nil {
		t.Fatalf("body is not JSON: %v", err)
	}
	if env.Error.Details.Code != string(apierror.CodeNotFoundMethod) {
		t.Errorf("error.details.code = %q, want %q", env.Error.Details.Code, apierror.CodeNotFoundMethod)
	}
}

// A HEAD on a read route runs the GET handler and its middleware, so it spends
// from the read bucket too. Otherwise HEAD would be a way around ADR-055's
// limit to the store.
func TestHeadSpendsTheReadBucket(t *testing.T) {
	t.Parallel()

	srv := NewServer(nil, fakeContracts{}, fakeEvents{}, "1.2.3", WithReadRateLimit(1, 1))
	serve(srv, http.MethodHead, "/contracts", "")
	if rr := serve(srv, http.MethodHead, "/contracts", ""); rr.Code != http.StatusTooManyRequests {
		t.Errorf("second HEAD /contracts = %d, want 429", rr.Code)
	}
	if rr := serve(srv, http.MethodGet, "/contracts", ""); rr.Code != http.StatusTooManyRequests {
		t.Errorf("GET /contracts after HEAD spent the bucket = %d, want 429", rr.Code)
	}
}

// HEAD reaches only the GET tree. With auth configured, an unauthenticated HEAD
// on /contracts/{id} answers like the public GET and never reaches the gated
// DELETE handler on the same path.
func TestHeadNeverReachesAWriteHandler(t *testing.T) {
	t.Parallel()

	deleted := false
	srv := NewServer(nil, fakeContracts{
		deleteFn: func(context.Context, string) error { deleted = true; return nil },
	}, fakeEvents{}, "1.2.3", WithAuthToken(testAuthToken))

	rr := serve(srv, http.MethodHead, "/contracts/"+validContractID, "")
	if rr.Code != http.StatusOK {
		t.Errorf("unauthenticated HEAD /contracts/{id} = %d, want 200 (the public GET)", rr.Code)
	}
	if deleted {
		t.Error("HEAD reached the DELETE handler")
	}
}

// A HEAD on an unknown path still answers 404, and every HEAD response carries
// the surface-wide security headers.
func TestHeadKeepsNotFoundAndSecurityHeaders(t *testing.T) {
	t.Parallel()

	srv := probeServer()
	if rr := serve(srv, http.MethodHead, "/does-not-exist", ""); rr.Code != http.StatusNotFound {
		t.Errorf("HEAD /does-not-exist = %d, want 404", rr.Code)
	}
	for _, target := range []string{"/", "/health", "/does-not-exist"} {
		if got := serve(srv, http.MethodHead, target, "").Header().Get("X-Content-Type-Options"); got != "nosniff" {
			t.Errorf("HEAD %s X-Content-Type-Options = %q, want nosniff", target, got)
		}
	}
}
