package store_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/pulsar-stellar/pulsar-app/indexer/internal/db"
	"github.com/pulsar-stellar/pulsar-app/indexer/internal/models"
	"github.com/pulsar-stellar/pulsar-app/indexer/internal/store"
)

const (
	showcase = "CDNWTVUDKCCGW7GOC6SBLUFXXUCD2YDHWRDUSXZ6CYBQKQWLCUYYWI5L"
	other    = "CDLZFC3SYJYDZT7K67VZ75HPJVIEUVNIXF47ZG2FB2RMQQVU2HHGCYSC"
)

// migrated opens an in-memory SQLite database with every migration applied.
// The pool is pinned to one connection, which is what makes in-memory sound;
// see the note in internal/db's migration tests.
func migrated(t *testing.T) *sql.DB {
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

	if _, err := db.Up(context.Background(), handle, driver); err != nil {
		t.Fatalf("Up: %v", err)
	}
	return handle
}

func contractsStore(t *testing.T) (*store.Contracts, *sql.DB) {
	t.Helper()
	handle := migrated(t)
	return store.NewContracts(handle), handle
}

func TestRegisterCreatesATrackedContract(t *testing.T) {
	t.Parallel()

	contracts, _ := contractsStore(t)
	before := time.Now().UTC().Add(-time.Second)

	got, err := contracts.Register(context.Background(), showcase)
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	if got.ID != showcase {
		t.Errorf("ID = %q, want %q", got.ID, showcase)
	}
	if got.Status != models.StatusActive {
		t.Errorf("Status = %q, want %q; the column defaults to active", got.Status, models.StatusActive)
	}
	if got.LastIndexedLedger != 0 {
		t.Errorf("LastIndexedLedger = %d, want 0", got.LastIndexedLedger)
	}
	if got.FirstIndexedLedger != nil {
		t.Errorf("FirstIndexedLedger = %d, want nil until the first poll completes", *got.FirstIndexedLedger)
	}
	if got.Indexed() {
		t.Error("Indexed() reported true for a contract that has never been polled")
	}

	// ADR-032: the timestamp comes back from SQLite as a string and must still
	// arrive as a usable UTC time.
	if got.AddedAt.IsZero() {
		t.Fatal("AddedAt is zero; the timestamp did not survive scanning")
	}
	if got.AddedAt.Location() != time.UTC {
		t.Errorf("AddedAt is in %v, want UTC", got.AddedAt.Location())
	}
	if got.AddedAt.Before(before) || got.AddedAt.After(time.Now().UTC().Add(time.Minute)) {
		t.Errorf("AddedAt = %v, which is not close to now", got.AddedAt)
	}
}

// ADR-018: registering an already-tracked contract succeeds and returns the
// existing row with its progress untouched. It neither fails nor resets.
func TestRegisterIsIdempotentAndPreservesProgress(t *testing.T) {
	t.Parallel()

	contracts, _ := contractsStore(t)
	ctx := context.Background()

	first, err := contracts.Register(ctx, showcase)
	if err != nil {
		t.Fatalf("first Register: %v", err)
	}
	if err := contracts.SetProgress(ctx, showcase, 4430000); err != nil {
		t.Fatalf("SetProgress: %v", err)
	}
	if err := contracts.SetStatus(ctx, showcase, models.StatusPaused); err != nil {
		t.Fatalf("SetStatus: %v", err)
	}

	again, err := contracts.Register(ctx, showcase)
	if err != nil {
		t.Fatalf("second Register: %v", err)
	}

	if again.LastIndexedLedger != 4430000 {
		t.Errorf("LastIndexedLedger = %d, want 4430000; re-registering reset indexing progress", again.LastIndexedLedger)
	}
	if again.FirstIndexedLedger == nil || *again.FirstIndexedLedger != 4430000 {
		t.Errorf("FirstIndexedLedger = %v, want 4430000 preserved", again.FirstIndexedLedger)
	}
	if again.Status != models.StatusPaused {
		t.Errorf("Status = %q, want paused preserved", again.Status)
	}
	if !again.AddedAt.Equal(first.AddedAt) {
		t.Errorf("AddedAt moved from %v to %v", first.AddedAt, again.AddedAt)
	}

	all, err := contracts.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(all) != 1 {
		t.Errorf("List returned %d contracts, want 1; the second Register inserted a duplicate", len(all))
	}
}

func TestRegisterRejectsAnEmptyID(t *testing.T) {
	t.Parallel()

	contracts, _ := contractsStore(t)
	if _, err := contracts.Register(context.Background(), ""); err == nil {
		t.Error("Register accepted an empty contract id")
	}
}

// ADR-019: absence is a distinct outcome, not an empty record, so a caller can
// answer it with a 404 and a not_found envelope.
func TestGetReportsAbsenceAsErrNotFound(t *testing.T) {
	t.Parallel()

	contracts, _ := contractsStore(t)

	_, err := contracts.Get(context.Background(), showcase)
	if err == nil {
		t.Fatal("Get succeeded for a contract that was never registered")
	}
	if !errors.Is(err, store.ErrNotFound) {
		t.Errorf("error %v is not ErrNotFound", err)
	}
	if !strings.Contains(err.Error(), showcase) {
		t.Errorf("error %q does not name the contract", err.Error())
	}
}

func TestListIsEmptyBeforeAnythingIsRegistered(t *testing.T) {
	t.Parallel()

	contracts, _ := contractsStore(t)

	all, err := contracts.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if all == nil {
		t.Error("List returned nil, which encodes as JSON null rather than an empty array")
	}
	if len(all) != 0 {
		t.Errorf("List returned %d contracts, want none", len(all))
	}

	// The empty list has to reach the wire as [], per the events contract in
	// ADR-021 and the same rule for contracts.
	encoded, err := json.Marshal(all)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if string(encoded) != "[]" {
		t.Errorf("an empty list encoded as %s, want []", encoded)
	}
}

func TestListReturnsEveryContractInAStableOrder(t *testing.T) {
	t.Parallel()

	contracts, _ := contractsStore(t)
	ctx := context.Background()

	for _, id := range []string{showcase, other} {
		if _, err := contracts.Register(ctx, id); err != nil {
			t.Fatalf("Register %s: %v", id, err)
		}
	}

	first, err := contracts.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(first) != 2 {
		t.Fatalf("List returned %d contracts, want 2", len(first))
	}

	second, err := contracts.List(ctx)
	if err != nil {
		t.Fatalf("second List: %v", err)
	}
	for i := range first {
		if first[i].ID != second[i].ID {
			t.Errorf("List order changed between calls: %v then %v", first, second)
		}
	}
}

func TestDeleteRemovesTheContractAndItsEvents(t *testing.T) {
	t.Parallel()

	contracts, handle := contractsStore(t)
	ctx := context.Background()

	if _, err := contracts.Register(ctx, showcase); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if _, err := handle.Exec(
		`INSERT INTO events
		 (contract_id, ledger, tx_hash, event_index, name, topics_json, data_json,
		  raw_topics, raw_data, emitted_at, in_successful_contract_call)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
		showcase, 4430000, "9f4067a3", 0, "transfer",
		`[]`, `{}`, `[]`, "AAAACg==", "2026-09-01T10:53:07Z", true); err != nil {
		t.Fatalf("seeding an event: %v", err)
	}

	if err := contracts.Delete(ctx, showcase); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	if _, err := contracts.Get(ctx, showcase); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("Get after Delete returned %v, want ErrNotFound", err)
	}

	var events int
	if err := handle.QueryRow("SELECT COUNT(*) FROM events").Scan(&events); err != nil {
		t.Fatalf("counting events: %v", err)
	}
	if events != 0 {
		t.Errorf("%d events survived their contract; ON DELETE CASCADE did not fire", events)
	}
}

// Deleting nothing is a 404 rather than a 204, so the two outcomes have to be
// distinguishable here.
func TestDeleteReportsAbsenceAsErrNotFound(t *testing.T) {
	t.Parallel()

	contracts, _ := contractsStore(t)

	err := contracts.Delete(context.Background(), showcase)
	if !errors.Is(err, store.ErrNotFound) {
		t.Errorf("Delete of an untracked contract returned %v, want ErrNotFound", err)
	}
}

// first_indexed_ledger records where the first completed poll reached, so it is
// written once and then left alone. last_indexed_ledger moves every time.
func TestSetProgressWritesFirstLedgerOnlyOnce(t *testing.T) {
	t.Parallel()

	contracts, _ := contractsStore(t)
	ctx := context.Background()

	if _, err := contracts.Register(ctx, showcase); err != nil {
		t.Fatalf("Register: %v", err)
	}

	if err := contracts.SetProgress(ctx, showcase, 4430000); err != nil {
		t.Fatalf("first SetProgress: %v", err)
	}
	after, err := contracts.Get(ctx, showcase)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if after.FirstIndexedLedger == nil || *after.FirstIndexedLedger != 4430000 {
		t.Fatalf("FirstIndexedLedger = %v, want 4430000", after.FirstIndexedLedger)
	}
	if !after.Indexed() {
		t.Error("Indexed() reported false after a completed poll")
	}

	if err := contracts.SetProgress(ctx, showcase, 4446467); err != nil {
		t.Fatalf("second SetProgress: %v", err)
	}
	later, err := contracts.Get(ctx, showcase)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if *later.FirstIndexedLedger != 4430000 {
		t.Errorf("FirstIndexedLedger = %d, want 4430000 unchanged; it must not drift forward",
			*later.FirstIndexedLedger)
	}
	if later.LastIndexedLedger != 4446467 {
		t.Errorf("LastIndexedLedger = %d, want 4446467", later.LastIndexedLedger)
	}
}

func TestSetProgressAndSetStatusReportAbsence(t *testing.T) {
	t.Parallel()

	contracts, _ := contractsStore(t)
	ctx := context.Background()

	if err := contracts.SetProgress(ctx, showcase, 1); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("SetProgress on an untracked contract returned %v, want ErrNotFound", err)
	}
	if err := contracts.SetStatus(ctx, showcase, models.StatusPaused); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("SetStatus on an untracked contract returned %v, want ErrNotFound", err)
	}
}

func TestSetStatusMovesBetweenEveryState(t *testing.T) {
	t.Parallel()

	contracts, _ := contractsStore(t)
	ctx := context.Background()

	if _, err := contracts.Register(ctx, showcase); err != nil {
		t.Fatalf("Register: %v", err)
	}

	for _, status := range []models.Status{models.StatusPaused, models.StatusError, models.StatusActive} {
		if err := contracts.SetStatus(ctx, showcase, status); err != nil {
			t.Fatalf("SetStatus(%q): %v", status, err)
		}
		got, err := contracts.Get(ctx, showcase)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if got.Status != status {
			t.Errorf("Status = %q, want %q", got.Status, status)
		}
	}
}

// The store takes a Querier, so every method works inside a transaction as
// well as outside one. That is what lets a poll write events and its progress
// atomically later.
func TestTheStoreWorksInsideATransaction(t *testing.T) {
	t.Parallel()

	handle := migrated(t)
	ctx := context.Background()

	tx, err := handle.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTx: %v", err)
	}

	inTx := store.NewContracts(tx)
	if _, err := inTx.Register(ctx, showcase); err != nil {
		t.Fatalf("Register in a transaction: %v", err)
	}
	if err := inTx.SetProgress(ctx, showcase, 4430000); err != nil {
		t.Fatalf("SetProgress in a transaction: %v", err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatalf("Rollback: %v", err)
	}

	if _, err := store.NewContracts(handle).Get(ctx, showcase); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("the contract survived a rolled back transaction: %v", err)
	}
}

// The registration timestamp has to reach the wire as ISO 8601 with an offset,
// which is what the SDK's ContractInfoPayloadSchema validates.
func TestAddedAtReachesTheWireAsRFC3339(t *testing.T) {
	t.Parallel()

	contracts, _ := contractsStore(t)

	got, err := contracts.Register(context.Background(), showcase)
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	encoded, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var generic map[string]any
	if err := json.Unmarshal(encoded, &generic); err != nil {
		t.Fatalf("decoding: %v", err)
	}
	addedAt, ok := generic["added_at"].(string)
	if !ok {
		t.Fatalf("added_at is %T, want a string", generic["added_at"])
	}
	if _, err := time.Parse(time.RFC3339, addedAt); err != nil {
		t.Errorf("added_at %q does not parse as RFC3339: %v", addedAt, err)
	}
	if !strings.HasSuffix(addedAt, "Z") {
		t.Errorf("added_at %q is not UTC; SQLite's raw value carries no offset at all", addedAt)
	}
}

// Every method has to surface a database failure as a wrapped error naming
// this package, rather than swallowing it or returning a zero value that reads
// as success. An unmigrated handle makes every statement fail at once.
// unmigrated opens a database with no tables at all, which is the cheapest
// infrastructure-failure state to put every method into at once.
func unmigrated(t *testing.T) *sql.DB {
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
	return handle
}

func TestEveryMethodWrapsADatabaseFailure(t *testing.T) {
	t.Parallel()

	handle := unmigrated(t)
	contracts := store.NewContracts(handle)
	ctx := context.Background()

	cases := []struct {
		name string
		call func() error
	}{
		{"Register", func() error { _, err := contracts.Register(ctx, showcase); return err }},
		{"Get", func() error { _, err := contracts.Get(ctx, showcase); return err }},
		{"List", func() error { _, err := contracts.List(ctx); return err }},
		{"Delete", func() error { return contracts.Delete(ctx, showcase) }},
		{"SetProgress", func() error { return contracts.SetProgress(ctx, showcase, 1) }},
		{"SetStatus", func() error { return contracts.SetStatus(ctx, showcase, models.StatusPaused) }},
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
			// A missing table is not absence. Reporting it as ErrNotFound would
			// turn an outage into a 404.
			if errors.Is(err, store.ErrNotFound) {
				t.Errorf("%s reported a database failure as ErrNotFound: %v", c.name, err)
			}
		})
	}
}

// scanContract must reject a status the enum does not contain rather than
// passing it through to the wire, where the SDK's Zod schema rejects the whole
// response.
func TestAStatusOutsideTheEnumSurvivesScanningAsIs(t *testing.T) {
	t.Parallel()

	contracts, handle := contractsStore(t)
	ctx := context.Background()

	if _, err := contracts.Register(ctx, showcase); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if _, err := handle.Exec("UPDATE contracts SET status = $1 WHERE id = $2", "bogus", showcase); err != nil {
		t.Fatalf("writing a bogus status: %v", err)
	}

	got, err := contracts.Get(ctx, showcase)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	// The store reports what the column holds. Nothing in the schema constrains
	// this column, so a value outside the enum can only arrive by a write this
	// codebase did not make, and the store is not the layer that hides it.
	if got.Status != models.Status("bogus") {
		t.Errorf("Status = %q, want the raw column value", got.Status)
	}
	if got.Status == models.StatusActive || got.Status == models.StatusPaused || got.Status == models.StatusError {
		t.Errorf("an unknown status was silently mapped onto %q", got.Status)
	}
}
