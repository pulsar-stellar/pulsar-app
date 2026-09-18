package api

import (
	"bytes"
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

// validContractID is a well-formed Soroban contract ID the handler tests reuse.
// It is pulsar-core's deployed showcase contract, a real ID.
const validContractID = "CDNWTVUDKCCGW7GOC6SBLUFXXUCD2YDHWRDUSXZ6CYBQKQWLCUYYWI5L"

// fakeContracts is a contractStore that returns whatever a test sets, so a
// handler test exercises the HTTP contract without a database. Stats stays
// backed by the stats/err fields the /health tests already use; the CRUD
// methods are backed by optional function fields, so a test both controls the
// result and captures the argument the handler passed. An unset function field
// returns a zero result, which keeps the /health tests, which set none of them,
// compiling and passing unchanged.
type fakeContracts struct {
	stats store.ContractStats
	err   error

	listFn     func(ctx context.Context) ([]models.Contract, error)
	registerFn func(ctx context.Context, id string) (models.Contract, error)
	getFn      func(ctx context.Context, id string) (models.Contract, error)
	deleteFn   func(ctx context.Context, id string) error
}

func (f fakeContracts) Stats(context.Context) (store.ContractStats, error) {
	return f.stats, f.err
}

func (f fakeContracts) List(ctx context.Context) ([]models.Contract, error) {
	if f.listFn != nil {
		return f.listFn(ctx)
	}
	return nil, nil
}

func (f fakeContracts) Register(ctx context.Context, id string) (models.Contract, error) {
	if f.registerFn != nil {
		return f.registerFn(ctx, id)
	}
	return models.Contract{}, nil
}

func (f fakeContracts) Get(ctx context.Context, id string) (models.Contract, error) {
	if f.getFn != nil {
		return f.getFn(ctx, id)
	}
	return models.Contract{}, nil
}

func (f fakeContracts) Delete(ctx context.Context, id string) error {
	if f.deleteFn != nil {
		return f.deleteFn(ctx, id)
	}
	return nil
}

// serve routes a request through the full mux, middleware included, so a
// handler test exercises routing and URL-parameter extraction the way a real
// request does. An empty body sends no body at all.
func serve(srv *Server, method, target, body string) *httptest.ResponseRecorder {
	var r *http.Request
	if body == "" {
		r = httptest.NewRequest(method, target, nil)
	} else {
		r = httptest.NewRequest(method, target, strings.NewReader(body))
	}
	rr := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rr, r)
	return rr
}

// decodedHealth mirrors the ADR-017 success envelope for /health: the payload
// under data, the indexer's timing under meta. next_cursor is decoded as a
// present/absent probe, since ADR-017 says it is absent on a non-paginated
// route rather than null.
type decodedHealth struct {
	Data struct {
		OK               bool   `json:"ok"`
		Version          string `json:"version"`
		LatestLedger     int64  `json:"latest_ledger"`
		TrackedContracts int    `json:"tracked_contracts"`
	} `json:"data"`
	Meta *struct {
		TookMs float64 `json:"took_ms"`
	} `json:"meta"`
}

func TestHealthReturnsTheStatusEnvelope(t *testing.T) {
	t.Parallel()

	srv := NewServer(nil, fakeContracts{stats: store.ContractStats{Count: 3, LatestLedger: 12345}}, "1.2.3")
	rr := httptest.NewRecorder()
	srv.handleHealth(rr, httptest.NewRequest(http.MethodGet, "/health", nil))

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
	assertJSONContentType(t, rr)

	var env decodedHealth
	if err := json.Unmarshal(rr.Body.Bytes(), &env); err != nil {
		t.Fatalf("body is not JSON: %v; body=%s", err, rr.Body.String())
	}
	if !env.Data.OK {
		t.Error("data.ok = false, want true on a healthy response")
	}
	if env.Data.Version != "1.2.3" {
		t.Errorf("data.version = %q, want 1.2.3", env.Data.Version)
	}
	if env.Data.LatestLedger != 12345 {
		t.Errorf("data.latest_ledger = %d, want 12345", env.Data.LatestLedger)
	}
	if env.Data.TrackedContracts != 3 {
		t.Errorf("data.tracked_contracts = %d, want 3", env.Data.TrackedContracts)
	}
	if env.Meta == nil {
		t.Fatal("meta is absent; ADR-017's health example carries meta.took_ms")
	}
	if env.Meta.TookMs < 0 {
		t.Errorf("meta.took_ms = %v, want >= 0 (the SDK rejects a negative)", env.Meta.TookMs)
	}

	// next_cursor must be absent, not null: it appears only on paginated routes.
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(rr.Body.Bytes(), &raw); err != nil {
		t.Fatalf("body is not a JSON object: %v", err)
	}
	if _, ok := raw["next_cursor"]; ok {
		t.Error("next_cursor is present on /health; it must be absent on a non-paginated route")
	}
}

// A missing version must never reach the wire: the SDK's HealthPayloadSchema
// requires a non-empty string, so NewServer substitutes "dev".
func TestHealthVersionFallsBackToDev(t *testing.T) {
	t.Parallel()

	srv := NewServer(nil, fakeContracts{}, "")
	rr := httptest.NewRecorder()
	srv.handleHealth(rr, httptest.NewRequest(http.MethodGet, "/health", nil))

	var env decodedHealth
	if err := json.Unmarshal(rr.Body.Bytes(), &env); err != nil {
		t.Fatalf("body is not JSON: %v", err)
	}
	if env.Data.Version != "dev" {
		t.Errorf("data.version = %q, want dev for an unstamped build", env.Data.Version)
	}
}

// When the store read fails the indexer cannot report its state, so /health
// returns a 500 internal envelope carrying INTERNAL_STORE. The failure is
// logged server-side and the underlying error must never reach the client.
func TestHealthReturns500WhenTheStoreFails(t *testing.T) {
	t.Parallel()

	log, buf := newTestLogger()
	const secret = "connection to 10.0.0.5 refused: password rejected"
	srv := NewServer(log, fakeContracts{err: errorString(secret)}, "1.2.3")
	rr := httptest.NewRecorder()
	srv.handleHealth(rr, httptest.NewRequest(http.MethodGet, "/health", nil))

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rr.Code)
	}
	assertJSONContentType(t, rr)

	var env decodedError
	if err := json.Unmarshal(rr.Body.Bytes(), &env); err != nil {
		t.Fatalf("body is not JSON: %v", err)
	}
	if env.Error.Code != "internal" {
		t.Errorf("error.code = %q, want internal", env.Error.Code)
	}
	if env.Error.Details.Code != string(apierror.CodeInternalStore) {
		t.Errorf("error.details.code = %q, want %q", env.Error.Details.Code, apierror.CodeInternalStore)
	}
	if strings.Contains(rr.Body.String(), "10.0.0.5") || strings.Contains(rr.Body.String(), "password") {
		t.Errorf("the underlying error leaked to the client: %s", rr.Body.String())
	}

	// The detail belongs in the server-side log, not the response.
	line := findLine(t, buf, "health_stats_failed")
	if !strings.Contains(line["error"].(string), secret) {
		t.Errorf("server log did not capture the underlying error; got %v", line["error"])
	}
}

// TestWriteDataRendersTheSuccessEnvelope pins the ADR-017 success shape writeData
// produces, independent of any handler: data wraps the payload, meta.took_ms
// carries the timing, and next_cursor stays absent.
func TestWriteDataRendersTheSuccessEnvelope(t *testing.T) {
	t.Parallel()

	rr := httptest.NewRecorder()
	writeData(rr, http.StatusOK, map[string]int{"n": 7}, 1.5)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
	assertJSONContentType(t, rr)

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(rr.Body.Bytes(), &raw); err != nil {
		t.Fatalf("body is not a JSON object: %v", err)
	}
	if _, ok := raw["data"]; !ok {
		t.Error("data key is absent")
	}
	if _, ok := raw["meta"]; !ok {
		t.Error("meta key is absent")
	}
	if _, ok := raw["next_cursor"]; ok {
		t.Error("next_cursor is present; writeData must omit it")
	}

	var env struct {
		Meta struct {
			TookMs float64 `json:"took_ms"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &env); err != nil {
		t.Fatalf("body is not JSON: %v", err)
	}
	if env.Meta.TookMs != 1.5 {
		t.Errorf("meta.took_ms = %v, want 1.5", env.Meta.TookMs)
	}
}

// errorString is a minimal error whose text a test controls, used to prove the
// message is logged but never sent to the client.
type errorString string

func (e errorString) Error() string { return string(e) }

// int64Ptr returns a pointer to n, for building a Contract whose nullable
// first_indexed_ledger is set.
func int64Ptr(n int64) *int64 { return &n }

// decodedContract mirrors the ADR-017 success envelope for a single contract:
// the record sits under data. Register and get both answer this shape, so a
// test decodes straight into models.Contract through its json tags.
type decodedContract struct {
	Data models.Contract `json:"data"`
}

// assertInternalStoreError asserts rr is the 500 INTERNAL_STORE envelope a
// store failure produces, that the underlying error never reached the client,
// and that it did reach the server log under wantMsg. It is the shared shape of
// every handler's store-failure path.
func assertInternalStoreError(t *testing.T, rr *httptest.ResponseRecorder, buf *bytes.Buffer, wantMsg, secret string) {
	t.Helper()

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rr.Code)
	}
	assertJSONContentType(t, rr)

	var env decodedError
	if err := json.Unmarshal(rr.Body.Bytes(), &env); err != nil {
		t.Fatalf("body is not JSON: %v; body=%s", err, rr.Body.String())
	}
	if env.Error.Code != "internal" {
		t.Errorf("error.code = %q, want internal", env.Error.Code)
	}
	if env.Error.Details.Code != string(apierror.CodeInternalStore) {
		t.Errorf("error.details.code = %q, want %q", env.Error.Details.Code, apierror.CodeInternalStore)
	}
	if strings.Contains(rr.Body.String(), secret) {
		t.Errorf("the underlying error leaked to the client: %s", rr.Body.String())
	}

	line := findLine(t, buf, wantMsg)
	if !strings.Contains(line["error"].(string), secret) {
		t.Errorf("server log did not capture the underlying error; got %v", line["error"])
	}
}

// assertNotFoundContract asserts rr is the 404 NOT_FOUND_CONTRACT envelope GET
// and DELETE return for a contract the indexer is not tracking, the structured
// absence ADR-019 requires.
func assertNotFoundContract(t *testing.T, rr *httptest.ResponseRecorder) {
	t.Helper()

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
	if env.Error.Details.Code != string(apierror.CodeNotFoundContract) {
		t.Errorf("error.details.code = %q, want %q", env.Error.Details.Code, apierror.CodeNotFoundContract)
	}
}

// assertValidationContractID asserts rr is the 400 VALIDATION_CONTRACT_ID
// envelope a malformed contract ID produces at the HTTP boundary.
func assertValidationContractID(t *testing.T, rr *httptest.ResponseRecorder) {
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
	if env.Error.Details.Code != string(apierror.CodeValidationContractID) {
		t.Errorf("error.details.code = %q, want %q", env.Error.Details.Code, apierror.CodeValidationContractID)
	}
}

// GET /contracts

func TestListContractsReturnsTheTrackedContracts(t *testing.T) {
	t.Parallel()

	secondID := "C" + strings.Repeat("A", 55)
	want := []models.Contract{
		{ID: validContractID, Status: models.StatusActive, LastIndexedLedger: 100},
		{ID: secondID, Status: models.StatusPaused},
	}
	srv := NewServer(nil, fakeContracts{listFn: func(context.Context) ([]models.Contract, error) {
		return want, nil
	}}, "1.2.3")

	rr := serve(srv, http.MethodGet, "/contracts", "")

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
	assertJSONContentType(t, rr)

	var env struct {
		Data struct {
			Items []models.Contract `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &env); err != nil {
		t.Fatalf("body is not JSON: %v; body=%s", err, rr.Body.String())
	}
	if len(env.Data.Items) != len(want) {
		t.Fatalf("data.items has %d records, want %d", len(env.Data.Items), len(want))
	}
	for i, c := range env.Data.Items {
		if c.ID != want[i].ID {
			t.Errorf("data.items[%d].id = %q, want %q", i, c.ID, want[i].ID)
		}
		if c.Status != want[i].Status {
			t.Errorf("data.items[%d].status = %q, want %q", i, c.Status, want[i].Status)
		}
	}
}

// An empty table must serialize as [], never null: the SDK's listContracts
// reads data.items as an array and would reject a null.
func TestListContractsReturnsAnEmptyArrayNotNull(t *testing.T) {
	t.Parallel()

	srv := NewServer(nil, fakeContracts{listFn: func(context.Context) ([]models.Contract, error) {
		return nil, nil // the store hands back a nil slice for an empty table
	}}, "1.2.3")

	rr := serve(srv, http.MethodGet, "/contracts", "")

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
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
		t.Errorf("data.items = %s, want []; an empty table must not serialize as null", got)
	}
}

func TestListContractsReturns500WhenTheStoreFails(t *testing.T) {
	t.Parallel()

	log, buf := newTestLogger()
	const secret = "connection to 10.0.0.9 refused: password rejected"
	srv := NewServer(log, fakeContracts{listFn: func(context.Context) ([]models.Contract, error) {
		return nil, errorString(secret)
	}}, "1.2.3")

	rr := serve(srv, http.MethodGet, "/contracts", "")

	assertInternalStoreError(t, rr, buf, "contracts_list_failed", secret)
}

// POST /contracts

// A registration returns whatever the store records, at 200. The two rows cover
// a fresh insert and, per ADR-018, a re-registration returning the existing
// record with its prior progress intact rather than erroring, which is how a
// client that retries a lost response stays safe.
func TestRegisterContractReturnsTheStoredRecord(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		record models.Contract
	}{
		{
			name:   "fresh registration",
			record: models.Contract{ID: validContractID, Status: models.StatusActive},
		},
		{
			name:   "re-registration returns the existing record",
			record: models.Contract{ID: validContractID, Status: models.StatusActive, FirstIndexedLedger: int64Ptr(42), LastIndexedLedger: 100},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var gotID string
			srv := NewServer(nil, fakeContracts{registerFn: func(_ context.Context, id string) (models.Contract, error) {
				gotID = id
				return tt.record, nil
			}}, "1.2.3")

			rr := serve(srv, http.MethodPost, "/contracts", `{"contract_id":"`+validContractID+`"}`)

			if rr.Code != http.StatusOK {
				t.Errorf("status = %d, want 200", rr.Code)
			}
			assertJSONContentType(t, rr)
			if gotID != validContractID {
				t.Errorf("store saw contract_id %q, want %q", gotID, validContractID)
			}

			var env decodedContract
			if err := json.Unmarshal(rr.Body.Bytes(), &env); err != nil {
				t.Fatalf("body is not JSON: %v; body=%s", err, rr.Body.String())
			}
			if env.Data.ID != tt.record.ID {
				t.Errorf("data.id = %q, want %q", env.Data.ID, tt.record.ID)
			}
			if env.Data.Status != tt.record.Status {
				t.Errorf("data.status = %q, want %q", env.Data.Status, tt.record.Status)
			}
			switch {
			case tt.record.FirstIndexedLedger == nil && env.Data.FirstIndexedLedger != nil:
				t.Errorf("data.first_indexed_ledger = %v, want null", *env.Data.FirstIndexedLedger)
			case tt.record.FirstIndexedLedger != nil && env.Data.FirstIndexedLedger == nil:
				t.Errorf("data.first_indexed_ledger = null, want %d", *tt.record.FirstIndexedLedger)
			case tt.record.FirstIndexedLedger != nil && *env.Data.FirstIndexedLedger != *tt.record.FirstIndexedLedger:
				t.Errorf("data.first_indexed_ledger = %d, want %d", *env.Data.FirstIndexedLedger, *tt.record.FirstIndexedLedger)
			}
		})
	}
}

// Every invalid registration is rejected at the boundary, before the store is
// touched: a body that is missing, oversized, or not the expected JSON object
// is a VALIDATION_BODY, while a well-formed body carrying a bad ID is a
// VALIDATION_CONTRACT_ID. The store hook fails the test if any of these reaches
// it, pinning that validation precedes the write.
func TestRegisterContractRejectsInvalidInput(t *testing.T) {
	t.Parallel()

	oversized := `{"contract_id":"` + strings.Repeat("A", int(maxRegisterBodyBytes)+1) + `"}`

	tests := []struct {
		name       string
		body       string
		wantDetail apierror.Code
	}{
		{"malformed JSON", `{`, apierror.CodeValidationBody},
		{"body is not an object", `"just a string"`, apierror.CodeValidationBody},
		{"contract_id is the wrong type", `{"contract_id": 123}`, apierror.CodeValidationBody},
		{"empty body", ``, apierror.CodeValidationBody},
		{"oversized body", oversized, apierror.CodeValidationBody},
		{"missing contract_id", `{}`, apierror.CodeValidationContractID},
		{"empty contract_id", `{"contract_id":""}`, apierror.CodeValidationContractID},
		{"whitespace contract_id", `{"contract_id":"   "}`, apierror.CodeValidationContractID},
		{"lowercase contract_id", `{"contract_id":"` + strings.ToLower(validContractID) + `"}`, apierror.CodeValidationContractID},
		{"wrong length contract_id", `{"contract_id":"CABC"}`, apierror.CodeValidationContractID},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			srv := NewServer(nil, fakeContracts{registerFn: func(context.Context, string) (models.Contract, error) {
				t.Error("Register reached the store despite invalid input")
				return models.Contract{}, nil
			}}, "1.2.3")

			rr := serve(srv, http.MethodPost, "/contracts", tt.body)

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
			if env.Error.Details.Code != string(tt.wantDetail) {
				t.Errorf("error.details.code = %q, want %q", env.Error.Details.Code, tt.wantDetail)
			}
		})
	}
}

func TestRegisterContractReturns500WhenTheStoreFails(t *testing.T) {
	t.Parallel()

	log, buf := newTestLogger()
	const secret = "disk full on /var/lib/pulsar at host db-1"
	srv := NewServer(log, fakeContracts{registerFn: func(context.Context, string) (models.Contract, error) {
		return models.Contract{}, errorString(secret)
	}}, "1.2.3")

	rr := serve(srv, http.MethodPost, "/contracts", `{"contract_id":"`+validContractID+`"}`)

	assertInternalStoreError(t, rr, buf, "contract_register_failed", secret)
}

// GET /contracts/{id}

func TestGetContractReturnsTheRecord(t *testing.T) {
	t.Parallel()

	var gotID string
	want := models.Contract{ID: validContractID, Status: models.StatusActive, FirstIndexedLedger: int64Ptr(7), LastIndexedLedger: 99}
	srv := NewServer(nil, fakeContracts{getFn: func(_ context.Context, id string) (models.Contract, error) {
		gotID = id
		return want, nil
	}}, "1.2.3")

	rr := serve(srv, http.MethodGet, "/contracts/"+validContractID, "")

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
	assertJSONContentType(t, rr)
	if gotID != validContractID {
		t.Errorf("store saw id %q, want %q", gotID, validContractID)
	}

	var env decodedContract
	if err := json.Unmarshal(rr.Body.Bytes(), &env); err != nil {
		t.Fatalf("body is not JSON: %v; body=%s", err, rr.Body.String())
	}
	if env.Data.ID != want.ID {
		t.Errorf("data.id = %q, want %q", env.Data.ID, want.ID)
	}
	if env.Data.LastIndexedLedger != want.LastIndexedLedger {
		t.Errorf("data.last_indexed_ledger = %d, want %d", env.Data.LastIndexedLedger, want.LastIndexedLedger)
	}
}

func TestGetContractReturns404WhenUntracked(t *testing.T) {
	t.Parallel()

	srv := NewServer(nil, fakeContracts{getFn: func(_ context.Context, id string) (models.Contract, error) {
		return models.Contract{}, fmt.Errorf("store: contract %s: %w", id, store.ErrNotFound)
	}}, "1.2.3")

	rr := serve(srv, http.MethodGet, "/contracts/"+validContractID, "")

	assertNotFoundContract(t, rr)
}

func TestGetContractRejectsAMalformedID(t *testing.T) {
	t.Parallel()

	srv := NewServer(nil, fakeContracts{getFn: func(context.Context, string) (models.Contract, error) {
		t.Error("Get reached the store despite a malformed id")
		return models.Contract{}, nil
	}}, "1.2.3")

	rr := serve(srv, http.MethodGet, "/contracts/not-a-contract-id", "")

	assertValidationContractID(t, rr)
}

func TestGetContractReturns500WhenTheStoreFails(t *testing.T) {
	t.Parallel()

	log, buf := newTestLogger()
	const secret = "query timeout against replica 10.1.2.3"
	srv := NewServer(log, fakeContracts{getFn: func(context.Context, string) (models.Contract, error) {
		return models.Contract{}, errorString(secret)
	}}, "1.2.3")

	rr := serve(srv, http.MethodGet, "/contracts/"+validContractID, "")

	assertInternalStoreError(t, rr, buf, "contract_get_failed", secret)
}

// DELETE /contracts/{id}

func TestDeleteContractReturns204(t *testing.T) {
	t.Parallel()

	var gotID string
	srv := NewServer(nil, fakeContracts{deleteFn: func(_ context.Context, id string) error {
		gotID = id
		return nil
	}}, "1.2.3")

	rr := serve(srv, http.MethodDelete, "/contracts/"+validContractID, "")

	if rr.Code != http.StatusNoContent {
		t.Errorf("status = %d, want 204", rr.Code)
	}
	if gotID != validContractID {
		t.Errorf("store saw id %q, want %q", gotID, validContractID)
	}
	if body := rr.Body.String(); body != "" {
		t.Errorf("204 carried a body %q, want none", body)
	}
}

func TestDeleteContractReturns404WhenUntracked(t *testing.T) {
	t.Parallel()

	srv := NewServer(nil, fakeContracts{deleteFn: func(_ context.Context, id string) error {
		return fmt.Errorf("store: contract %s: %w", id, store.ErrNotFound)
	}}, "1.2.3")

	rr := serve(srv, http.MethodDelete, "/contracts/"+validContractID, "")

	assertNotFoundContract(t, rr)
}

func TestDeleteContractRejectsAMalformedID(t *testing.T) {
	t.Parallel()

	srv := NewServer(nil, fakeContracts{deleteFn: func(context.Context, string) error {
		t.Error("Delete reached the store despite a malformed id")
		return nil
	}}, "1.2.3")

	rr := serve(srv, http.MethodDelete, "/contracts/not-a-contract-id", "")

	assertValidationContractID(t, rr)
}

func TestDeleteContractReturns500WhenTheStoreFails(t *testing.T) {
	t.Parallel()

	log, buf := newTestLogger()
	const secret = "permission denied for table contracts as user pulsar_rw"
	srv := NewServer(log, fakeContracts{deleteFn: func(context.Context, string) error {
		return errorString(secret)
	}}, "1.2.3")

	rr := serve(srv, http.MethodDelete, "/contracts/"+validContractID, "")

	assertInternalStoreError(t, rr, buf, "contract_delete_failed", secret)
}
