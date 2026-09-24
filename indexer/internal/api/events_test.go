package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/pulsar-stellar/pulsar-app/indexer/internal/apierror"
	"github.com/pulsar-stellar/pulsar-app/indexer/internal/models"
	"github.com/pulsar-stellar/pulsar-app/indexer/internal/store"
)

// fakeEvents is an eventStore backed by optional function fields, so an events
// handler test both controls the store's result and captures the argument the
// handler passed. It mirrors fakeContracts. An unset field returns a zero
// result, so a test wires only the method the case exercises and any store call
// the handler should not have made can be failed from the hook.
type fakeEvents struct {
	queryFn func(ctx context.Context, q store.EventQuery) (store.EventsPage, error)
	getFn   func(ctx context.Context, id int64) (*models.Event, error)
}

func (f fakeEvents) Query(ctx context.Context, q store.EventQuery) (store.EventsPage, error) {
	if f.queryFn != nil {
		return f.queryFn(ctx, q)
	}
	return store.EventsPage{}, nil
}

func (f fakeEvents) Get(ctx context.Context, id int64) (*models.Event, error) {
	if f.getFn != nil {
		return f.getFn(ctx, id)
	}
	return nil, nil
}

// sampleEvent builds a fully populated event whose JSON serializes to the SDK's
// DecodedEventPayloadSchema: a string id, snake_case keys, decoded topic and
// data objects, and an RFC 3339 emitted_at. A test controls the id, ledger, and
// name, which are the fields it asserts on.
func sampleEvent(id, ledger int64, name string) *models.Event {
	return &models.Event{
		ID:                       id,
		ContractID:               validContractID,
		Ledger:                   ledger,
		TxHash:                   "3389e9f0f1a5b2c7d4e6f8a0b1c2d3e4f5a6b7c8d9e0f1a2b3c4d5e6f7a8b9c0",
		EventIndex:               0,
		Name:                     name,
		TopicsJSON:               json.RawMessage(`[{"type":"symbol","value":"` + name + `"}]`),
		DataJSON:                 json.RawMessage(`{"type":"u32","value":7}`),
		RawTopics:                []string{"AAAAAA=="},
		RawData:                  "BBBBBB==",
		EmittedAt:                time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
		InSuccessfulContractCall: true,
	}
}

// decodedEventList mirrors the ADR-017 paged envelope for GET
// /contracts/:id/events: the events under data.items, the next page's cursor as
// next_cursor beside data rather than inside it, and the timing under meta.
type decodedEventList struct {
	Data struct {
		Items []struct {
			ID         string `json:"id"`
			ContractID string `json:"contract_id"`
			Ledger     int64  `json:"ledger"`
			Name       string `json:"name"`
		} `json:"items"`
	} `json:"data"`
	NextCursor *string `json:"next_cursor"`
	Meta       *struct {
		TookMs float64 `json:"took_ms"`
	} `json:"meta"`
}

// assertValidationDetail asserts rr is the 400 validation envelope carrying want
// as its catalog code, the shared shape of every rejected query parameter.
func assertValidationDetail(t *testing.T, rr *httptest.ResponseRecorder, want apierror.Code) {
	t.Helper()

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
	assertJSONContentType(t, rr)

	var env decodedError
	if err := json.Unmarshal(rr.Body.Bytes(), &env); err != nil {
		t.Fatalf("body is not JSON: %v; body=%s", err, rr.Body.String())
	}
	if env.Error.Code != "validation" {
		t.Errorf("error.code = %q, want validation", env.Error.Code)
	}
	if env.Error.Details.Code != string(want) {
		t.Errorf("error.details.code = %q, want %q", env.Error.Details.Code, want)
	}
}

// GET /contracts/{id}/events

func TestListEventsReturnsThePagedEnvelope(t *testing.T) {
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

	rr := serve(srv, http.MethodGet, "/contracts/"+validContractID+"/events?limit=1&order=desc", "")

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	assertJSONContentType(t, rr)

	// The contract on the path must reach the store as the query's contract id.
	if gotQuery.ContractID != validContractID {
		t.Errorf("store saw contract id %q, want %q", gotQuery.ContractID, validContractID)
	}

	var env decodedEventList
	if err := json.Unmarshal(rr.Body.Bytes(), &env); err != nil {
		t.Fatalf("body is not JSON: %v; body=%s", err, rr.Body.String())
	}
	if len(env.Data.Items) != 1 {
		t.Fatalf("data.items has %d events, want 1", len(env.Data.Items))
	}
	if env.Data.Items[0].ID != "7" {
		t.Errorf("data.items[0].id = %q, want \"7\"", env.Data.Items[0].ID)
	}
	if env.Data.Items[0].Name != "transfer" {
		t.Errorf("data.items[0].name = %q, want transfer", env.Data.Items[0].Name)
	}
	if env.NextCursor == nil || *env.NextCursor != "9" {
		t.Errorf("next_cursor = %v, want \"9\" as a top-level sibling of data", env.NextCursor)
	}
	if env.Meta == nil {
		t.Fatal("meta is absent; a paged response still carries meta.took_ms")
	}

	// The id must travel as a JSON string, not a number: BIGSERIAL exceeds the
	// range a JSON number carries without loss, per ADR-021.
	if !strings.Contains(rr.Body.String(), `"id":"7"`) {
		t.Errorf("event id is not a JSON string; body=%s", rr.Body.String())
	}
}

// An exhausted page omits next_cursor rather than sending it as null, so the SDK
// pages until the field is absent, per ADR-021.
func TestListEventsOmitsNextCursorWhenExhausted(t *testing.T) {
	t.Parallel()

	srv := NewServer(nil, fakeContracts{}, fakeEvents{queryFn: func(_ context.Context, _ store.EventQuery) (store.EventsPage, error) {
		return store.EventsPage{Events: []*models.Event{sampleEvent(1, 10, "transfer")}}, nil
	}}, "1.2.3")

	rr := serve(srv, http.MethodGet, "/contracts/"+validContractID+"/events", "")

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(rr.Body.Bytes(), &raw); err != nil {
		t.Fatalf("body is not a JSON object: %v", err)
	}
	if _, ok := raw["next_cursor"]; ok {
		t.Error("next_cursor is present on an exhausted page; it must be absent")
	}
}

// A tracked contract with no matching events is a 200 with an empty items array,
// never null: "no matches" is a valid empty page, distinct from an untracked
// contract's 404. The store hands back a zero page whose Events slice is nil, so
// this pins that the handler coerces it to [].
func TestListEventsReturnsAnEmptyArrayNotNull(t *testing.T) {
	t.Parallel()

	srv := NewServer(nil, fakeContracts{}, fakeEvents{queryFn: func(_ context.Context, _ store.EventQuery) (store.EventsPage, error) {
		return store.EventsPage{}, nil
	}}, "1.2.3")

	rr := serve(srv, http.MethodGet, "/contracts/"+validContractID+"/events", "")

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}

	var raw struct {
		Data struct {
			Items json.RawMessage `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &raw); err != nil {
		t.Fatalf("body is not JSON: %v; body=%s", err, rr.Body.String())
	}
	if got := string(raw.Data.Items); got != "[]" {
		t.Errorf("data.items = %s, want []; no matches must not serialize as null", got)
	}
}

// Every query parameter reaches the store on the built EventQuery, so the wire
// contract the SDK sends is threaded through intact.
func TestListEventsThreadsQueryParamsToTheStore(t *testing.T) {
	t.Parallel()

	var got store.EventQuery
	srv := NewServer(nil, fakeContracts{}, fakeEvents{queryFn: func(_ context.Context, q store.EventQuery) (store.EventsPage, error) {
		got = q
		return store.EventsPage{Events: []*models.Event{}}, nil
	}}, "1.2.3")

	target := "/contracts/" + validContractID +
		"/events?name=transfer&from_ledger=10&to_ledger=20&topic_contains=GABC&limit=25&cursor=42&order=asc"
	rr := serve(srv, http.MethodGet, target, "")
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}

	want := store.EventQuery{
		ContractID:    validContractID,
		Name:          "transfer",
		FromLedger:    10,
		ToLedger:      20,
		Limit:         25,
		Cursor:        "42",
		Order:         "asc",
		TopicContains: "GABC",
	}
	if got != want {
		t.Errorf("store saw query %+v, want %+v", got, want)
	}
}

// With no query parameters the handler defaults limit to 50 and order to desc,
// matching the SDK's EventQuerySchema, even though the store's own default order
// is ascending. See ADR-041.
func TestListEventsAppliesTheSDKDefaults(t *testing.T) {
	t.Parallel()

	var got store.EventQuery
	srv := NewServer(nil, fakeContracts{}, fakeEvents{queryFn: func(_ context.Context, q store.EventQuery) (store.EventsPage, error) {
		got = q
		return store.EventsPage{Events: []*models.Event{}}, nil
	}}, "1.2.3")

	rr := serve(srv, http.MethodGet, "/contracts/"+validContractID+"/events", "")
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

// A query against a contract the indexer is not tracking is a 404
// NOT_FOUND_CONTRACT, not an empty page, per ADR-021, and the events store is
// never read.
func TestListEventsReturns404WhenContractUntracked(t *testing.T) {
	t.Parallel()

	srv := NewServer(nil, fakeContracts{getFn: func(_ context.Context, id string) (models.Contract, error) {
		return models.Contract{}, fmt.Errorf("store: contract %s: %w", id, store.ErrNotFound)
	}}, fakeEvents{queryFn: func(context.Context, store.EventQuery) (store.EventsPage, error) {
		t.Error("Query reached the store for an untracked contract")
		return store.EventsPage{}, nil
	}}, "1.2.3")

	rr := serve(srv, http.MethodGet, "/contracts/"+validContractID+"/events", "")

	assertNotFoundContract(t, rr)
}

// A malformed contract id is rejected at the boundary, before either store is
// touched.
func TestListEventsRejectsAMalformedContractID(t *testing.T) {
	t.Parallel()

	srv := NewServer(nil, fakeContracts{getFn: func(context.Context, string) (models.Contract, error) {
		t.Error("Get reached the store despite a malformed contract id")
		return models.Contract{}, nil
	}}, fakeEvents{queryFn: func(context.Context, store.EventQuery) (store.EventsPage, error) {
		t.Error("Query reached the store despite a malformed contract id")
		return store.EventsPage{}, nil
	}}, "1.2.3")

	rr := serve(srv, http.MethodGet, "/contracts/not-a-contract-id/events", "")

	assertValidationContractID(t, rr)
}

// Every invalid query parameter is a catalogued 400 naming the field that
// failed, and validation precedes both store reads: the hooks fail the test if
// either is reached.
func TestListEventsRejectsInvalidQueryParams(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		query      string
		wantDetail apierror.Code
	}{
		{"limit below the minimum", "limit=0", apierror.CodeValidationLimit},
		{"limit above the maximum", "limit=501", apierror.CodeValidationLimit},
		{"limit is not a number", "limit=abc", apierror.CodeValidationLimit},
		{"order is another word", "order=up", apierror.CodeValidationOrder},
		{"order is the wrong case", "order=DESC", apierror.CodeValidationOrder},
		{"cursor is not digits", "cursor=abc", apierror.CodeValidationCursor},
		{"cursor is negative", "cursor=-5", apierror.CodeValidationCursor},
		{"from_ledger is negative", "from_ledger=-1", apierror.CodeValidationLedgerRange},
		{"from_ledger is not a number", "from_ledger=abc", apierror.CodeValidationLedgerRange},
		{"to_ledger is not a number", "to_ledger=nope", apierror.CodeValidationLedgerRange},
		{"inverted ledger window", "from_ledger=100&to_ledger=50", apierror.CodeValidationLedgerRange},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			srv := NewServer(nil, fakeContracts{getFn: func(context.Context, string) (models.Contract, error) {
				t.Error("Get reached the store despite an invalid query")
				return models.Contract{}, nil
			}}, fakeEvents{queryFn: func(context.Context, store.EventQuery) (store.EventsPage, error) {
				t.Error("Query reached the store despite an invalid query")
				return store.EventsPage{}, nil
			}}, "1.2.3")

			rr := serve(srv, http.MethodGet, "/contracts/"+validContractID+"/events?"+tt.query, "")

			assertValidationDetail(t, rr, tt.wantDetail)
		})
	}
}

func TestListEventsReturns500WhenTheEventsStoreFails(t *testing.T) {
	t.Parallel()

	log, buf := newTestLogger()
	const secret = "query timeout against replica 10.2.3.4"
	srv := NewServer(log, fakeContracts{}, fakeEvents{queryFn: func(context.Context, store.EventQuery) (store.EventsPage, error) {
		return store.EventsPage{}, errorString(secret)
	}}, "1.2.3")

	rr := serve(srv, http.MethodGet, "/contracts/"+validContractID+"/events", "")

	assertInternalStoreError(t, rr, buf, "events_query_failed", secret)
}

// A tracked-check failure that is not a plain absence is a 500, with the cause
// logged and kept out of the response, and the events store is never read.
func TestListEventsReturns500WhenTheTrackedCheckFails(t *testing.T) {
	t.Parallel()

	log, buf := newTestLogger()
	const secret = "connection to 10.9.9.9 refused"
	srv := NewServer(log, fakeContracts{getFn: func(context.Context, string) (models.Contract, error) {
		return models.Contract{}, errorString(secret)
	}}, fakeEvents{queryFn: func(context.Context, store.EventQuery) (store.EventsPage, error) {
		t.Error("Query reached the store despite the tracked check failing")
		return store.EventsPage{}, nil
	}}, "1.2.3")

	rr := serve(srv, http.MethodGet, "/contracts/"+validContractID+"/events", "")

	assertInternalStoreError(t, rr, buf, "events_contract_get_failed", secret)
}

// GET /events/{id}

func TestGetEventReturnsTheEvent(t *testing.T) {
	t.Parallel()

	var gotID int64
	srv := NewServer(nil, fakeContracts{}, fakeEvents{getFn: func(_ context.Context, id int64) (*models.Event, error) {
		gotID = id
		return sampleEvent(42, 100, "transfer"), nil
	}}, "1.2.3")

	rr := serve(srv, http.MethodGet, "/events/42", "")

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	assertJSONContentType(t, rr)
	if gotID != 42 {
		t.Errorf("store saw id %d, want 42", gotID)
	}

	// The event sits directly under data, not wrapped in an items list.
	var env struct {
		Data struct {
			ID         string `json:"id"`
			ContractID string `json:"contract_id"`
			Name       string `json:"name"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &env); err != nil {
		t.Fatalf("body is not JSON: %v; body=%s", err, rr.Body.String())
	}
	if env.Data.ID != "42" {
		t.Errorf("data.id = %q, want \"42\"", env.Data.ID)
	}
	if env.Data.Name != "transfer" {
		t.Errorf("data.name = %q, want transfer", env.Data.Name)
	}
	if strings.Contains(rr.Body.String(), `"items"`) {
		t.Errorf("single-event response carries items; the event must sit directly in data; body=%s", rr.Body.String())
	}
}

func TestGetEventReturns404WhenAbsent(t *testing.T) {
	t.Parallel()

	srv := NewServer(nil, fakeContracts{}, fakeEvents{getFn: func(_ context.Context, id int64) (*models.Event, error) {
		return nil, fmt.Errorf("store: event %d: %w", id, store.ErrNotFound)
	}}, "1.2.3")

	rr := serve(srv, http.MethodGet, "/events/999", "")

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
	if env.Error.Details.Code != string(apierror.CodeNotFoundEvent) {
		t.Errorf("error.details.code = %q, want %q", env.Error.Details.Code, apierror.CodeNotFoundEvent)
	}
}

// A malformed id is a 400 VALIDATION_EVENT_ID, distinct from a well-formed id
// that finds nothing, and the store is never read.
func TestGetEventRejectsAMalformedID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		id   string
	}{
		{"not a number", "abc"},
		{"negative", "-5"},
		{"decimal", "1.5"},
		{"overflows an int64", strings.Repeat("9", 40)},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			srv := NewServer(nil, fakeContracts{}, fakeEvents{getFn: func(context.Context, int64) (*models.Event, error) {
				t.Error("Get reached the store despite a malformed id")
				return nil, nil
			}}, "1.2.3")

			rr := serve(srv, http.MethodGet, "/events/"+tt.id, "")

			assertValidationDetail(t, rr, apierror.CodeValidationEventID)
		})
	}
}

func TestGetEventReturns500WhenTheStoreFails(t *testing.T) {
	t.Parallel()

	log, buf := newTestLogger()
	const secret = "disk i/o error on shard 3"
	srv := NewServer(log, fakeContracts{}, fakeEvents{getFn: func(context.Context, int64) (*models.Event, error) {
		return nil, errorString(secret)
	}}, "1.2.3")

	rr := serve(srv, http.MethodGet, "/events/42", "")

	assertInternalStoreError(t, rr, buf, "event_get_failed", secret)
}
