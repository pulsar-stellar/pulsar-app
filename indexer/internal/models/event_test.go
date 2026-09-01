package models_test

import (
	"context"
	"encoding/json"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/pulsar-stellar/pulsar-app/indexer/internal/db"
	"github.com/pulsar-stellar/pulsar-app/indexer/internal/models"
)

func sampleEvent() models.Event {
	return models.Event{
		ID:                       9007199254740993, // 2^53 + 1, past JSON number precision
		ContractID:               showcase,
		Ledger:                   4430000,
		TxHash:                   "9f4067a379ac4febdbce3bc061776b4b5c770874216ae6619848ed8a6625e0ea",
		EventIndex:               5,
		Name:                     "transfer",
		TopicsJSON:               json.RawMessage(`[{"type":"symbol","value":"transfer"}]`),
		DataJSON:                 json.RawMessage(`{"type":"i128","value":"100"}`),
		RawTopics:                []string{"AAAADwAAAANmZWUA"},
		RawData:                  "AAAACgAAAAAAAAAAAAAAAAAAAGQ=",
		EmittedAt:                time.Date(2026, 8, 31, 10, 53, 7, 0, time.UTC),
		InSuccessfulContractCall: true,
	}
}

// The struct is compared against the schema the migrations actually produce,
// read back from the migrated database rather than parsed out of a CREATE
// TABLE. 0003 adds its column with ALTER TABLE, so parsing 0001 alone would
// miss it and the comparison would be wrong in the reassuring direction.
func TestEventTagsMatchTheMigratedTable(t *testing.T) {
	t.Parallel()

	driver, err := db.Resolve(db.Options{DriverName: "sqlite"})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	handle, err := db.Open(driver, db.ConnOptions{DSN: "file::memory:"})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = handle.Close() }()

	if _, err := db.Up(context.Background(), handle, driver); err != nil {
		t.Fatalf("Up: %v", err)
	}

	rows, err := handle.Query("SELECT name FROM pragma_table_info('events')")
	if err != nil {
		t.Fatalf("reading the events schema: %v", err)
	}
	defer func() { _ = rows.Close() }()

	var columns []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scanning a column name: %v", err)
		}
		columns = append(columns, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("reading the events schema: %v", err)
	}
	if len(columns) == 0 {
		t.Fatal("the events table reported no columns")
	}

	tags := dbTags(models.Event{})
	sort.Strings(columns)
	sort.Strings(tags)
	if !reflect.DeepEqual(columns, tags) {
		t.Errorf("events columns and Event db tags differ.\n  columns: %v\n  tags:    %v", columns, tags)
	}
}

// ADR-021: the id is a string of digits on the wire because BIGSERIAL outgrows
// a JSON number. This is the opposite of Contract.ID, which is already a
// string and must not carry the tag.
func TestEventIDMarshalsAsAStringOfDigits(t *testing.T) {
	t.Parallel()

	encoded, err := json.Marshal(sampleEvent())
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var generic map[string]any
	if err := json.Unmarshal(encoded, &generic); err != nil {
		t.Fatalf("decoding: %v", err)
	}

	id, ok := generic["id"].(string)
	if !ok {
		t.Fatalf("id is %T, want a string; without the ,string tag it would be a number", generic["id"])
	}
	if id != "9007199254740993" {
		t.Errorf("id = %q, want the exact int64", id)
	}
	if !regexp.MustCompile(`^\d+$`).MatchString(id) {
		t.Errorf("id = %q does not match the SDK's EventIdSchema pattern", id)
	}

	// The value survives a JSON number's precision limit, which is the reason
	// for the tag. As a float64 it would come back as ...92.
	if strings.Contains(string(encoded), "9007199254740992") {
		t.Errorf("the id lost precision: %s", encoded)
	}

	// Ledger and event_index stay numbers.
	if _, ok := generic["ledger"].(float64); !ok {
		t.Errorf("ledger is %T, want a JSON number", generic["ledger"])
	}
	if _, ok := generic["event_index"].(float64); !ok {
		t.Errorf("event_index is %T, want a JSON number", generic["event_index"])
	}
}

func TestEventWireKeysAreSnakeCase(t *testing.T) {
	t.Parallel()

	encoded, err := json.Marshal(sampleEvent())
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var generic map[string]any
	if err := json.Unmarshal(encoded, &generic); err != nil {
		t.Fatalf("decoding: %v", err)
	}

	want := []string{
		"contract_id", "data_json", "emitted_at", "event_index", "id",
		"in_successful_contract_call", "ledger", "name", "raw_data",
		"raw_topics", "topics_json", "tx_hash",
	}
	var got []string
	for k := range generic {
		got = append(got, k)
	}
	sort.Strings(got)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("keys = %v, want %v", got, want)
	}

	// ADR-017 fixes snake_case on the wire; the SDK maps to camelCase itself.
	snake := regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
	for _, key := range got {
		if !snake.MatchString(key) {
			t.Errorf("key %q is not snake_case", key)
		}
	}
}

func TestEventRoundTrips(t *testing.T) {
	t.Parallel()

	original := sampleEvent()
	encoded, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var decoded models.Event
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if decoded.ID != original.ID {
		t.Errorf("ID = %d, want %d; the string tag must survive a round trip", decoded.ID, original.ID)
	}
	if decoded.ContractID != original.ContractID || decoded.TxHash != original.TxHash ||
		decoded.Name != original.Name || decoded.RawData != original.RawData {
		t.Errorf("round trip lost strings: %+v", decoded)
	}
	if decoded.Ledger != original.Ledger || decoded.EventIndex != original.EventIndex {
		t.Errorf("round trip lost ordinals: %+v", decoded)
	}
	if decoded.InSuccessfulContractCall != original.InSuccessfulContractCall {
		t.Error("round trip lost the success flag")
	}
	if !decoded.EmittedAt.Equal(original.EmittedAt) {
		t.Errorf("EmittedAt = %v, want %v", decoded.EmittedAt, original.EmittedAt)
	}
	if !reflect.DeepEqual(decoded.RawTopics, original.RawTopics) {
		t.Errorf("RawTopics = %v, want %v", decoded.RawTopics, original.RawTopics)
	}
}

// json.RawMessage passes its bytes through as JSON. A []byte in the same place
// would marshal to base64, putting a blob on the wire where the SDK expects
// the ADR-023 taxonomy.
func TestDecodedColumnsStayJSONRatherThanBase64(t *testing.T) {
	t.Parallel()

	encoded, err := json.Marshal(sampleEvent())
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var generic map[string]any
	if err := json.Unmarshal(encoded, &generic); err != nil {
		t.Fatalf("decoding: %v", err)
	}

	topics, ok := generic["topics_json"].([]any)
	if !ok {
		t.Fatalf("topics_json is %T, want an array; a []byte field would encode it as a base64 string",
			generic["topics_json"])
	}
	if len(topics) != 1 {
		t.Fatalf("topics_json has %d entries, want 1", len(topics))
	}
	first, ok := topics[0].(map[string]any)
	if !ok || first["type"] != "symbol" || first["value"] != "transfer" {
		t.Errorf("topics_json[0] = %v, want the decoded taxonomy value", topics[0])
	}

	if _, ok := generic["data_json"].(map[string]any); !ok {
		t.Fatalf("data_json is %T, want an object", generic["data_json"])
	}

	// Whatever valid JSON the column holds passes through unchanged.
	nested := sampleEvent()
	nested.DataJSON = json.RawMessage(`{"type":"map","value":[{"key":{"type":"symbol","value":"a"}}]}`)
	out, err := json.Marshal(nested)
	if err != nil {
		t.Fatalf("Marshal with a nested value: %v", err)
	}
	if !strings.Contains(string(out), `"value":[{"key":`) {
		t.Errorf("a nested decoded value was altered on the way out: %s", out)
	}
}

// ADR-026. A reverted call still emits events that land in the ledger, so the
// flag is the only thing separating them, and false must serialize explicitly
// rather than being omitted.
func TestSuccessFlagSerializesBothWays(t *testing.T) {
	t.Parallel()

	reverted := sampleEvent()
	reverted.InSuccessfulContractCall = false

	encoded, err := json.Marshal(reverted)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if !strings.Contains(string(encoded), `"in_successful_contract_call":false`) {
		t.Errorf("a reverted event encoded as %s, want an explicit false", encoded)
	}

	committed, err := json.Marshal(sampleEvent())
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if !strings.Contains(string(committed), `"in_successful_contract_call":true`) {
		t.Errorf("a committed event encoded as %s, want an explicit true", committed)
	}
}

func TestEmittedAtMarshalsAsRFC3339(t *testing.T) {
	t.Parallel()

	encoded, err := json.Marshal(sampleEvent())
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var generic map[string]any
	if err := json.Unmarshal(encoded, &generic); err != nil {
		t.Fatalf("decoding: %v", err)
	}

	emittedAt, ok := generic["emitted_at"].(string)
	if !ok {
		t.Fatalf("emitted_at is %T, want a string", generic["emitted_at"])
	}
	if _, err := time.Parse(time.RFC3339, emittedAt); err != nil {
		t.Errorf("emitted_at %q does not parse as RFC3339: %v", emittedAt, err)
	}
	if !strings.HasSuffix(emittedAt, "Z") && !regexp.MustCompile(`[+-]\d{2}:\d{2}$`).MatchString(emittedAt) {
		t.Errorf("emitted_at %q carries no UTC offset, which the SDK requires", emittedAt)
	}
}

// An empty name is a real state, not a failure: an emitter whose first topic is
// not a Symbol produces a nameless event with its topics intact, per ADR-026.
func TestNamedReportsWhetherTheFirstTopicWasASymbol(t *testing.T) {
	t.Parallel()

	named := sampleEvent()
	if !named.Named() {
		t.Error("Named() reported false for an event with a name")
	}

	nameless := sampleEvent()
	nameless.Name = ""
	if nameless.Named() {
		t.Error("Named() reported true for a nameless event")
	}

	encoded, err := json.Marshal(nameless)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if !strings.Contains(string(encoded), `"name":""`) {
		t.Errorf("a nameless event encoded as %s, want an explicit empty name", encoded)
	}
}
