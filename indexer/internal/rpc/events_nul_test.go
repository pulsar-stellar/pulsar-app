package rpc

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"testing"
	"time"

	protocol "github.com/stellar/go-stellar-sdk/protocols/rpc"
	"github.com/stellar/go-stellar-sdk/xdr"

	"github.com/pulsar-stellar/pulsar-app/indexer/internal/db"
	"github.com/pulsar-stellar/pulsar-app/indexer/internal/decoder"
	"github.com/pulsar-stellar/pulsar-app/indexer/internal/store"
)

// engine is one storage backend the poller commits through.
type engine struct {
	name    string
	handle  *sql.DB
	dialect store.Dialect
}

// engines returns SQLite always, and Postgres when PULSAR_TEST_POSTGRES_DSN is
// set (CI sets it). Each is freshly migrated with the showcase registered.
func engines(t *testing.T) []engine {
	t.Helper()
	list := []engine{{"sqlite", migrated(t), store.DialectSQLite}}

	dsn := os.Getenv("PULSAR_TEST_POSTGRES_DSN")
	if dsn == "" {
		return list
	}
	driver, err := db.Resolve(db.Options{DriverName: "postgres"})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	handle, err := db.Open(driver, db.ConnOptions{
		DSN:              dsn,
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
	if _, err := store.NewContracts(handle).Register(ctx, showcase); err != nil {
		t.Fatalf("Register: %v", err)
	}
	return append(list, engine{"postgres", handle, store.DialectPostgres})
}

// stringValueBase64 encodes an ScString holding raw, as RPC sends a value.
func stringValueBase64(t *testing.T, raw string) string {
	t.Helper()
	s := xdr.ScString(raw)
	encoded, err := xdr.MarshalBase64(xdr.ScVal{Type: xdr.ScValTypeScvString, Str: &s})
	if err != nil {
		t.Fatalf("encoding the ScString: %v", err)
	}
	return encoded
}

// A contract can emit a string holding NUL, and Postgres jsonb rejects \u0000.
// Because a batch and its cursor commit together, that one event used to fail
// the commit on every tick and stall the contract for good. The tick must now
// commit past it, storing the event with the NUL replaced and its raw XDR as
// RPC sent it (ADR-053), and store the same thing on both engines.
func TestATickCommitsPastAStringHoldingNUL(t *testing.T) {
	for _, e := range engines(t) {
		t.Run(e.name, func(t *testing.T) {
			ctx := context.Background()
			if err := store.NewContracts(e.handle).SetProgress(ctx, showcase, 4430000); err != nil {
				t.Fatalf("SetProgress: %v", err)
			}

			withNUL := event(t, 4430001, 0, "deposit_topic0", "deposit_data", true)
			withNUL.ValueXDR = stringValueBase64(t, "a\x00b")
			after := event(t, 4430002, 0, "transfer_topic0", "transfer_data", true)
			caller := &mockCaller{eventsQueue: []eventsReply{{resp: protocol.GetEventsResponse{
				Events:       []protocol.EventInfo{withNUL, after},
				Cursor:       endOfWindowCursor(4430003),
				LatestLedger: 4430003,
			}}}}

			p := NewPoller(caller, decoder.New(), e.handle, store.NewContracts(e.handle), e.dialect,
				10*time.Millisecond, 100, nil)
			if err := p.tickOnce(ctx, showcase); err != nil {
				t.Fatalf("tickOnce: %v", err)
			}

			if got := progress(t, e.handle); got != 4430003 {
				t.Errorf("progress = %d, want 4430003: the cursor did not move past the batch", got)
			}
			page, err := store.NewEvents(e.handle, e.dialect).Query(ctx, store.EventQuery{ContractID: showcase})
			if err != nil {
				t.Fatalf("Query: %v", err)
			}
			if len(page.Events) != 2 {
				t.Fatalf("stored %d events, want 2: the NUL event must be kept, not dropped", len(page.Events))
			}
			stored := page.Events[0]
			if stored.RawData != withNUL.ValueXDR {
				t.Errorf("raw_data = %q, want the XDR RPC sent, %q", stored.RawData, withNUL.ValueXDR)
			}
			if !strings.Contains(string(stored.DataJSON), "a�b") || strings.Contains(string(stored.DataJSON), `\u0000`) {
				t.Errorf("data_json = %s, want the string a\\ufffdb", stored.DataJSON)
			}
		})
	}
}
