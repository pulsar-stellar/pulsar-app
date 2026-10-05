package store_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"testing"

	"github.com/pulsar-stellar/pulsar-app/indexer/internal/db"
	"github.com/pulsar-stellar/pulsar-app/indexer/internal/store"
)

// Postgres dialect tests.
//
// Every other test in this package runs against in-memory SQLite, so
// store.DialectPostgres had never been executed by anything: the one query
// fragment the engines spell differently, topicContainsClause, was asserted
// only as a SQL string. A typo in the jsonb spelling would have surfaced first
// in production, where Postgres is the engine. These tests run the real query
// path against a real server.
//
// Gated on PULSAR_TEST_POSTGRES_DSN and skipped when it is unset, so a
// contributor with only SQLite still gets a green `go test ./...` (ADR-003).
//
// These share one database with the migration tests in internal/db, and both
// reset the public schema, so the Postgres-backed packages must not run
// concurrently: use `go test -p 1 ./...` when the DSN is set.

const postgresDSNEnv = "PULSAR_TEST_POSTGRES_DSN"

// postgresEventsStore returns an events store backed by a freshly migrated
// Postgres schema, with the showcase contract registered.
func postgresEventsStore(t *testing.T) (*store.Events, *sql.DB) {
	t.Helper()

	dsn := os.Getenv(postgresDSNEnv)
	if dsn == "" {
		t.Skipf("%s is not set; skipping the Postgres dialect tests", postgresDSNEnv)
	}

	driver, err := db.Resolve(db.Options{DriverName: "postgres"})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	handle, err := db.Open(driver, db.ConnOptions{
		DSN: dsn,
		// CI points at the official postgres image, which ships with TLS off.
		AllowInsecureTLS: os.Getenv("PULSAR_TEST_POSTGRES_INSECURE") == "true",
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = handle.Close() })

	ctx := context.Background()
	if _, err := handle.ExecContext(ctx, `DROP SCHEMA public CASCADE; CREATE SCHEMA public;`); err != nil {
		t.Fatalf("resetting the public schema: %v", err)
	}
	if _, err := db.Up(ctx, handle, driver); err != nil {
		t.Fatalf("Up: %v", err)
	}

	contracts := store.NewContracts(handle)
	if _, err := contracts.Register(ctx, showcase); err != nil {
		t.Fatalf("Register: %v", err)
	}
	return store.NewEvents(handle, store.DialectPostgres), handle
}

// TestPostgresQueryFiltersByTopicContains mirrors the SQLite table in
// events_test.go case for case. The two engines must agree: the filter is a
// literal, case-sensitive substring test over every topic's value, with % and _
// ordinary characters rather than LIKE wildcards.
func TestPostgresQueryFiltersByTopicContains(t *testing.T) {
	events, _ := postgresEventsStore(t)
	ctx := context.Background()

	custom := newEvent(4430010, 10, "custom")
	custom.TopicsJSON = json.RawMessage(`[{"type":"symbol","value":"admin"},{"type":"string","value":"fee%rate"}]`)

	seed(t, events,
		newEvent(4430000, 0, "transfer"),
		newEvent(4430001, 1, "mint"),
		newEvent(4430002, 2, "burn"),
		custom,
	)

	cases := []struct {
		name  string
		query store.EventQuery
		want  []string
	}{
		{"literal substring", store.EventQuery{ContractID: showcase, TopicContains: "trans"}, []string{"transfer"}},
		{"case sensitive misses a different case", store.EventQuery{ContractID: showcase, TopicContains: "Trans"}, nil},
		{"matches a value that is not the event name", store.EventQuery{ContractID: showcase, TopicContains: "admin"}, []string{"custom"}},
		{"matches a topic that is not the first", store.EventQuery{ContractID: showcase, TopicContains: "fee%"}, []string{"custom"}},
		{"percent is literal not a wildcard", store.EventQuery{ContractID: showcase, TopicContains: "fee%rate"}, []string{"custom"}},
		{"underscore is literal not a wildcard", store.EventQuery{ContractID: showcase, TopicContains: "fee_rate"}, nil},
		{"no match is an empty page", store.EventQuery{ContractID: showcase, TopicContains: "nowhere"}, nil},
		{"combines with name", store.EventQuery{ContractID: showcase, Name: "transfer", TopicContains: "trans"}, []string{"transfer"}},
		{"name filters out a topic match", store.EventQuery{ContractID: showcase, Name: "mint", TopicContains: "trans"}, nil},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			page, err := events.Query(ctx, c.query)
			if err != nil {
				t.Fatalf("Query: %v", err)
			}
			if page.Events == nil {
				t.Fatal("Events is nil, which encodes as JSON null rather than []")
			}
			got := make([]string, len(page.Events))
			for i, e := range page.Events {
				got[i] = e.Name
			}
			if len(got) != len(c.want) {
				t.Fatalf("got %v, want %v", got, c.want)
			}
			for i := range c.want {
				if got[i] != c.want[i] {
					t.Errorf("result %d is %q, want %q (%v)", i, got[i], c.want[i], got)
				}
			}
		})
	}
}

// TestPostgresTopicFilterToleratesAnEventWithNoTopics pins the guard the clause
// carries: topics_json for an event with no topics is the scalar JSON null, and
// jsonb_array_elements raises on a non-array rather than matching nothing. The
// jsonb_typeof check is what keeps the query from erroring.
func TestPostgresTopicFilterToleratesAnEventWithNoTopics(t *testing.T) {
	events, _ := postgresEventsStore(t)
	ctx := context.Background()

	topicless := newEvent(4430020, 20, "topicless")
	topicless.TopicsJSON = json.RawMessage(`null`)

	seed(t, events, topicless, newEvent(4430021, 21, "transfer"))

	page, err := events.Query(ctx, store.EventQuery{ContractID: showcase, TopicContains: "trans"})
	if err != nil {
		t.Fatalf("Query raised with a topicless event present: %v", err)
	}
	if len(page.Events) != 1 || page.Events[0].Name != "transfer" {
		t.Errorf("got %d events, want just transfer", len(page.Events))
	}
}

// TestPostgresInsertRoundTripsJSONB confirms the decoded payloads survive the
// jsonb columns unchanged. SQLite stores them as TEXT, so this is the one
// engine where the database parses and re-renders the JSON.
func TestPostgresInsertRoundTripsJSONB(t *testing.T) {
	events, _ := postgresEventsStore(t)
	ctx := context.Background()

	original := newEvent(4430030, 30, "transfer")
	seed(t, events, original)

	page, err := events.Query(ctx, store.EventQuery{ContractID: showcase})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(page.Events) != 1 {
		t.Fatalf("got %d events, want 1", len(page.Events))
	}

	got := page.Events[0]
	var gotTopics, wantTopics []map[string]any
	if err := json.Unmarshal(got.TopicsJSON, &gotTopics); err != nil {
		t.Fatalf("unmarshalling the stored topics: %v", err)
	}
	if err := json.Unmarshal(original.TopicsJSON, &wantTopics); err != nil {
		t.Fatalf("unmarshalling the original topics: %v", err)
	}
	if len(gotTopics) != len(wantTopics) || gotTopics[0]["value"] != wantTopics[0]["value"] {
		t.Errorf("topics round-tripped as %v, want %v", gotTopics, wantTopics)
	}
	if !got.InSuccessfulContractCall {
		t.Error("the success flag round-tripped as false, want true")
	}
}
