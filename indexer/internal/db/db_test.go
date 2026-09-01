package db_test

import (
	"strings"
	"testing"

	"github.com/pulsar-stellar/pulsar-app/indexer/internal/db"
)

func TestResolveReturnsPerEngineDefaults(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		wantKind db.Kind
		wantDir  string
		wantMax  int
		wantMin  int
	}{
		{"sqlite", db.KindSQLite, "sqlite", 1, 1},
		{"postgres", db.KindPostgres, "postgres", 10, 2},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			d, err := db.Resolve(db.Options{DriverName: c.name})
			if err != nil {
				t.Fatalf("Resolve(%q): unexpected error: %v", c.name, err)
			}
			if d.Kind != c.wantKind {
				t.Errorf("Kind = %q, want %q", d.Kind, c.wantKind)
			}
			if d.MigrationsDir != c.wantDir {
				t.Errorf("MigrationsDir = %q, want %q", d.MigrationsDir, c.wantDir)
			}
			if d.PoolMax != c.wantMax || d.PoolMin != c.wantMin {
				t.Errorf("pool = (%d, %d), want (%d, %d)", d.PoolMax, d.PoolMin, c.wantMax, c.wantMin)
			}
		})
	}
}

// Each engine reads its own migration directory, because SQLite accepts the
// Postgres spelling of an autoincrementing primary key and then writes NULL
// ids. See ADR-029.
func TestResolveGivesEachEngineItsOwnMigrationDirectory(t *testing.T) {
	t.Parallel()

	sqlite, err := db.Resolve(db.Options{DriverName: "sqlite"})
	if err != nil {
		t.Fatalf("Resolve sqlite: %v", err)
	}
	postgres, err := db.Resolve(db.Options{DriverName: "postgres"})
	if err != nil {
		t.Fatalf("Resolve postgres: %v", err)
	}

	if sqlite.MigrationsDir == postgres.MigrationsDir {
		t.Errorf("both engines resolved to migrations dir %q; ADR-029 requires separate directories",
			sqlite.MigrationsDir)
	}
}

// An unrecognised driver must fail rather than fall back. Quietly opening
// SQLite because someone typoed "postgress" would write production data to a
// local file that vanishes with the container.
func TestResolveFailsClosedOnUnknownDrivers(t *testing.T) {
	t.Parallel()

	for _, name := range []string{
		"",
		"postgress",
		"Postgres",
		"SQLITE",
		"mysql",
		"sqlite3",
		" sqlite",
	} {
		t.Run("driver="+name, func(t *testing.T) {
			t.Parallel()

			d, err := db.Resolve(db.Options{DriverName: name})
			if err == nil {
				t.Fatalf("Resolve(%q) returned %+v, want an error", name, d)
			}
			if d != (db.Driver{}) {
				t.Errorf("Resolve returned %+v alongside an error, want the zero Driver", d)
			}
			for _, want := range db.KindNames() {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not list the supported driver %q", err.Error(), want)
				}
			}
		})
	}
}

func TestResolveAppliesPoolOverridesForPostgres(t *testing.T) {
	t.Parallel()

	d, err := db.Resolve(db.Options{DriverName: "postgres", PoolMax: 25, PoolMin: 5})
	if err != nil {
		t.Fatalf("Resolve: unexpected error: %v", err)
	}
	if d.PoolMax != 25 || d.PoolMin != 5 {
		t.Errorf("pool = (%d, %d), want (25, 5)", d.PoolMax, d.PoolMin)
	}
}

func TestResolveKeepsDefaultsWhenOverridesAreZero(t *testing.T) {
	t.Parallel()

	d, err := db.Resolve(db.Options{DriverName: "postgres", PoolMax: 0, PoolMin: 0})
	if err != nil {
		t.Fatalf("Resolve: unexpected error: %v", err)
	}
	if d.PoolMax != 10 || d.PoolMin != 2 {
		t.Errorf("pool = (%d, %d), want the postgres defaults (10, 2)", d.PoolMax, d.PoolMin)
	}
}

// SQLite permits one writer at a time. A larger pool does not produce
// concurrency, it produces SQLITE_BUSY spread across callers, so the override
// is rejected rather than accepted and quietly ignored.
func TestResolveRejectsSQLitePoolsOtherThanOne(t *testing.T) {
	t.Parallel()

	for _, opts := range []db.Options{
		{DriverName: "sqlite", PoolMax: 10},
		{DriverName: "sqlite", PoolMin: 2},
		{DriverName: "sqlite", PoolMax: 4, PoolMin: 2},
	} {
		_, err := db.Resolve(opts)
		if err == nil {
			t.Errorf("Resolve(%+v): expected an error", opts)
			continue
		}
		if !strings.Contains(err.Error(), "sqlite") {
			t.Errorf("error %q does not mention sqlite", err.Error())
		}
	}

	// The one value SQLite does allow is accepted.
	d, err := db.Resolve(db.Options{DriverName: "sqlite", PoolMax: 1, PoolMin: 1})
	if err != nil {
		t.Fatalf("Resolve with the allowed sqlite pool: unexpected error: %v", err)
	}
	if d.PoolMax != 1 || d.PoolMin != 1 {
		t.Errorf("pool = (%d, %d), want (1, 1)", d.PoolMax, d.PoolMin)
	}
}

func TestResolveRejectsIncoherentPoolBounds(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		opts db.Options
		want string
	}{
		{"min above max", db.Options{DriverName: "postgres", PoolMax: 2, PoolMin: 5}, "greater than"},
		{"negative max", db.Options{DriverName: "postgres", PoolMax: -1}, "negative"},
		{"negative min", db.Options{DriverName: "postgres", PoolMin: -1}, "negative"},
		{"min above default max", db.Options{DriverName: "postgres", PoolMin: 50}, "greater than"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			_, err := db.Resolve(c.opts)
			if err == nil {
				t.Fatalf("Resolve(%+v): expected an error", c.opts)
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("error %q does not contain %q", err.Error(), c.want)
			}
		})
	}
}

// A Postgres DSN embeds the password, so a Driver must be safe to log. See
// ADR-031.
func TestDriverStringCarriesNoDSN(t *testing.T) {
	t.Parallel()

	d, err := db.Resolve(db.Options{DriverName: "postgres"})
	if err != nil {
		t.Fatalf("Resolve: unexpected error: %v", err)
	}

	got := d.String()
	for _, secret := range []string{"password", "pass", "@", "postgres://"} {
		if strings.Contains(got, secret) {
			t.Errorf("Driver.String() = %q, which contains %q", got, secret)
		}
	}
	if !strings.Contains(got, "driver=postgres") {
		t.Errorf("Driver.String() = %q, want it to name the driver", got)
	}
}

func TestKindNamesListsEverySupportedDriver(t *testing.T) {
	t.Parallel()

	names := db.KindNames()
	if len(names) != 2 {
		t.Fatalf("KindNames() = %v, want two entries", names)
	}
	for _, name := range names {
		if _, err := db.Resolve(db.Options{DriverName: name}); err != nil {
			t.Errorf("KindNames() lists %q but Resolve rejects it: %v", name, err)
		}
	}
}
