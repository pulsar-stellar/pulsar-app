package api

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers"
	"github.com/getkin/kin-openapi/routers/legacy"
	"github.com/go-chi/chi/v5"

	"github.com/pulsar-stellar/pulsar-app/indexer/internal/models"
	"github.com/pulsar-stellar/pulsar-app/indexer/internal/store"
)

// The docs page and its assets are HTML, CSS and JavaScript. kin-openapi reads
// those response bodies as plain text, so the schema check sees a string.
func init() {
	for _, ct := range []string{"text/html", "text/css", "text/javascript"} {
		openapi3filter.RegisterBodyDecoder(ct, openapi3filter.PlainBodyDecoder)
	}
}

// loadSpec parses and validates the embedded OpenAPI document. The validator
// is a test-only dependency (ADR-058); the binary serves the bytes as written.
func loadSpec(t *testing.T) *openapi3.T {
	t.Helper()

	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromData(openAPISpec)
	if err != nil {
		t.Fatalf("openapi.json does not parse: %v", err)
	}
	if err := doc.Validate(context.Background()); err != nil {
		t.Fatalf("openapi.json is not a valid OpenAPI document: %v", err)
	}
	return doc
}

func TestOpenAPISpecIsValid(t *testing.T) {
	t.Parallel()
	loadSpec(t)
}

// The spec and the router describe the same surface (ADR-058). Every route the
// router registers has an operation in the spec, and every operation in the
// spec has a route, so neither can drift without failing CI. HEAD is not
// listed: it mirrors GET on every route (ADR-057).
func TestOpenAPIMatchesTheRouter(t *testing.T) {
	t.Parallel()

	doc := loadSpec(t)
	inSpec := map[string]bool{}
	for path, item := range doc.Paths.Map() {
		for method := range item.Operations() {
			inSpec[method+" "+path] = true
		}
	}

	inRouter := map[string]bool{}
	err := chi.Walk(probeServer().router(), func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		inRouter[method+" "+route] = true
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	for _, op := range sortedKeys(inRouter) {
		if !inSpec[op] {
			t.Errorf("%s is routed but missing from openapi.json", op)
		}
	}
	for _, op := range sortedKeys(inSpec) {
		if !inRouter[op] {
			t.Errorf("%s is in openapi.json but no route serves it", op)
		}
	}
}

func sortedKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// GET /openapi.json serves the document with info.version set to the running
// build, so the spec never claims a different version from the binary.
func TestOpenAPIServesTheBuildVersion(t *testing.T) {
	t.Parallel()

	rr := serve(probeServer(), http.MethodGet, "/openapi.json", "")
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	assertJSONContentType(t, rr)
	if got := rr.Header().Get("Cache-Control"); got != "public, max-age=300" {
		t.Errorf("Cache-Control = %q, want public, max-age=300", got)
	}

	served, err := openapi3.NewLoader().LoadFromData(rr.Body.Bytes())
	if err != nil {
		t.Fatalf("served document does not parse: %v", err)
	}
	if served.Info.Version != "1.2.3" {
		t.Errorf("info.version = %q, want the build version 1.2.3", served.Info.Version)
	}
	if got, want := len(served.Paths.Map()), len(loadSpec(t).Paths.Map()); got != want {
		t.Errorf("served %d paths, the embedded spec has %d", got, want)
	}
}

func TestOpenAPIIsReadRateLimited(t *testing.T) {
	t.Parallel()

	srv := NewServer(nil, fakeContracts{}, fakeEvents{}, "1.2.3", WithReadRateLimit(1, 1))
	serve(srv, http.MethodGet, "/openapi.json", "")
	if rr := serve(srv, http.MethodGet, "/openapi.json", ""); rr.Code != http.StatusTooManyRequests {
		t.Errorf("second GET /openapi.json = %d, want 429", rr.Code)
	}
}

// shapeCase is one request whose response must match the spec: its status
// must be declared for the operation, and its body must match that status's
// schema.
type shapeCase struct {
	name   string
	srv    *Server
	method string
	target string
	body   string
	authed bool
	spend  bool // send one request first, to spend a one-token bucket
	want   int
}

// Each handler's real response, success and failure, is checked against the
// spec (ADR-058). A renamed field, a new status, or a changed type in a handler
// fails here, so the published document can't describe a different API from
// the one served.
func TestResponsesMatchTheSpec(t *testing.T) {
	t.Parallel()

	doc := loadSpec(t)
	router, err := legacy.NewRouter(doc)
	if err != nil {
		t.Fatal(err)
	}

	next := "41"
	full := NewServer(nil, fakeContracts{
		stats: store.ContractStats{Count: 1, LatestLedger: 900},
		listFn: func(context.Context) ([]models.Contract, error) {
			return []models.Contract{sampleContract(), {ID: validContractID, Status: models.StatusActive}}, nil
		},
		getFn: func(context.Context, string) (models.Contract, error) { return sampleContract(), nil },
		registerFn: func(context.Context, string) (models.Contract, error) {
			return sampleContract(), nil
		},
	}, fakeEvents{
		queryFn: func(context.Context, store.EventQuery) (store.EventsPage, error) {
			return store.EventsPage{Events: []*models.Event{sampleEvent(42, 100, "transfer")}, NextCursor: &next}, nil
		},
		getFn: func(context.Context, int64) (*models.Event, error) { return sampleEvent(42, 100, "transfer"), nil },
	}, "1.2.3", WithAuthToken(testAuthToken), WithDocsURL(testDocsURL))

	failing := NewServer(nil, fakeContracts{
		err:    errorString("store down"),
		listFn: func(context.Context) ([]models.Contract, error) { return nil, errorString("store down") },
		getFn:  func(context.Context, string) (models.Contract, error) { return models.Contract{}, store.ErrNotFound },
	}, fakeEvents{
		getFn: func(context.Context, int64) (*models.Event, error) { return nil, store.ErrNotFound },
	}, "1.2.3", WithAuthToken(testAuthToken))

	limited := NewServer(nil, fakeContracts{}, fakeEvents{}, "1.2.3", WithReadRateLimit(1, 1))

	// The document is built once per server; this one's build has already
	// failed, which drives handleOpenAPI's 500 path.
	brokenSpec := NewServer(nil, fakeContracts{}, fakeEvents{}, "1.2.3")
	brokenSpec.openAPIOnce.Do(func() { brokenSpec.openAPIErr = errorString("bad document") })

	contract := "/contracts/" + validContractID
	cases := []shapeCase{
		{"index", full, http.MethodGet, "/", "", false, false, 200},
		{"health", full, http.MethodGet, "/health", "", false, false, 200},
		{"health store down", failing, http.MethodGet, "/health", "", false, false, 500},
		{"openapi", full, http.MethodGet, "/openapi.json", "", false, false, 200},
		{"openapi build failed", brokenSpec, http.MethodGet, "/openapi.json", "", false, false, 500},
		{"docs page", full, http.MethodGet, "/docs", "", false, false, 200},
		{"docs script", full, http.MethodGet, "/docs/assets/init.js", "", false, false, 200},
		{"docs stylesheet", full, http.MethodGet, "/docs/assets/swagger-ui.css", "", false, false, 200},
		{"docs licence", full, http.MethodGet, "/docs/assets/LICENSE", "", false, false, 200},
		{"docs unknown asset", full, http.MethodGet, "/docs/assets/nope.js", "", false, false, 404},
		{"list contracts", full, http.MethodGet, "/contracts", "", false, false, 200},
		{"list contracts store down", failing, http.MethodGet, "/contracts", "", false, false, 500},
		{"list contracts rate limited", limited, http.MethodGet, "/contracts", "", false, true, 429},
		{"register", full, http.MethodPost, "/contracts", `{"contract_id":"` + validContractID + `"}`, true, false, 200},
		{"register bad body", full, http.MethodPost, "/contracts", `{`, true, false, 400},
		{"register unauthenticated", full, http.MethodPost, "/contracts", `{"contract_id":"` + validContractID + `"}`, false, false, 401},
		{"get contract", full, http.MethodGet, contract, "", false, false, 200},
		{"get contract untracked", failing, http.MethodGet, contract, "", false, false, 404},
		{"delete contract", full, http.MethodDelete, contract, "", true, false, 204},
		{"list events", full, http.MethodGet, contract + "/events?limit=1&order=asc", "", false, false, 200},
		{"list events bad limit", full, http.MethodGet, contract + "/events?limit=0", "", false, false, 400},
		{"get event", full, http.MethodGet, "/events/42", "", false, false, 200},
		{"get event missing", failing, http.MethodGet, "/events/42", "", false, false, 404},
		{"graphql", full, http.MethodPost, "/graphql", `{"query":"{ health { ok } }"}`, false, false, 200},
		{"graphql no query", full, http.MethodPost, "/graphql", `{}`, false, false, 400},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if tc.spend {
				serve(tc.srv, tc.method, tc.target, tc.body)
			}
			var rr *httptest.ResponseRecorder
			if tc.authed {
				rr = serveAuthed(tc.srv, tc.method, tc.target, tc.body)
			} else {
				rr = serve(tc.srv, tc.method, tc.target, tc.body)
			}
			if rr.Code != tc.want {
				t.Fatalf("status = %d, want %d; body=%s", rr.Code, tc.want, rr.Body.String())
			}
			assertMatchesSpec(t, router, tc, rr)
		})
	}
}

func assertMatchesSpec(t *testing.T, router routers.Router, tc shapeCase, rr *httptest.ResponseRecorder) {
	t.Helper()

	req := httptest.NewRequest(tc.method, tc.target, strings.NewReader(tc.body))
	if tc.body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	route, pathParams, err := router.FindRoute(req)
	if err != nil {
		t.Fatalf("%s %s has no operation in openapi.json: %v", tc.method, tc.target, err)
	}

	input := &openapi3filter.ResponseValidationInput{
		RequestValidationInput: &openapi3filter.RequestValidationInput{
			Request:    req,
			PathParams: pathParams,
			Route:      route,
		},
		Status:  rr.Code,
		Header:  rr.Header(),
		Body:    io.NopCloser(bytes.NewReader(rr.Body.Bytes())),
		Options: &openapi3filter.Options{IncludeResponseStatus: true},
	}
	if err := openapi3filter.ValidateResponse(context.Background(), input); err != nil {
		t.Errorf("%s %s (%d) does not match openapi.json: %v\nbody=%s", tc.method, tc.target, rr.Code, err, rr.Body.String())
	}
}

// sampleContract is a fully populated contract record.
func sampleContract() models.Contract {
	first := int64(10)
	return models.Contract{
		ID:                 validContractID,
		AddedAt:            time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
		FirstIndexedLedger: &first,
		LastIndexedLedger:  20,
		Status:             models.StatusActive,
	}
}
