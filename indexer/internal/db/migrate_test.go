package db_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/pulsar-stellar/pulsar-app/indexer/internal/db"
	"github.com/pulsar-stellar/pulsar-app/indexer/migrations"
)

// allVersions is derived from the shipped migrations rather than hard-coded, so
// adding a migration does not require editing every count in this file.
func allVersions(t *testing.T) []int {
	t.Helper()

	ms, err := migrations.For(migrations.DirSQLite)
	if err != nil {
		t.Fatalf("loading migrations: %v", err)
	}
	versions := make([]int, 0, len(ms))
	for _, m := range ms {
		versions = append(versions, m.Version)
	}
	if len(versions) == 0 {
		t.Fatal("no migrations are shipped")
	}
	return versions
}

func latestVersion(t *testing.T) int {
	t.Helper()
	versions := allVersions(t)
	return versions[len(versions)-1]
}

func reversed(versions []int) []int {
	out := make([]int, len(versions))
	for i, v := range versions {
		out[len(versions)-1-i] = v
	}
	return out
}

func equalInts(got, want []int) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

const showcase = "CDNWTVUDKCCGW7GOC6SBLUFXXUCD2YDHWRDUSXZ6CYBQKQWLCUYYWI5L"

// newSQLite opens an in-memory database through the real driver.
//
// In-memory is safe here only because the SQLite pool is pinned to a single
// connection: database/sql handing out a second connection would hand out a
// second, empty database. TestInMemoryDatabaseSurvivesPoolReuse pins that
// dependency so a later change to the pool bounds fails loudly rather than
// leaving this suite testing empty databases.
func newSQLite(t *testing.T) (*sql.DB, db.Driver) {
	t.Helper()

	driver, err := db.Resolve(db.Options{DriverName: "sqlite"})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	handle, err := db.Open(driver, db.ConnOptions{DSN: "file::memory:"})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = handle.Close() })
	return handle, driver
}

func namesOfType(t *testing.T, handle *sql.DB, kind string) []string {
	t.Helper()

	rows, err := handle.Query(
		"SELECT name FROM sqlite_master WHERE type = $1 AND name NOT LIKE 'sqlite_%'", kind)
	if err != nil {
		t.Fatalf("listing %ss: %v", kind, err)
	}
	defer func() { _ = rows.Close() }()

	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scanning %s name: %v", kind, err)
		}
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("listing %ss: %v", kind, err)
	}
	sort.Strings(names)
	return names
}

func tableNames(t *testing.T, handle *sql.DB) []string {
	t.Helper()
	return namesOfType(t, handle, "table")
}

func indexNames(t *testing.T, handle *sql.DB) []string {
	t.Helper()
	return namesOfType(t, handle, "index")
}

func insertContract(t *testing.T, handle *sql.DB, id string) {
	t.Helper()
	if _, err := handle.Exec("INSERT INTO contracts (id) VALUES ($1)", id); err != nil {
		t.Fatalf("inserting contract: %v", err)
	}
}

func insertEvent(handle *sql.DB, contractID string, ledger, eventIndex int, txHash string) error {
	_, err := handle.Exec(
		`INSERT INTO events
		 (contract_id, ledger, tx_hash, event_index, name, topics_json, data_json, raw_topics, raw_data, emitted_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		contractID, ledger, txHash, eventIndex, "transfer",
		`["AAAADwAAAANmZWUA"]`, `{"amount":100}`, `["AAAADwAAAANmZWUA"]`, "AAAACg==",
		"2026-09-01T10:53:07Z")
	return err
}

func TestUpAppliesEveryMigrationInOrder(t *testing.T) {
	t.Parallel()

	handle, driver := newSQLite(t)

	applied, err := db.Up(context.Background(), handle, driver)
	if err != nil {
		t.Fatalf("Up: %v", err)
	}
	if want := allVersions(t); !equalInts(applied, want) {
		t.Fatalf("Up applied %v, want %v", applied, want)
	}

	want := []string{"contracts", "events", "schema_migrations"}
	if got := tableNames(t, handle); !equalStrings(got, want) {
		t.Errorf("tables = %v, want %v", got, want)
	}

	version, err := db.Version(context.Background(), handle)
	if err != nil {
		t.Fatalf("Version: %v", err)
	}
	if want := latestVersion(t); version != want {
		t.Errorf("Version = %d, want %d", version, want)
	}
}

func TestUpCreatesTheIndexesFrom0002(t *testing.T) {
	t.Parallel()

	handle, driver := newSQLite(t)
	if _, err := db.Up(context.Background(), handle, driver); err != nil {
		t.Fatalf("Up: %v", err)
	}

	want := []string{
		"idx_events_contract_ledger",
		"idx_events_emitted_at",
		"idx_events_name",
		"idx_events_topics",
	}
	if got := indexNames(t, handle); !equalStrings(got, want) {
		t.Errorf("indexes = %v, want %v", got, want)
	}
}

// The schema is only correct if it behaves correctly: the id autoincrements
// rather than arriving NULL as BIGSERIAL would have made it, the ledger
// ordinal is unique, and deleting a contract takes its events with it.
func TestTheMigratedSchemaEnforcesItsConstraints(t *testing.T) {
	t.Parallel()

	handle, driver := newSQLite(t)
	if _, err := db.Up(context.Background(), handle, driver); err != nil {
		t.Fatalf("Up: %v", err)
	}
	insertContract(t, handle, showcase)

	if err := insertEvent(handle, showcase, 4430000, 0, "9f4067a3"); err != nil {
		t.Fatalf("inserting first event: %v", err)
	}
	if err := insertEvent(handle, showcase, 4430000, 1, "d10d31e5"); err != nil {
		t.Fatalf("inserting second event: %v", err)
	}

	// ADR-029 is the reason the two engines have separate migration files.
	rows, err := handle.Query("SELECT id FROM events ORDER BY id")
	if err != nil {
		t.Fatalf("reading ids: %v", err)
	}
	var ids []int64
	for rows.Next() {
		var id sql.NullInt64
		if err := rows.Scan(&id); err != nil {
			t.Fatalf("scanning id: %v", err)
		}
		if !id.Valid {
			t.Fatal("event id is NULL, which is what BIGSERIAL on SQLite would produce")
		}
		ids = append(ids, id.Int64)
	}
	_ = rows.Close()
	if len(ids) != 2 || ids[0] != 1 || ids[1] != 2 {
		t.Errorf("ids = %v, want [1 2]", ids)
	}

	// ADR-022: uniqueness is on (ledger, event_index), not (tx_hash, event_index).
	if err := insertEvent(handle, showcase, 4430000, 0, "a-different-tx-hash"); err == nil {
		t.Error("a repeated (ledger, event_index) was accepted; the ADR-022 constraint is not enforced")
	}
	if err := insertEvent(handle, showcase, 4430001, 0, "another-tx"); err != nil {
		t.Errorf("a new ledger reusing ordinal 0 was rejected: %v", err)
	}

	// ON DELETE CASCADE only fires because the driver sets foreign_keys(1).
	if _, err := handle.Exec("DELETE FROM contracts WHERE id = $1", showcase); err != nil {
		t.Fatalf("deleting contract: %v", err)
	}
	var remaining int
	if err := handle.QueryRow("SELECT COUNT(*) FROM events").Scan(&remaining); err != nil {
		t.Fatalf("counting events: %v", err)
	}
	if remaining != 0 {
		t.Errorf("%d events survived their contract; ON DELETE CASCADE did not fire", remaining)
	}
}

func TestDownToOneReversesOnly0002(t *testing.T) {
	t.Parallel()

	handle, driver := newSQLite(t)
	if _, err := db.Up(context.Background(), handle, driver); err != nil {
		t.Fatalf("Up: %v", err)
	}

	reverted, err := db.Down(context.Background(), handle, driver, 1)
	if err != nil {
		t.Fatalf("Down: %v", err)
	}
	var wantReverted []int
	for _, v := range reversed(allVersions(t)) {
		if v > 1 {
			wantReverted = append(wantReverted, v)
		}
	}
	if !equalInts(reverted, wantReverted) {
		t.Fatalf("Down reverted %v, want %v", reverted, wantReverted)
	}

	if got := indexNames(t, handle); len(got) != 0 {
		t.Errorf("indexes = %v, want none after reversing 0002", got)
	}
	want := []string{"contracts", "events", "schema_migrations"}
	if got := tableNames(t, handle); !equalStrings(got, want) {
		t.Errorf("tables = %v, want %v; reversing 0002 must leave 0001 intact", got, want)
	}
	if version, _ := db.Version(context.Background(), handle); version != 1 {
		t.Errorf("Version = %d, want 1", version)
	}
}

func TestDownToZeroLeavesOnlyTheBookkeepingTable(t *testing.T) {
	t.Parallel()

	handle, driver := newSQLite(t)
	if _, err := db.Up(context.Background(), handle, driver); err != nil {
		t.Fatalf("Up: %v", err)
	}

	reverted, err := db.Down(context.Background(), handle, driver, 0)
	if err != nil {
		t.Fatalf("Down: %v", err)
	}
	if want := reversed(allVersions(t)); !equalInts(reverted, want) {
		t.Fatalf("Down reverted %v, want %v, newest first", reverted, want)
	}

	if got := tableNames(t, handle); !equalStrings(got, []string{"schema_migrations"}) {
		t.Errorf("tables = %v, want only schema_migrations", got)
	}
	if version, _ := db.Version(context.Background(), handle); version != 0 {
		t.Errorf("Version = %d, want 0", version)
	}
}

func TestUpAfterDownRebuildsTheSchema(t *testing.T) {
	t.Parallel()

	handle, driver := newSQLite(t)
	ctx := context.Background()

	if _, err := db.Up(ctx, handle, driver); err != nil {
		t.Fatalf("first Up: %v", err)
	}
	if _, err := db.Down(ctx, handle, driver, 0); err != nil {
		t.Fatalf("Down: %v", err)
	}
	applied, err := db.Up(ctx, handle, driver)
	if err != nil {
		t.Fatalf("second Up: %v", err)
	}
	if want := allVersions(t); !equalInts(applied, want) {
		t.Fatalf("second Up applied %v, want %v again", applied, want)
	}

	insertContract(t, handle, showcase)
	if err := insertEvent(handle, showcase, 4430000, 0, "abc"); err != nil {
		t.Errorf("the rebuilt schema does not accept an event: %v", err)
	}
}

func TestUpIsIdempotent(t *testing.T) {
	t.Parallel()

	handle, driver := newSQLite(t)
	ctx := context.Background()

	if _, err := db.Up(ctx, handle, driver); err != nil {
		t.Fatalf("first Up: %v", err)
	}
	applied, err := db.Up(ctx, handle, driver)
	if err != nil {
		t.Fatalf("second Up: %v", err)
	}
	if len(applied) != 0 {
		t.Errorf("second Up applied %v, want nothing; migrations must not reapply", applied)
	}

	var rows int
	if err := handle.QueryRow("SELECT COUNT(*) FROM schema_migrations").Scan(&rows); err != nil {
		t.Fatalf("counting schema_migrations: %v", err)
	}
	if want := len(allVersions(t)); rows != want {
		t.Errorf("schema_migrations has %d rows, want %d", rows, want)
	}
}

// A recorded version with its predecessor missing means the schema is not any
// version at all, so Version refuses rather than reporting the highest.
func TestVersionRefusesASchemaWithAGap(t *testing.T) {
	t.Parallel()

	handle, driver := newSQLite(t)
	ctx := context.Background()

	if _, err := db.Up(ctx, handle, driver); err != nil {
		t.Fatalf("Up: %v", err)
	}
	if _, err := handle.Exec("DELETE FROM schema_migrations WHERE version = 1"); err != nil {
		t.Fatalf("removing the record: %v", err)
	}
	latest := latestVersion(t)

	_, err := db.Version(ctx, handle)
	if err == nil {
		t.Fatal("Version accepted a schema recorded as 2 with 1 missing")
	}
	if !errors.Is(err, db.ErrDirtySchema) {
		t.Errorf("error %v is not ErrDirtySchema", err)
	}
	if !strings.Contains(err.Error(), "1") || !strings.Contains(err.Error(), fmt.Sprint(latest)) {
		t.Errorf("error %q does not name both the missing version and the highest", err.Error())
	}
}

func TestDownRejectsANegativeTarget(t *testing.T) {
	t.Parallel()

	handle, driver := newSQLite(t)
	if _, err := db.Down(context.Background(), handle, driver, -1); err == nil {
		t.Error("Down accepted a negative target")
	}
}

// A migration that fails partway must leave nothing behind: no half-created
// schema and no row claiming it succeeded. Both engines have transactional
// DDL, which is what makes this hold rather than being a hope.
func TestAFailedMigrationRollsBackEntirely(t *testing.T) {
	t.Parallel()

	handle, driver := newSQLite(t)
	ctx := context.Background()

	// contracts already exists, so 0001's first statement fails. Its second
	// statement, which creates events, must never run.
	if _, err := handle.Exec("CREATE TABLE contracts (id TEXT NOT NULL PRIMARY KEY)"); err != nil {
		t.Fatalf("seeding the conflicting table: %v", err)
	}

	applied, err := db.Up(ctx, handle, driver)
	if err == nil {
		t.Fatal("Up succeeded against a conflicting schema")
	}
	if len(applied) != 0 {
		t.Errorf("Up reported %v as applied, want none", applied)
	}
	for _, want := range []string{"0001_init.up.sql", "rolled back", "statement 1 of 2"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err.Error(), want)
		}
	}

	if got := tableNames(t, handle); contains(got, "events") {
		t.Errorf("tables = %v; events was created by a migration that failed", got)
	}

	var recorded int
	if err := handle.QueryRow("SELECT COUNT(*) FROM schema_migrations").Scan(&recorded); err != nil {
		t.Fatalf("counting schema_migrations: %v", err)
	}
	if recorded != 0 {
		t.Errorf("schema_migrations has %d rows after a failed migration, want 0", recorded)
	}
	if version, err := db.Version(ctx, handle); err != nil || version != 0 {
		t.Errorf("Version = %d (err %v), want 0; a failure must leave the previous version", version, err)
	}
}

// The second migration failing must leave the first applied and recorded,
// rather than rolling the whole run back or recording the second anyway.
func TestAFailureLeavesEarlierMigrationsApplied(t *testing.T) {
	t.Parallel()

	handle, driver := newSQLite(t)
	ctx := context.Background()

	if _, err := db.Up(ctx, handle, driver); err != nil {
		t.Fatalf("Up: %v", err)
	}
	if _, err := db.Down(ctx, handle, driver, 1); err != nil {
		t.Fatalf("Down to 1: %v", err)
	}
	// An index 0002 wants to create already exists, so 0002 fails.
	if _, err := handle.Exec("CREATE INDEX idx_events_name ON events (name)"); err != nil {
		t.Fatalf("seeding the conflicting index: %v", err)
	}

	applied, err := db.Up(ctx, handle, driver)
	if err == nil {
		t.Fatal("Up succeeded despite a conflicting index")
	}
	if len(applied) != 0 {
		t.Errorf("Up reported %v as applied", applied)
	}

	version, err := db.Version(ctx, handle)
	if err != nil {
		t.Fatalf("Version: %v", err)
	}
	if version != 1 {
		t.Errorf("Version = %d, want 1; the failure must leave 0001 applied", version)
	}
	if got := indexNames(t, handle); contains(got, "idx_events_topics") {
		t.Errorf("indexes = %v; a later statement of the failed migration was committed", got)
	}
}

// 0003 adds the ADR-026 flag the SDK requires on every event. SQLite has no
// boolean type and stores the column with INTEGER affinity, so the round trip
// through a Go bool is checked rather than assumed.
func TestEventSuccessFlagRoundTripsAsABool(t *testing.T) {
	t.Parallel()

	handle, driver := newSQLite(t)
	if _, err := db.Up(context.Background(), handle, driver); err != nil {
		t.Fatalf("Up: %v", err)
	}
	insertContract(t, handle, showcase)

	if _, err := handle.Exec(
		`INSERT INTO events
		 (contract_id, ledger, tx_hash, event_index, name, topics_json, data_json,
		  raw_topics, raw_data, emitted_at, in_successful_contract_call)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
		showcase, 4430000, "9f4067a3", 0, "transfer",
		`["AAAADwAAAANmZWUA"]`, `{"amount":100}`, `["AAAADwAAAANmZWUA"]`, "AAAACg==",
		"2026-09-01T10:53:07Z", false); err != nil {
		t.Fatalf("inserting a reverted event: %v", err)
	}
	if _, err := handle.Exec(
		`INSERT INTO events
		 (contract_id, ledger, tx_hash, event_index, name, topics_json, data_json,
		  raw_topics, raw_data, emitted_at, in_successful_contract_call)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
		showcase, 4430000, "d10d31e5", 1, "transfer",
		`["AAAADwAAAANmZWUA"]`, `{"amount":100}`, `["AAAADwAAAANmZWUA"]`, "AAAACg==",
		"2026-09-01T10:53:07Z", true); err != nil {
		t.Fatalf("inserting a committed event: %v", err)
	}

	rows, err := handle.Query("SELECT in_successful_contract_call FROM events ORDER BY event_index")
	if err != nil {
		t.Fatalf("reading the flag: %v", err)
	}
	defer func() { _ = rows.Close() }()

	var flags []bool
	for rows.Next() {
		var flag bool
		if err := rows.Scan(&flag); err != nil {
			t.Fatalf("scanning the flag into a bool: %v", err)
		}
		flags = append(flags, flag)
	}
	if len(flags) != 2 || flags[0] || !flags[1] {
		t.Errorf("flags = %v, want [false true]", flags)
	}

	// Reverting 0003 removes the column again.
	if _, err := db.Down(context.Background(), handle, driver, 2); err != nil {
		t.Fatalf("Down to 2: %v", err)
	}
	if _, err := handle.Query("SELECT in_successful_contract_call FROM events"); err == nil {
		t.Error("the column survived a reversal of 0003")
	}
}

// An in-memory SQLite database belongs to its connection. This suite is only
// valid because the pool is pinned to exactly one, which ADR-029's driver rules
// enforce. If that changes, this fails rather than the suite quietly testing
// empty databases.
func TestInMemoryDatabaseSurvivesPoolReuse(t *testing.T) {
	t.Parallel()

	handle, driver := newSQLite(t)
	if got := handle.Stats().MaxOpenConnections; got != 1 {
		t.Fatalf("MaxOpenConnections = %d, want 1; in-memory tests are only valid on a single connection", got)
	}
	if _, err := db.Up(context.Background(), handle, driver); err != nil {
		t.Fatalf("Up: %v", err)
	}

	// Each query returns the connection to the pool before the next runs.
	for i := 0; i < 5; i++ {
		var n int
		if err := handle.QueryRow("SELECT COUNT(*) FROM events").Scan(&n); err != nil {
			t.Fatalf("query %d found the schema gone: %v", i, err)
		}
	}
}

// The runner reads the directory the driver resolves, so an engine's migrations
// and its connection cannot come apart. See ADR-029.
func TestUpUsesTheDriversOwnMigrationDirectory(t *testing.T) {
	t.Parallel()

	handle, _ := newSQLite(t)
	postgres, err := db.Resolve(db.Options{DriverName: "postgres"})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	if _, err := db.Up(context.Background(), handle, postgres); err == nil {
		t.Error("the Postgres migration set applied cleanly to SQLite")
	}
}

func TestUpOnAnUnresolvedDriverFailsClosed(t *testing.T) {
	t.Parallel()

	handle, _ := newSQLite(t)
	if _, err := db.Up(context.Background(), handle, db.Driver{Kind: "mysql", MigrationsDir: "mysql"}); err == nil {
		t.Error("Up accepted a driver with no migration directory")
	}
}

func equalStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func contains(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}
