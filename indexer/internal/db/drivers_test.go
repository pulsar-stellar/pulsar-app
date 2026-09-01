package db_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/pulsar-stellar/pulsar-app/indexer/internal/db"
)

func resolve(t *testing.T, name string) db.Driver {
	t.Helper()
	d, err := db.Resolve(db.Options{DriverName: name})
	if err != nil {
		t.Fatalf("Resolve(%q): %v", name, err)
	}
	return d
}

// SQLite ships with foreign key enforcement off. The events table references
// contracts with ON DELETE CASCADE, so without the pragma that clause parses,
// applies, and silently does nothing.
func TestSQLiteEnablesForeignKeys(t *testing.T) {
	t.Parallel()

	handle, err := db.Open(resolve(t, "sqlite"),
		db.ConnOptions{DSN: "file:" + filepath.Join(t.TempDir(), "fk.db")})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer handle.Close()

	var foreignKeys int
	if err := handle.QueryRow("PRAGMA foreign_keys").Scan(&foreignKeys); err != nil {
		t.Fatalf("PRAGMA foreign_keys: %v", err)
	}
	if foreignKeys != 1 {
		t.Errorf("foreign_keys = %d, want 1; ON DELETE CASCADE is inert without it", foreignKeys)
	}
}

func TestSQLiteAppliesTheRemainingPragmas(t *testing.T) {
	t.Parallel()

	handle, err := db.Open(resolve(t, "sqlite"),
		db.ConnOptions{DSN: "file:" + filepath.Join(t.TempDir(), "pragmas.db")})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer handle.Close()

	var journalMode string
	if err := handle.QueryRow("PRAGMA journal_mode").Scan(&journalMode); err != nil {
		t.Fatalf("PRAGMA journal_mode: %v", err)
	}
	if !strings.EqualFold(journalMode, "wal") {
		t.Errorf("journal_mode = %q, want wal", journalMode)
	}

	var busyTimeout int
	if err := handle.QueryRow("PRAGMA busy_timeout").Scan(&busyTimeout); err != nil {
		t.Fatalf("PRAGMA busy_timeout: %v", err)
	}
	if busyTimeout != 5000 {
		t.Errorf("busy_timeout = %d, want 5000", busyTimeout)
	}
}

// A pragma the operator set explicitly is left alone, because silently
// overriding it would be its own surprise.
func TestSQLiteDoesNotOverrideAnOperatorsPragma(t *testing.T) {
	t.Parallel()

	dsn := "file:" + filepath.Join(t.TempDir(), "override.db") + "?_pragma=busy_timeout(250)"
	handle, err := db.Open(resolve(t, "sqlite"), db.ConnOptions{DSN: dsn})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer handle.Close()

	var busyTimeout int
	if err := handle.QueryRow("PRAGMA busy_timeout").Scan(&busyTimeout); err != nil {
		t.Fatalf("PRAGMA busy_timeout: %v", err)
	}
	if busyTimeout != 250 {
		t.Errorf("busy_timeout = %d, want the operator's 250", busyTimeout)
	}

	// The pragmas the operator did not set are still applied.
	var foreignKeys int
	if err := handle.QueryRow("PRAGMA foreign_keys").Scan(&foreignKeys); err != nil {
		t.Fatalf("PRAGMA foreign_keys: %v", err)
	}
	if foreignKeys != 1 {
		t.Errorf("foreign_keys = %d, want 1", foreignKeys)
	}
}

func TestSQLitePinsThePoolToOneConnection(t *testing.T) {
	t.Parallel()

	handle, err := db.Open(resolve(t, "sqlite"),
		db.ConnOptions{DSN: "file:" + filepath.Join(t.TempDir(), "pool.db")})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer handle.Close()

	if got := handle.Stats().MaxOpenConnections; got != 1 {
		t.Errorf("MaxOpenConnections = %d, want 1", got)
	}
}

// ADR-031. A DSN that could complete an unencrypted connection is refused at
// startup, and the check is structural so it covers every DSN shape.
func TestPostgresRejectsDSNsThatPermitPlaintext(t *testing.T) {
	t.Parallel()

	for _, dsn := range []string{
		"postgres://u:p@h:5432/pulsar",                 // no sslmode means prefer
		"postgres://u:p@h:5432/pulsar?sslmode=prefer",  // explicit prefer
		"postgres://u:p@h:5432/pulsar?sslmode=allow",   // tries plaintext first
		"postgres://u:p@h:5432/pulsar?sslmode=disable", // never encrypts
		"host=h user=u password=p dbname=pulsar",       // keyword form, no sslmode
	} {
		t.Run(dsn, func(t *testing.T) {
			t.Parallel()

			handle, err := db.Open(resolve(t, "postgres"), db.ConnOptions{DSN: dsn})
			if err == nil {
				_ = handle.Close()
				t.Fatalf("Open(%q) succeeded, want a refusal", dsn)
			}
			if !strings.Contains(err.Error(), "sslmode") {
				t.Errorf("error %q does not mention sslmode", err.Error())
			}
			if handle != nil {
				t.Error("Open returned a handle alongside an error")
			}
		})
	}
}

func TestPostgresAcceptsEncryptedDSNs(t *testing.T) {
	t.Parallel()

	for _, dsn := range []string{
		"postgres://u:p@h:5432/pulsar?sslmode=require",
		"postgres://u:p@h:5432/pulsar?sslmode=verify-ca",
		"postgres://u:p@h:5432/pulsar?sslmode=verify-full",
		"host=h user=u password=p dbname=pulsar sslmode=require",
	} {
		t.Run(dsn, func(t *testing.T) {
			t.Parallel()

			handle, err := db.Open(resolve(t, "postgres"), db.ConnOptions{DSN: dsn})
			if err != nil {
				t.Fatalf("Open(%q): unexpected error: %v", dsn, err)
			}
			defer handle.Close()

			if got := handle.Stats().MaxOpenConnections; got != 10 {
				t.Errorf("MaxOpenConnections = %d, want the postgres default 10", got)
			}
		})
	}
}

// The escape hatch exists for a local Docker Postgres with no certificate, and
// must work, or operators will reach for something worse.
func TestPostgresAllowsPlaintextOnlyWhenExplicitlyPermitted(t *testing.T) {
	t.Parallel()

	dsn := "postgres://u:p@h:5432/pulsar?sslmode=disable"

	if _, err := db.Open(resolve(t, "postgres"), db.ConnOptions{DSN: dsn}); err == nil {
		t.Fatal("expected a refusal without the opt-out")
	}

	handle, err := db.Open(resolve(t, "postgres"),
		db.ConnOptions{DSN: dsn, AllowInsecureTLS: true})
	if err != nil {
		t.Fatalf("Open with AllowInsecureTLS: unexpected error: %v", err)
	}
	_ = handle.Close()
}

// A Postgres DSN embeds the password. No error this package produces may
// contain it, whatever the failure was.
func TestErrorsNeverCarryTheDSN(t *testing.T) {
	t.Parallel()

	const password = "sup3rs3cr3t"
	dsns := []string{
		"postgres://admin:" + password + "@db.internal:5432/pulsar",
		"postgres://admin:" + password + "@db.internal:5432/pulsar?sslmode=prefer",
		"host=db.internal user=admin password=" + password + " dbname=pulsar",
		"::not a url at all::" + password,
	}

	for _, dsn := range dsns {
		for _, driverName := range []string{"postgres", "sqlite"} {
			_, err := db.Open(resolve(t, driverName), db.ConnOptions{DSN: dsn})
			if err == nil {
				continue
			}
			if strings.Contains(err.Error(), password) {
				t.Errorf("%s error leaked the password: %q", driverName, err.Error())
			}
			if strings.Contains(err.Error(), "admin") {
				t.Errorf("%s error leaked the username: %q", driverName, err.Error())
			}
		}
	}
}

func TestOpenRejectsAnEmptyDSN(t *testing.T) {
	t.Parallel()

	for _, driverName := range []string{"sqlite", "postgres"} {
		t.Run(driverName, func(t *testing.T) {
			t.Parallel()

			for _, dsn := range []string{"", "   "} {
				if _, err := db.Open(resolve(t, driverName), db.ConnOptions{DSN: dsn}); err == nil {
					t.Errorf("Open(%q) with an empty DSN succeeded, want an error", dsn)
				}
			}
		})
	}
}

// Open's switch mirrors Resolve's and fails closed, so a Driver built by hand
// rather than by Resolve cannot reach an unsupported engine.
func TestOpenFailsClosedOnAnUnresolvedDriver(t *testing.T) {
	t.Parallel()

	_, err := db.Open(db.Driver{Kind: "mysql", PoolMax: 1, PoolMin: 1},
		db.ConnOptions{DSN: "whatever"})
	if err == nil {
		t.Fatal("expected an error for an unsupported Kind")
	}
	for _, want := range db.KindNames() {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not list %q", err.Error(), want)
		}
	}
}
