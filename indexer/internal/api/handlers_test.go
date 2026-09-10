package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pulsar-stellar/pulsar-app/indexer/internal/apierror"
	"github.com/pulsar-stellar/pulsar-app/indexer/internal/store"
)

// fakeContracts is a contractStatsReader that returns whatever a test sets,
// so a handler test exercises the HTTP contract without a database.
type fakeContracts struct {
	stats store.ContractStats
	err   error
}

func (f fakeContracts) Stats(context.Context) (store.ContractStats, error) {
	return f.stats, f.err
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
