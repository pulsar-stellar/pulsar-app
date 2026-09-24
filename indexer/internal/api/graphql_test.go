package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pulsar-stellar/pulsar-app/indexer/internal/apierror"
	"github.com/pulsar-stellar/pulsar-app/indexer/internal/models"
	"github.com/pulsar-stellar/pulsar-app/indexer/internal/store"
)

// gqlResponse mirrors the spec-fixed GraphQL { data, errors } envelope. data is
// kept raw so a test decodes only the shape it asserts on, and each error
// carries the catalog code and wire class the resolverError adapter attaches to
// extensions (ADR-043).
type gqlResponse struct {
	Data   json.RawMessage `json:"data"`
	Errors []struct {
		Message    string `json:"message"`
		Extensions struct {
			Code  string `json:"code"`
			Class string `json:"class"`
		} `json:"extensions"`
	} `json:"errors"`
}

// gql POSTs a GraphQL query to /graphql through the full mux and returns the
// recorder, mirroring how the REST tests use serve. The body is the
// GraphQL-over-HTTP JSON shape with just the query.
func gql(srv *Server, query string) *httptest.ResponseRecorder {
	body, _ := json.Marshal(map[string]any{"query": query})
	return serve(srv, http.MethodPost, "/graphql", string(body))
}

// decodeGQL decodes rr's body as the GraphQL envelope, failing the test if it
// is not JSON.
func decodeGQL(t *testing.T, rr *httptest.ResponseRecorder) gqlResponse {
	t.Helper()
	var resp gqlResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("body is not JSON: %v; body=%s", err, rr.Body.String())
	}
	return resp
}

// assertGQLErrorCode asserts rr is a 200 carrying exactly one GraphQL error
// whose extensions.code is want, the shared shape of every catalogued resolver
// failure.
func assertGQLErrorCode(t *testing.T, rr *httptest.ResponseRecorder, want apierror.Code) {
	t.Helper()
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	resp := decodeGQL(t, rr)
	if len(resp.Errors) != 1 {
		t.Fatalf("errors has %d entries, want 1; body=%s", len(resp.Errors), rr.Body.String())
	}
	if resp.Errors[0].Extensions.Code != string(want) {
		t.Errorf("errors[0].extensions.code = %q, want %q", resp.Errors[0].Extensions.Code, want)
	}
}

// Query.events

func TestGraphQLEventsReturnsTheConnection(t *testing.T) {
	t.Parallel()

	next := "9"
	var gotQuery store.EventQuery
	srv := NewServer(nil, fakeContracts{}, fakeEvents{queryFn: func(_ context.Context, q store.EventQuery) (store.EventsPage, error) {
		gotQuery = q
		return store.EventsPage{
			Events:     []*models.Event{sampleEvent(7, 100, "transfer")},
			NextCursor: &next,
		}, nil
	}}, "1.2.3")

	query := fmt.Sprintf(`{ events(contractId: %q, limit: 1) { items { id contractId ledger name } nextCursor } }`, validContractID)
	rr := gql(srv, query)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	assertJSONContentType(t, rr)

	// The typed arguments must reach the store as the mapped EventQuery: the
	// contract id set, the limit threaded, and order defaulted to desc even
	// though the store's own default is ascending (ADR-041).
	if gotQuery.ContractID != validContractID {
		t.Errorf("store saw contract id %q, want %q", gotQuery.ContractID, validContractID)
	}
	if gotQuery.Limit != 1 {
		t.Errorf("store saw limit %d, want 1", gotQuery.Limit)
	}
	if gotQuery.Order != "desc" {
		t.Errorf("store saw order %q, want desc", gotQuery.Order)
	}

	resp := decodeGQL(t, rr)
	if len(resp.Errors) != 0 {
		t.Fatalf("errors = %+v, want none; body=%s", resp.Errors, rr.Body.String())
	}

	var data struct {
		Events struct {
			Items []struct {
				ID         string `json:"id"`
				ContractID string `json:"contractId"`
				Ledger     int64  `json:"ledger"`
				Name       string `json:"name"`
			} `json:"items"`
			NextCursor *string `json:"nextCursor"`
		} `json:"events"`
	}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		t.Fatalf("data is not the expected shape: %v; data=%s", err, resp.Data)
	}
	if len(data.Events.Items) != 1 {
		t.Fatalf("items has %d events, want 1", len(data.Events.Items))
	}
	if data.Events.Items[0].ID != "7" {
		t.Errorf("items[0].id = %q, want \"7\"", data.Events.Items[0].ID)
	}
	if data.Events.Items[0].Name != "transfer" {
		t.Errorf("items[0].name = %q, want transfer", data.Events.Items[0].Name)
	}
	if data.Events.NextCursor == nil || *data.Events.NextCursor != "9" {
		t.Errorf("nextCursor = %v, want \"9\"", data.Events.NextCursor)
	}

	// The id must travel as a JSON string, not a number: a BIGSERIAL exceeds the
	// range a JSON number carries without loss (ADR-021), and the output field is
	// camelCase.
	if !strings.Contains(rr.Body.String(), `"id":"7"`) {
		t.Errorf("event id is not a JSON string; body=%s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"contractId"`) {
		t.Errorf("output field is not camelCase contractId; body=%s", rr.Body.String())
	}
}

func TestGraphQLEventsAppliesTheSDKDefaults(t *testing.T) {
	t.Parallel()

	var got store.EventQuery
	srv := NewServer(nil, fakeContracts{}, fakeEvents{queryFn: func(_ context.Context, q store.EventQuery) (store.EventsPage, error) {
		got = q
		return store.EventsPage{Events: []*models.Event{}}, nil
	}}, "1.2.3")

	query := fmt.Sprintf(`{ events(contractId: %q) { items { id } } }`, validContractID)
	rr := gql(srv, query)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	if got.Limit != 50 {
		t.Errorf("default limit = %d, want 50", got.Limit)
	}
	if got.Order != "desc" {
		t.Errorf("default order = %q, want desc", got.Order)
	}
}

// A tracked contract with no matches is items: [], never null, and distinct
// from an untracked contract's error.
func TestGraphQLEventsReturnsEmptyItemsNotNull(t *testing.T) {
	t.Parallel()

	srv := NewServer(nil, fakeContracts{}, fakeEvents{queryFn: func(context.Context, store.EventQuery) (store.EventsPage, error) {
		return store.EventsPage{}, nil
	}}, "1.2.3")

	query := fmt.Sprintf(`{ events(contractId: %q) { items { id } nextCursor } }`, validContractID)
	rr := gql(srv, query)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"items":[]`) {
		t.Errorf("empty page did not serialize items as []; body=%s", rr.Body.String())
	}
}

// An untracked contract is a NOT_FOUND_CONTRACT resolver error, not an empty
// connection, and the events store is never read (ADR-021).
func TestGraphQLEventsReturnsNotFoundWhenContractUntracked(t *testing.T) {
	t.Parallel()

	srv := NewServer(nil, fakeContracts{getFn: func(_ context.Context, id string) (models.Contract, error) {
		return models.Contract{}, fmt.Errorf("store: contract %s: %w", id, store.ErrNotFound)
	}}, fakeEvents{queryFn: func(context.Context, store.EventQuery) (store.EventsPage, error) {
		t.Error("Query reached the store for an untracked contract")
		return store.EventsPage{}, nil
	}}, "1.2.3")

	query := fmt.Sprintf(`{ events(contractId: %q) { items { id } } }`, validContractID)
	rr := gql(srv, query)

	assertGQLErrorCode(t, rr, apierror.CodeNotFoundContract)
}

// Every invalid argument is a catalogued resolver error naming the field that
// failed, and validation precedes every store read: the hooks fail the test if
// either store is reached.
func TestGraphQLEventsRejectsInvalidArguments(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		args       string
		wantDetail apierror.Code
	}{
		{"limit below the minimum", `limit: 0`, apierror.CodeValidationLimit},
		{"limit above the maximum", `limit: 501`, apierror.CodeValidationLimit},
		{"order is another word", `order: "up"`, apierror.CodeValidationOrder},
		{"order is the wrong case", `order: "DESC"`, apierror.CodeValidationOrder},
		{"cursor is negative", `cursor: "-5"`, apierror.CodeValidationCursor},
		{"from_ledger is negative", `fromLedger: -1`, apierror.CodeValidationLedgerRange},
		{"inverted ledger window", `fromLedger: 100, toLedger: 50`, apierror.CodeValidationLedgerRange},
		{"name carries a NUL byte", `name: "\u0000"`, apierror.CodeValidationFilter},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			srv := NewServer(nil, fakeContracts{getFn: func(context.Context, string) (models.Contract, error) {
				t.Error("Get reached the store despite an invalid argument")
				return models.Contract{}, nil
			}}, fakeEvents{queryFn: func(context.Context, store.EventQuery) (store.EventsPage, error) {
				t.Error("Query reached the store despite an invalid argument")
				return store.EventsPage{}, nil
			}}, "1.2.3")

			query := fmt.Sprintf(`{ events(contractId: %q, %s) { items { id } } }`, validContractID, tt.args)
			rr := gql(srv, query)

			assertGQLErrorCode(t, rr, tt.wantDetail)
		})
	}
}

// A malformed contract id is rejected before either store is read.
func TestGraphQLEventsRejectsAMalformedContractID(t *testing.T) {
	t.Parallel()

	srv := NewServer(nil, fakeContracts{getFn: func(context.Context, string) (models.Contract, error) {
		t.Error("Get reached the store despite a malformed contract id")
		return models.Contract{}, nil
	}}, fakeEvents{queryFn: func(context.Context, store.EventQuery) (store.EventsPage, error) {
		t.Error("Query reached the store despite a malformed contract id")
		return store.EventsPage{}, nil
	}}, "1.2.3")

	rr := gql(srv, `{ events(contractId: "not-a-contract-id") { items { id } } }`)

	assertGQLErrorCode(t, rr, apierror.CodeValidationContractID)
}

func TestGraphQLEventsReturnsInternalWhenTheStoreFails(t *testing.T) {
	t.Parallel()

	log, buf := newTestLogger()
	const secret = "query timeout against replica 10.2.3.4"
	srv := NewServer(log, fakeContracts{}, fakeEvents{queryFn: func(context.Context, store.EventQuery) (store.EventsPage, error) {
		return store.EventsPage{}, errorString(secret)
	}}, "1.2.3")

	query := fmt.Sprintf(`{ events(contractId: %q) { items { id } } }`, validContractID)
	rr := gql(srv, query)

	assertGQLErrorCode(t, rr, apierror.CodeInternalStore)

	// The real cause is logged server-side but must never reach the client, the
	// same discipline as the REST leak tests.
	if strings.Contains(rr.Body.String(), secret) {
		t.Errorf("the store error leaked into the response; body=%s", rr.Body.String())
	}
	line := findLine(t, buf, "graphql_events_query_failed")
	if got, _ := line["error"].(string); got != secret {
		t.Errorf("logged error = %q, want the injected cause %q", got, secret)
	}
}

// Query.event and Query.contract

func TestGraphQLEventReturnsTheEvent(t *testing.T) {
	t.Parallel()

	var gotID int64
	srv := NewServer(nil, fakeContracts{}, fakeEvents{getFn: func(_ context.Context, id int64) (*models.Event, error) {
		gotID = id
		return sampleEvent(42, 100, "transfer"), nil
	}}, "1.2.3")

	rr := gql(srv, `{ event(id: "42") { id name } }`)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	if gotID != 42 {
		t.Errorf("store saw id %d, want 42", gotID)
	}
	if !strings.Contains(rr.Body.String(), `"id":"42"`) {
		t.Errorf("event id is not a JSON string; body=%s", rr.Body.String())
	}
}

// A well-formed id that finds nothing resolves to null with no error, the
// idiomatic "looked up, absent" (ADR-043).
func TestGraphQLEventReturnsNullWhenAbsent(t *testing.T) {
	t.Parallel()

	srv := NewServer(nil, fakeContracts{}, fakeEvents{getFn: func(_ context.Context, id int64) (*models.Event, error) {
		return nil, fmt.Errorf("store: event %d: %w", id, store.ErrNotFound)
	}}, "1.2.3")

	rr := gql(srv, `{ event(id: "999") { id } }`)

	resp := decodeGQL(t, rr)
	if len(resp.Errors) != 0 {
		t.Fatalf("errors = %+v, want none; absence is null, not an error", resp.Errors)
	}
	if !strings.Contains(rr.Body.String(), `"event":null`) {
		t.Errorf("absent event did not resolve to null; body=%s", rr.Body.String())
	}
}

func TestGraphQLEventRejectsAMalformedID(t *testing.T) {
	t.Parallel()

	srv := NewServer(nil, fakeContracts{}, fakeEvents{getFn: func(context.Context, int64) (*models.Event, error) {
		t.Error("Get reached the store despite a malformed id")
		return nil, nil
	}}, "1.2.3")

	rr := gql(srv, `{ event(id: "abc") { id } }`)

	assertGQLErrorCode(t, rr, apierror.CodeValidationEventID)
}

func TestGraphQLContractReturnsNullWhenAbsent(t *testing.T) {
	t.Parallel()

	srv := NewServer(nil, fakeContracts{getFn: func(_ context.Context, id string) (models.Contract, error) {
		return models.Contract{}, fmt.Errorf("store: contract %s: %w", id, store.ErrNotFound)
	}}, fakeEvents{}, "1.2.3")

	query := fmt.Sprintf(`{ contract(id: %q) { id } }`, validContractID)
	rr := gql(srv, query)

	resp := decodeGQL(t, rr)
	if len(resp.Errors) != 0 {
		t.Fatalf("errors = %+v, want none; absence is null, not an error", resp.Errors)
	}
	if !strings.Contains(rr.Body.String(), `"contract":null`) {
		t.Errorf("absent contract did not resolve to null; body=%s", rr.Body.String())
	}
}

// Contract.events nests a contract's events without a second tracked check: the
// parent already resolved, so the contracts store is read once for the parent
// and the events store once for the nested field.
func TestGraphQLContractEventsNests(t *testing.T) {
	t.Parallel()

	var gotQuery store.EventQuery
	srv := NewServer(nil, fakeContracts{getFn: func(_ context.Context, id string) (models.Contract, error) {
		return models.Contract{ID: id, Status: models.StatusActive, LastIndexedLedger: 100}, nil
	}}, fakeEvents{queryFn: func(_ context.Context, q store.EventQuery) (store.EventsPage, error) {
		gotQuery = q
		return store.EventsPage{Events: []*models.Event{sampleEvent(7, 100, "transfer")}}, nil
	}}, "1.2.3")

	query := fmt.Sprintf(`{ contract(id: %q) { id status events(limit: 5) { items { id name } } } }`, validContractID)
	rr := gql(srv, query)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	resp := decodeGQL(t, rr)
	if len(resp.Errors) != 0 {
		t.Fatalf("errors = %+v, want none; body=%s", resp.Errors, rr.Body.String())
	}
	if gotQuery.ContractID != validContractID {
		t.Errorf("nested events store saw contract id %q, want %q", gotQuery.ContractID, validContractID)
	}
	if gotQuery.Limit != 5 {
		t.Errorf("nested events store saw limit %d, want 5", gotQuery.Limit)
	}
	if !strings.Contains(rr.Body.String(), `"name":"transfer"`) {
		t.Errorf("nested events did not resolve; body=%s", rr.Body.String())
	}
}

// A panic inside a resolver is recovered by the library before the mux-level
// Recoverer sees it, so the schema's own PanicHandler must turn it into the
// generic INTERNAL_PANIC error: the recovered value is logged server-side but
// never reaches the client, the same leak discipline as the store-failure path
// (ADR-043).
func TestGraphQLRecoversAResolverPanicWithoutLeaking(t *testing.T) {
	t.Parallel()

	log, buf := newTestLogger()
	const secret = "panic against replica 10.2.3.4"
	srv := NewServer(log, fakeContracts{listFn: func(context.Context) ([]models.Contract, error) {
		panic(secret)
	}}, fakeEvents{}, "1.2.3")

	rr := gql(srv, `{ contracts { id } }`)

	assertGQLErrorCode(t, rr, apierror.CodeInternalPanic)

	// The recovered value is logged but must not travel to the client.
	if strings.Contains(rr.Body.String(), secret) {
		t.Errorf("the panic value leaked into the response; body=%s", rr.Body.String())
	}
	line := findLine(t, buf, "graphql_resolver_panic")
	if got, _ := line["panic"].(string); got != secret {
		t.Errorf("logged panic = %q, want the recovered value %q", got, secret)
	}
}

// Transport and security posture

// A body over the cap is a transport-level rejection: 400 with a
// GraphQL-shaped error, before the schema executes.
func TestGraphQLRejectsABodyOverTheCap(t *testing.T) {
	t.Parallel()

	srv := NewServer(nil, fakeContracts{}, fakeEvents{}, "1.2.3")

	// A query padded past maxGraphQLBodyBytes with a long comment. It stays valid
	// GraphQL so only the size, not the shape, triggers the rejection.
	pad := strings.Repeat("#pad\n", (maxGraphQLBodyBytes/5)+16)
	body, _ := json.Marshal(map[string]any{"query": pad + "{ __typename }"})
	rr := serve(srv, http.MethodPost, "/graphql", string(body))

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rr.Code, rr.Body.String())
	}
	assertJSONContentType(t, rr)
	if len(decodeGQL(t, rr).Errors) == 0 {
		t.Errorf("over-cap body did not return a GraphQL-shaped error; body=%s", rr.Body.String())
	}
}

// A query over MaxQueryLength is rejected by the schema at 200 with an error,
// the built-in GraphQL DoS guard that motivated the library choice (ADR-043).
func TestGraphQLRejectsAQueryOverTheLengthLimit(t *testing.T) {
	t.Parallel()

	srv := NewServer(nil, fakeContracts{}, fakeEvents{}, "1.2.3")

	// Many aliased __typename selections build a valid query longer than the
	// 8192-byte MaxQueryLength without needing an over-cap body.
	var b strings.Builder
	b.WriteString("{ ")
	for i := 0; i < 800; i++ {
		fmt.Fprintf(&b, "a%d: __typename ", i)
	}
	b.WriteString("}")
	rr := gql(srv, b.String())

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	if len(decodeGQL(t, rr).Errors) == 0 {
		t.Errorf("over-length query was not rejected; body=%s", rr.Body.String())
	}
}

// A malformed JSON body is a transport-level rejection at 400.
func TestGraphQLRejectsAMalformedBody(t *testing.T) {
	t.Parallel()

	srv := NewServer(nil, fakeContracts{}, fakeEvents{}, "1.2.3")

	rr := serve(srv, http.MethodPost, "/graphql", "{ this is not json")

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rr.Code, rr.Body.String())
	}
	if len(decodeGQL(t, rr).Errors) == 0 {
		t.Errorf("malformed body did not return a GraphQL-shaped error; body=%s", rr.Body.String())
	}
}

// An empty query is a transport-level rejection at 400: there is nothing for the
// schema to execute.
func TestGraphQLRejectsAnEmptyQuery(t *testing.T) {
	t.Parallel()

	srv := NewServer(nil, fakeContracts{}, fakeEvents{}, "1.2.3")

	rr := serve(srv, http.MethodPost, "/graphql", `{"query": ""}`)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rr.Code, rr.Body.String())
	}
}

// Introspection is enabled: the schema is public and documented, so the
// standard introspection query succeeds (the recorded posture, ADR-043).
func TestGraphQLIntrospectionIsEnabled(t *testing.T) {
	t.Parallel()

	srv := NewServer(nil, fakeContracts{}, fakeEvents{}, "1.2.3")

	rr := gql(srv, `{ __schema { queryType { name } } }`)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	resp := decodeGQL(t, rr)
	if len(resp.Errors) != 0 {
		t.Fatalf("introspection returned errors %+v; it must succeed", resp.Errors)
	}
	if !strings.Contains(rr.Body.String(), `"name":"Query"`) {
		t.Errorf("introspection did not report the Query type; body=%s", rr.Body.String())
	}
}
