package store_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/pulsar-stellar/pulsar-app/indexer/internal/models"
	"github.com/pulsar-stellar/pulsar-app/indexer/internal/store"
)

func eventsStore(t *testing.T) (*store.Events, *store.Contracts, *sql.DB) {
	t.Helper()
	handle := migrated(t)
	contracts := store.NewContracts(handle)
	if _, err := contracts.Register(context.Background(), showcase); err != nil {
		t.Fatalf("Register: %v", err)
	}
	return store.NewEvents(handle), contracts, handle
}

func newEvent(ledger, ordinal int64, name string) *models.Event {
	return &models.Event{
		ContractID:               showcase,
		Ledger:                   ledger,
		TxHash:                   fmt.Sprintf("hash-%d-%d", ledger, ordinal),
		EventIndex:               ordinal,
		Name:                     name,
		TopicsJSON:               json.RawMessage(`[{"type":"symbol","value":"` + name + `"}]`),
		DataJSON:                 json.RawMessage(`{"type":"i128","value":"100"}`),
		RawTopics:                []string{"AAAADwAAAANmZWUA", "AAAAEgAAAAAAAAAA"},
		RawData:                  "AAAACgAAAAAAAAAAAAAAAAAAAGQ=",
		EmittedAt:                time.Date(2026, 8, 31, 10, 53, 7, 0, time.UTC),
		InSuccessfulContractCall: true,
	}
}

func seed(t *testing.T, events *store.Events, list ...*models.Event) {
	t.Helper()
	if _, err := events.Insert(context.Background(), list); err != nil {
		t.Fatalf("Insert: %v", err)
	}
}

// --- insert path ---

func TestInsertStoresAnEventWholeAndReadsItBack(t *testing.T) {
	t.Parallel()

	events, _, _ := eventsStore(t)
	original := newEvent(4430000, 0, "transfer")

	inserted, err := events.Insert(context.Background(), []*models.Event{original})
	if err != nil {
		t.Fatalf("Insert: %v", err)
	}
	if inserted != 1 {
		t.Fatalf("Insert reported %d rows, want 1", inserted)
	}

	page, err := events.Query(context.Background(), store.EventQuery{ContractID: showcase})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(page.Events) != 1 {
		t.Fatalf("Query returned %d events, want 1", len(page.Events))
	}
	got := page.Events[0]

	if got.ID == 0 {
		t.Error("the event id is zero; the column did not autoincrement")
	}
	if got.ContractID != original.ContractID || got.TxHash != original.TxHash ||
		got.Name != original.Name || got.RawData != original.RawData {
		t.Errorf("strings did not round trip: %+v", got)
	}
	if got.Ledger != original.Ledger || got.EventIndex != original.EventIndex {
		t.Errorf("ordinals did not round trip: %+v", got)
	}
	if !got.InSuccessfulContractCall {
		t.Error("the success flag did not round trip")
	}
	if string(got.TopicsJSON) != string(original.TopicsJSON) {
		t.Errorf("TopicsJSON = %s, want %s", got.TopicsJSON, original.TopicsJSON)
	}
	if string(got.DataJSON) != string(original.DataJSON) {
		t.Errorf("DataJSON = %s, want %s", got.DataJSON, original.DataJSON)
	}
}

// raw_topics is TEXT holding a JSON array on both engines, per ADR-029, so the
// slice has to survive a marshal and unmarshal unchanged.
func TestRawTopicsRoundTripThroughStorage(t *testing.T) {
	t.Parallel()

	events, _, _ := eventsStore(t)
	ctx := context.Background()

	cases := []struct {
		name   string
		topics []string
	}{
		{"several base64 topics", []string{"AAAADwAAAANmZWUA", "AAAAEgAAAAAAAAAA", "AAAACg=="}},
		{"one topic", []string{"AAAADwAAAANmZWUA"}},
		{"no topics", []string{}},
		{"values needing quoting", []string{`a"b`, "c,d", "e{f}g", "h\\i"}},
		{"empty string among them", []string{"", "AAAA"}},
	}

	for i, c := range cases {
		event := newEvent(int64(4430000+i), 0, "transfer")
		event.RawTopics = c.topics
		seed(t, events, event)

		page, err := events.Query(ctx, store.EventQuery{
			ContractID: showcase, FromLedger: int64(4430000 + i), ToLedger: int64(4430000 + i),
		})
		if err != nil {
			t.Fatalf("%s: Query: %v", c.name, err)
		}
		if len(page.Events) != 1 {
			t.Fatalf("%s: got %d events, want 1", c.name, len(page.Events))
		}

		got := page.Events[0].RawTopics
		if len(got) != len(c.topics) {
			t.Errorf("%s: got %d topics, want %d: %q", c.name, len(got), len(c.topics), got)
			continue
		}
		for j := range c.topics {
			if got[j] != c.topics[j] {
				t.Errorf("%s: topic %d = %q, want %q", c.name, j, got[j], c.topics[j])
			}
		}
	}

	// nil is stored as an empty array, so it reads back as [] rather than null.
	nilTopics := newEvent(4440000, 0, "transfer")
	nilTopics.RawTopics = nil
	seed(t, events, nilTopics)

	page, err := events.Query(ctx, store.EventQuery{ContractID: showcase, FromLedger: 4440000})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if page.Events[0].RawTopics == nil {
		t.Error("nil topics read back as nil, which encodes as JSON null rather than []")
	}
}

// A column holding the JSON literal null decodes to an empty slice, not to a
// nil one. This package never writes null, but a row written by hand or by a
// later code path could hold one, and nil would reach the wire as JSON null
// where the SDK requires an array.
func TestRawTopicsNullDecodesToAnEmptySlice(t *testing.T) {
	t.Parallel()

	events, _, handle := eventsStore(t)
	ctx := context.Background()
	seed(t, events, newEvent(4430000, 0, "transfer"))

	if _, err := handle.Exec("UPDATE events SET raw_topics = $1", "null"); err != nil {
		t.Fatalf("writing a null raw_topics: %v", err)
	}

	page, err := events.Query(ctx, store.EventQuery{ContractID: showcase})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	got := page.Events[0].RawTopics
	if got == nil {
		t.Fatal("a null column decoded to nil, which encodes as JSON null rather than []")
	}
	if len(got) != 0 {
		t.Errorf("RawTopics = %v, want empty", got)
	}

	encoded, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if string(encoded) != "[]" {
		t.Errorf("encoded as %s, want []", encoded)
	}
}

// A column holding something that is not a JSON array is a storage problem.
// Reporting it as an event with no topics would be a claim about the ledger.
func TestMalformedRawTopicsIsAnErrorNotAnEmptySlice(t *testing.T) {
	t.Parallel()

	events, _, handle := eventsStore(t)
	ctx := context.Background()
	seed(t, events, newEvent(4430000, 0, "transfer"))

	if _, err := handle.Exec("UPDATE events SET raw_topics = $1", "not json at all"); err != nil {
		t.Fatalf("corrupting raw_topics: %v", err)
	}

	_, err := events.Query(ctx, store.EventQuery{ContractID: showcase})
	if err == nil {
		t.Fatal("Query succeeded against a malformed raw_topics column")
	}
	if !strings.Contains(err.Error(), "raw_topics") {
		t.Errorf("error %q does not name the column", err.Error())
	}
}

// ADR-032: emitted_at is bound on every insert, so this is the column the
// formatTime rule actually protects.
func TestEmittedAtSurvivesInsertAsRFC3339(t *testing.T) {
	t.Parallel()

	events, _, handle := eventsStore(t)
	want := time.Date(2026, 8, 31, 10, 53, 7, 0, time.UTC)
	seed(t, events, newEvent(4430000, 0, "transfer"))

	var stored string
	if err := handle.QueryRow("SELECT emitted_at FROM events").Scan(&stored); err != nil {
		t.Fatalf("reading the raw column: %v", err)
	}
	if _, err := time.Parse(time.RFC3339, stored); err != nil {
		t.Errorf("emitted_at was stored as %q, which is not RFC3339: %v", stored, err)
	}
	if strings.Contains(stored, " ") {
		t.Errorf("emitted_at was stored as %q, which looks like Go's String() output", stored)
	}

	page, err := events.Query(context.Background(), store.EventQuery{ContractID: showcase})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	got := page.Events[0].EmittedAt
	if !got.Equal(want) {
		t.Errorf("EmittedAt = %v, want %v", got, want)
	}
	if got.Location() != time.UTC {
		t.Errorf("EmittedAt is in %v, want UTC", got.Location())
	}
}

func TestInsertWritesABatchInOneStatement(t *testing.T) {
	t.Parallel()

	events, _, _ := eventsStore(t)
	batch := make([]*models.Event, 0, 250)
	for i := 0; i < 250; i++ {
		batch = append(batch, newEvent(4430000, int64(i), "transfer"))
	}

	inserted, err := events.Insert(context.Background(), batch)
	if err != nil {
		t.Fatalf("Insert: %v", err)
	}
	if inserted != 250 {
		t.Errorf("Insert reported %d rows, want 250", inserted)
	}

	page, err := events.Query(context.Background(), store.EventQuery{ContractID: showcase, Limit: 300})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(page.Events) != 250 {
		t.Errorf("Query returned %d events, want 250", len(page.Events))
	}
}

// A poll that overlaps the previous one is the normal case at a ledger
// boundary, so a repeated event must not abort the batch.
func TestInsertSkipsEventsAlreadyStored(t *testing.T) {
	t.Parallel()

	events, _, _ := eventsStore(t)
	ctx := context.Background()

	first := []*models.Event{newEvent(4430000, 0, "transfer"), newEvent(4430000, 1, "mint")}
	if _, err := events.Insert(ctx, first); err != nil {
		t.Fatalf("first Insert: %v", err)
	}

	overlapping := []*models.Event{
		newEvent(4430000, 1, "mint"),
		newEvent(4430000, 2, "burn"),
	}
	inserted, err := events.Insert(ctx, overlapping)
	if err != nil {
		t.Fatalf("overlapping Insert: %v", err)
	}
	if inserted != 1 {
		t.Errorf("Insert reported %d new rows, want 1", inserted)
	}

	page, err := events.Query(ctx, store.EventQuery{ContractID: showcase})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(page.Events) != 3 {
		t.Errorf("stored %d events, want 3", len(page.Events))
	}
}

func TestInsertValidatesItsBatch(t *testing.T) {
	t.Parallel()

	events, _, _ := eventsStore(t)
	ctx := context.Background()

	if n, err := events.Insert(ctx, nil); err != nil || n != 0 {
		t.Errorf("Insert(nil) = %d, %v; want 0 and no error", n, err)
	}
	if n, err := events.Insert(ctx, []*models.Event{}); err != nil || n != 0 {
		t.Errorf("Insert(empty) = %d, %v; want 0 and no error", n, err)
	}
	if _, err := events.Insert(ctx, []*models.Event{nil}); err == nil {
		t.Error("Insert accepted a nil event")
	}

	noContract := newEvent(4430000, 0, "transfer")
	noContract.ContractID = ""
	if _, err := events.Insert(ctx, []*models.Event{noContract}); err == nil {
		t.Error("Insert accepted an event with no contract id")
	}
}

// --- query path ---

// ADR-022: the global emission order is (ledger, event_index), not insertion
// order and not the transaction hash.
func TestQueryReturnsEventsInLedgerOrder(t *testing.T) {
	t.Parallel()

	events, _, _ := eventsStore(t)
	seed(t, events,
		newEvent(4430002, 0, "third"),
		newEvent(4430000, 1, "second"),
		newEvent(4430000, 0, "first"),
	)

	page, err := events.Query(context.Background(), store.EventQuery{ContractID: showcase})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}

	want := []string{"first", "second", "third"}
	if len(page.Events) != len(want) {
		t.Fatalf("got %d events, want %d", len(page.Events), len(want))
	}
	for i, name := range want {
		if page.Events[i].Name != name {
			t.Errorf("event %d is %q, want %q", i, page.Events[i].Name, name)
		}
	}
}

func TestQueryFiltersByNameAndLedgerRange(t *testing.T) {
	t.Parallel()

	events, _, _ := eventsStore(t)
	ctx := context.Background()
	seed(t, events,
		newEvent(4430000, 0, "transfer"),
		newEvent(4430001, 1, "mint"),
		newEvent(4430002, 2, "transfer"),
		newEvent(4430003, 3, "burn"),
	)

	cases := []struct {
		name  string
		query store.EventQuery
		want  int
	}{
		{"no filter", store.EventQuery{ContractID: showcase}, 4},
		{"by name", store.EventQuery{ContractID: showcase, Name: "transfer"}, 2},
		{"from ledger", store.EventQuery{ContractID: showcase, FromLedger: 4430002}, 2},
		{"to ledger", store.EventQuery{ContractID: showcase, ToLedger: 4430001}, 2},
		{"ledger range", store.EventQuery{ContractID: showcase, FromLedger: 4430001, ToLedger: 4430002}, 2},
		{"name and range", store.EventQuery{ContractID: showcase, Name: "transfer", FromLedger: 4430001}, 1},
		{"name matching nothing", store.EventQuery{ContractID: showcase, Name: "nope"}, 0},
		{"range matching nothing", store.EventQuery{ContractID: showcase, FromLedger: 9000000}, 0},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			page, err := events.Query(ctx, c.query)
			if err != nil {
				t.Fatalf("Query: %v", err)
			}
			if len(page.Events) != c.want {
				t.Errorf("got %d events, want %d", len(page.Events), c.want)
			}
			if page.NextCursor != nil {
				t.Errorf("NextCursor = %q, want nil for a complete result", *page.NextCursor)
			}
		})
	}
}

// A tracked contract with no matching events is an empty page, not an error,
// per ADR-021. The untracked case is the caller's to distinguish.
func TestQueryReturnsAnEmptyPageRatherThanNil(t *testing.T) {
	t.Parallel()

	events, _, _ := eventsStore(t)

	page, err := events.Query(context.Background(), store.EventQuery{ContractID: showcase})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if page.Events == nil {
		t.Error("Events is nil, which encodes as JSON null rather than an empty array")
	}
	if page.NextCursor != nil {
		t.Errorf("NextCursor = %q, want nil", *page.NextCursor)
	}

	encoded, err := json.Marshal(page.Events)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if string(encoded) != "[]" {
		t.Errorf("an empty page encoded as %s, want []", encoded)
	}
}

// ADR-021: a caller pages until NextCursor is nil. Exhaustion is structural,
// so there is no empty-string sentinel to mistake for a usable cursor.
func TestPagingWalksEveryEventExactlyOnce(t *testing.T) {
	t.Parallel()

	events, _, _ := eventsStore(t)
	ctx := context.Background()

	const total = 25
	batch := make([]*models.Event, 0, total)
	for i := 0; i < total; i++ {
		batch = append(batch, newEvent(4430000+int64(i/3), int64(i), "transfer"))
	}
	seed(t, events, batch...)

	seen := map[int64]bool{}
	var order []int64
	cursor := ""
	pages := 0

	for {
		page, err := events.Query(ctx, store.EventQuery{ContractID: showcase, Limit: 7, Cursor: cursor})
		if err != nil {
			t.Fatalf("page %d: %v", pages, err)
		}
		pages++
		if pages > 10 {
			t.Fatal("paging did not terminate")
		}

		for _, event := range page.Events {
			if seen[event.ID] {
				t.Errorf("event %d appeared on more than one page", event.ID)
			}
			seen[event.ID] = true
			order = append(order, event.ID)
		}

		if page.NextCursor == nil {
			break
		}
		if *page.NextCursor == "" {
			t.Fatal("NextCursor is a pointer to an empty string; exhaustion must be nil")
		}
		cursor = *page.NextCursor
	}

	if len(seen) != total {
		t.Errorf("paging saw %d events, want %d", len(seen), total)
	}
	if pages != 4 {
		t.Errorf("paging took %d pages, want 4 for %d events at 7 per page", pages, total)
	}
	for i := 1; i < len(order); i++ {
		if order[i] <= order[i-1] {
			t.Errorf("paging returned ids out of order at %d: %v", i, order)
			break
		}
	}
}

// The last full page must not claim another page exists, or a caller loops once
// more for nothing and a naive consumer never terminates.
func TestAnExactlyFullPageReportsExhaustion(t *testing.T) {
	t.Parallel()

	events, _, _ := eventsStore(t)
	ctx := context.Background()

	batch := make([]*models.Event, 0, 10)
	for i := 0; i < 10; i++ {
		batch = append(batch, newEvent(4430000, int64(i), "transfer"))
	}
	seed(t, events, batch...)

	page, err := events.Query(ctx, store.EventQuery{ContractID: showcase, Limit: 10})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(page.Events) != 10 {
		t.Fatalf("got %d events, want 10", len(page.Events))
	}
	if page.NextCursor != nil {
		t.Errorf("NextCursor = %q on a page that consumed every event, want nil", *page.NextCursor)
	}
}

func TestQueryValidatesItsArguments(t *testing.T) {
	t.Parallel()

	events, _, _ := eventsStore(t)
	ctx := context.Background()

	if _, err := events.Query(ctx, store.EventQuery{}); err == nil {
		t.Error("Query accepted a request with no contract id")
	}
	if _, err := events.Query(ctx, store.EventQuery{ContractID: showcase, Limit: -1}); err == nil {
		t.Error("Query accepted a negative limit")
	}
	if _, err := events.Query(ctx, store.EventQuery{ContractID: showcase, Limit: 10001}); err == nil {
		t.Error("Query accepted a limit above the RPC page ceiling")
	}

	for _, cursor := range []string{"abc", "-1", "1.5", " 1", "0x10"} {
		if _, err := events.Query(ctx, store.EventQuery{ContractID: showcase, Cursor: cursor}); err == nil {
			t.Errorf("Query accepted the cursor %q", cursor)
		}
	}
}

func TestGetReturnsOneEventOrErrNotFound(t *testing.T) {
	t.Parallel()

	events, _, _ := eventsStore(t)
	ctx := context.Background()
	seed(t, events, newEvent(4430000, 0, "transfer"))

	page, err := events.Query(ctx, store.EventQuery{ContractID: showcase})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	id := page.Events[0].ID

	got, err := events.Get(ctx, id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ID != id || got.Name != "transfer" {
		t.Errorf("Get returned %+v", got)
	}

	if _, err := events.Get(ctx, id+9999); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("Get of an absent event returned %v, want ErrNotFound", err)
	}
}

// --- error paths ---

func TestEventMethodsWrapADatabaseFailure(t *testing.T) {
	t.Parallel()

	handle := unmigrated(t)
	events := store.NewEvents(handle)
	ctx := context.Background()

	cases := []struct {
		name string
		call func() error
	}{
		{"Insert", func() error {
			_, err := events.Insert(ctx, []*models.Event{newEvent(4430000, 0, "transfer")})
			return err
		}},
		{"Get", func() error { _, err := events.Get(ctx, 1); return err }},
		{"Query", func() error {
			_, err := events.Query(ctx, store.EventQuery{ContractID: showcase})
			return err
		}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			err := c.call()
			if err == nil {
				t.Fatalf("%s succeeded against a database with no tables", c.name)
			}
			if !strings.HasPrefix(err.Error(), "store: ") {
				t.Errorf("error %q is not wrapped by this package", err.Error())
			}
			if errors.Is(err, store.ErrNotFound) {
				t.Errorf("%s reported a database failure as ErrNotFound: %v", c.name, err)
			}
		})
	}
}

// The events store takes a Querier too, so a poll can write its events and its
// progress in one transaction.
func TestEventsAndProgressCommitTogether(t *testing.T) {
	t.Parallel()

	handle := migrated(t)
	ctx := context.Background()
	if _, err := store.NewContracts(handle).Register(ctx, showcase); err != nil {
		t.Fatalf("Register: %v", err)
	}

	tx, err := handle.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTx: %v", err)
	}
	if _, err := store.NewEvents(tx).Insert(ctx, []*models.Event{newEvent(4430000, 0, "transfer")}); err != nil {
		t.Fatalf("Insert in a transaction: %v", err)
	}
	if err := store.NewContracts(tx).SetProgress(ctx, showcase, 4430000); err != nil {
		t.Fatalf("SetProgress in a transaction: %v", err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatalf("Rollback: %v", err)
	}

	page, err := store.NewEvents(handle).Query(ctx, store.EventQuery{ContractID: showcase})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(page.Events) != 0 {
		t.Errorf("%d events survived a rolled back transaction", len(page.Events))
	}
	contract, err := store.NewContracts(handle).Get(ctx, showcase)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if contract.LastIndexedLedger != 0 {
		t.Errorf("LastIndexedLedger = %d, want 0; progress survived the rollback", contract.LastIndexedLedger)
	}
}

// The events of one contract must not appear in another's page.
func TestQueryIsScopedToItsContract(t *testing.T) {
	t.Parallel()

	events, contracts, _ := eventsStore(t)
	ctx := context.Background()
	if _, err := contracts.Register(ctx, other); err != nil {
		t.Fatalf("Register: %v", err)
	}

	mine := newEvent(4430000, 0, "transfer")
	theirs := newEvent(4430000, 1, "transfer")
	theirs.ContractID = other
	seed(t, events, mine, theirs)

	page, err := events.Query(ctx, store.EventQuery{ContractID: showcase})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(page.Events) != 1 {
		t.Fatalf("got %d events, want only this contract's 1", len(page.Events))
	}
	if page.Events[0].ContractID != showcase {
		t.Errorf("ContractID = %q, want %q", page.Events[0].ContractID, showcase)
	}
}

func TestCursorIsTheEventIDAsDigits(t *testing.T) {
	t.Parallel()

	events, _, _ := eventsStore(t)
	ctx := context.Background()
	seed(t, events,
		newEvent(4430000, 0, "a"),
		newEvent(4430000, 1, "b"),
	)

	page, err := events.Query(ctx, store.EventQuery{ContractID: showcase, Limit: 1})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if page.NextCursor == nil {
		t.Fatal("NextCursor is nil with a page still to come")
	}
	if _, err := strconv.ParseInt(*page.NextCursor, 10, 64); err != nil {
		t.Errorf("NextCursor = %q, which is not a string of digits", *page.NextCursor)
	}
	if *page.NextCursor != strconv.FormatInt(page.Events[0].ID, 10) {
		t.Errorf("NextCursor = %q, want the last event's id %d", *page.NextCursor, page.Events[0].ID)
	}
}
