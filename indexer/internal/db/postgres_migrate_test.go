package db_test

import (
	"context"
	"database/sql"
	"os"
	"testing"

	"github.com/pulsar-stellar/pulsar-app/indexer/internal/db"
	"github.com/pulsar-stellar/pulsar-app/indexer/migrations"
)

// Postgres migration tests.
//
// Everything Postgres-specific was previously verified structurally rather than
// by running it: the migration files were diffed against their SQLite
// counterparts, the DSN's TLS refusal was checked against pgx.ParseConfig's
// output, and the statement splitter was unit tested because pgx's extended
// protocol rejects the multi-statement Exec SQLite accepts. No Postgres server
// had ever applied the schema. These tests close that gap.
//
// They need a real server, so they are gated on PULSAR_TEST_POSTGRES_DSN and
// skip when it is unset. That keeps `go test ./...` green for a contributor who
// has only SQLite, which ADR-003's zero-setup promise requires, while CI and
// any maintainer with a local Postgres run them for real.
//
// The DSN should carry sslmode=require (or stricter), so these tests exercise
// the ADR-031 encrypted-connection guard rather than bypassing it. Set
// PULSAR_TEST_POSTGRES_INSECURE=true only against a server with TLS off, such
// as the official postgres image CI uses.
//
// These share one database with the dialect tests in internal/store, and both
// reset the public schema, so the Postgres-backed packages must not run
// concurrently: use `go test -p 1 ./...` when the DSN is set.

const postgresDSNEnv = "PULSAR_TEST_POSTGRES_DSN"

// newPostgres opens the test database and hands back a clean, empty schema.
//
// Each test starts from nothing, so ordering between tests cannot matter and a
// failed run does not poison the next one.
func newPostgres(t *testing.T) (*sql.DB, db.Driver) {
	t.Helper()

	dsn := os.Getenv(postgresDSNEnv)
	if dsn == "" {
		t.Skipf("%s is not set; skipping the Postgres migration tests", postgresDSNEnv)
	}

	driver, err := db.Resolve(db.Options{DriverName: "postgres"})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if driver.MigrationsDir != migrations.DirPostgres {
		t.Fatalf("resolved migrations dir %q, want %q", driver.MigrationsDir, migrations.DirPostgres)
	}

	// AllowInsecureTLS stays false against a server that offers TLS, so the
	// ADR-031 guard is exercised rather than bypassed. CI points at the
	// official postgres image, which ships with TLS off, and opts out.
	handle, err := db.Open(driver, db.ConnOptions{
		DSN:              dsn,
		AllowInsecureTLS: os.Getenv("PULSAR_TEST_POSTGRES_INSECURE") == "true",
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = handle.Close() })

	if err := handle.PingContext(context.Background()); err != nil {
		t.Fatalf("connecting to the test Postgres: %v", err)
	}

	resetSchema(t, handle)
	t.Cleanup(func() { resetSchema(t, handle) })

	return handle, driver
}

// resetSchema drops everything the migrations create, plus the bookkeeping
// table, leaving an empty public schema.
func resetSchema(t *testing.T, handle *sql.DB) {
	t.Helper()

	const reset = `
		DROP SCHEMA public CASCADE;
		CREATE SCHEMA public;
	`
	if _, err := handle.ExecContext(context.Background(), reset); err != nil {
		t.Fatalf("resetting the public schema: %v", err)
	}
}

func postgresVersions(t *testing.T) []int {
	t.Helper()

	ms, err := migrations.For(migrations.DirPostgres)
	if err != nil {
		t.Fatalf("loading postgres migrations: %v", err)
	}
	versions := make([]int, 0, len(ms))
	for _, m := range ms {
		versions = append(versions, m.Version)
	}
	if len(versions) == 0 {
		t.Fatal("no postgres migrations are shipped")
	}
	return versions
}

// TestPostgresUpAppliesEveryMigrationInOrder is the test the gap was about: it
// runs the real Up against a real server, so the embedded SQL, the statement
// splitter, and pgx's extended protocol are all exercised together.
func TestPostgresUpAppliesEveryMigrationInOrder(t *testing.T) {
	handle, driver := newPostgres(t)

	applied, err := db.Up(context.Background(), handle, driver)
	if err != nil {
		t.Fatalf("Up: %v", err)
	}

	want := postgresVersions(t)
	if !equalInts(applied, want) {
		t.Errorf("Up applied %v, want %v", applied, want)
	}

	version, err := db.Version(context.Background(), handle)
	if err != nil {
		t.Fatalf("Version: %v", err)
	}
	if version != want[len(want)-1] {
		t.Errorf("Version reports %d, want %d", version, want[len(want)-1])
	}
}

// TestPostgresSchemaUsesItsNativeTypes checks the things that differ from
// SQLite on purpose: JSONB columns and the GIN index that makes a
// topic_contains filter an indexed lookup rather than a scan.
func TestPostgresSchemaUsesItsNativeTypes(t *testing.T) {
	handle, driver := newPostgres(t)
	if _, err := db.Up(context.Background(), handle, driver); err != nil {
		t.Fatalf("Up: %v", err)
	}

	for _, column := range []string{"topics_json", "data_json"} {
		var dataType string
		err := handle.QueryRowContext(context.Background(),
			`SELECT data_type FROM information_schema.columns
			 WHERE table_name = 'events' AND column_name = $1`, column).Scan(&dataType)
		if err != nil {
			t.Fatalf("reading the type of %s: %v", column, err)
		}
		if dataType != "jsonb" {
			t.Errorf("%s is %s, want jsonb", column, dataType)
		}
	}

	var ginIndexes int
	err := handle.QueryRowContext(context.Background(),
		`SELECT count(*) FROM pg_indexes
		 WHERE tablename = 'events' AND indexdef ILIKE '%USING gin%'`).Scan(&ginIndexes)
	if err != nil {
		t.Fatalf("counting GIN indexes: %v", err)
	}
	if ginIndexes != 1 {
		t.Errorf("found %d GIN indexes on events, want 1", ginIndexes)
	}
}

// TestPostgresDeleteCascadesToEvents pins the behaviour the delete route
// depends on (ADR-044): removing a contract removes its events.
func TestPostgresDeleteCascadesToEvents(t *testing.T) {
	handle, driver := newPostgres(t)
	if _, err := db.Up(context.Background(), handle, driver); err != nil {
		t.Fatalf("Up: %v", err)
	}

	ctx := context.Background()
	const contractID = "CDNWTVUDKCCGW7GOC6SBLUFXXUCD2YDHWRDUSXZ6CYBQKQWLCUYYWI5L"

	if _, err := handle.ExecContext(ctx,
		`INSERT INTO contracts (id) VALUES ($1)`, contractID); err != nil {
		t.Fatalf("inserting the contract: %v", err)
	}
	if _, err := handle.ExecContext(ctx,
		`INSERT INTO events (contract_id, ledger, tx_hash, event_index, name,
		                     topics_json, data_json, raw_topics, raw_data,
		                     emitted_at, in_successful_contract_call)
		 VALUES ($1, 1, 'abc', 0, 'transfer', '[]'::jsonb, '{}'::jsonb, '[]', '',
		         now(), false)`, contractID); err != nil {
		t.Fatalf("inserting the event: %v", err)
	}

	if _, err := handle.ExecContext(ctx, `DELETE FROM contracts WHERE id = $1`, contractID); err != nil {
		t.Fatalf("deleting the contract: %v", err)
	}

	var remaining int
	if err := handle.QueryRowContext(ctx, `SELECT count(*) FROM events`).Scan(&remaining); err != nil {
		t.Fatalf("counting events: %v", err)
	}
	if remaining != 0 {
		t.Errorf("%d events survived the contract delete, want 0", remaining)
	}
}

// TestPostgresEventSuccessFlagRoundTripsAsABool mirrors the SQLite test: the
// 0003 column must scan back into a Go bool on this engine too.
func TestPostgresEventSuccessFlagRoundTripsAsABool(t *testing.T) {
	handle, driver := newPostgres(t)
	if _, err := db.Up(context.Background(), handle, driver); err != nil {
		t.Fatalf("Up: %v", err)
	}

	ctx := context.Background()
	const contractID = "CDNWTVUDKCCGW7GOC6SBLUFXXUCD2YDHWRDUSXZ6CYBQKQWLCUYYWI5L"

	if _, err := handle.ExecContext(ctx, `INSERT INTO contracts (id) VALUES ($1)`, contractID); err != nil {
		t.Fatalf("inserting the contract: %v", err)
	}
	if _, err := handle.ExecContext(ctx,
		`INSERT INTO events (contract_id, ledger, tx_hash, event_index, name,
		                     topics_json, data_json, raw_topics, raw_data,
		                     emitted_at, in_successful_contract_call)
		 VALUES ($1, 2, 'def', 1, 'mint', '[]'::jsonb, '{}'::jsonb, '[]', '',
		         now(), false)`, contractID); err != nil {
		t.Fatalf("inserting the event: %v", err)
	}

	var committed bool
	if err := handle.QueryRowContext(ctx,
		`SELECT in_successful_contract_call FROM events WHERE ledger = 2`).Scan(&committed); err != nil {
		t.Fatalf("scanning the flag: %v", err)
	}
	if committed {
		t.Error("the flag scanned back as true, want false")
	}
}

// TestPostgresDownReversesEveryMigration confirms the down files work against a
// real server, so a rollback is not a path that has only ever been read.
func TestPostgresDownReversesEveryMigration(t *testing.T) {
	handle, driver := newPostgres(t)
	ctx := context.Background()

	if _, err := db.Up(ctx, handle, driver); err != nil {
		t.Fatalf("Up: %v", err)
	}

	reverted, err := db.Down(ctx, handle, driver, 0)
	if err != nil {
		t.Fatalf("Down: %v", err)
	}
	if want := reversed(postgresVersions(t)); !equalInts(reverted, want) {
		t.Errorf("Down reverted %v, want %v", reverted, want)
	}

	var tables int
	if err := handle.QueryRowContext(ctx,
		`SELECT count(*) FROM information_schema.tables
		 WHERE table_schema = 'public' AND table_name IN ('contracts', 'events')`).Scan(&tables); err != nil {
		t.Fatalf("counting tables: %v", err)
	}
	if tables != 0 {
		t.Errorf("%d migrated tables survived a full down, want 0", tables)
	}
}

// TestPostgresUpIsIdempotent confirms a second Up is a no-op, which is what
// makes restarting a deployed indexer safe.
func TestPostgresUpIsIdempotent(t *testing.T) {
	handle, driver := newPostgres(t)
	ctx := context.Background()

	if _, err := db.Up(ctx, handle, driver); err != nil {
		t.Fatalf("first Up: %v", err)
	}
	again, err := db.Up(ctx, handle, driver)
	if err != nil {
		t.Fatalf("second Up: %v", err)
	}
	if len(again) != 0 {
		t.Errorf("the second Up applied %v, want nothing", again)
	}
}
