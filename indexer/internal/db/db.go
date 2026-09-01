// Package db resolves the configured database driver and the settings that
// differ between engines.
//
// Resolution is separate from connecting. Everything here is a pure decision
// about which engine was asked for, which migration directory it reads, and
// what its pool bounds are, so it is settled and validated before anything
// opens a socket or a file.
package db

import (
	"database/sql"
	"fmt"
	"strings"
)

// Kind is the set of engines the indexer supports. It is closed: a name that
// is not one of these is an error, never a default.
type Kind string

const (
	KindSQLite   Kind = "sqlite"
	KindPostgres Kind = "postgres"
)

// Pool bounds per engine.
//
// SQLite takes a single connection because it allows one writer at a time.
// A larger pool does not buy concurrency, it converts the contention into
// SQLITE_BUSY errors spread across callers.
//
// Postgres takes 10, which is well inside the default 100 max_connections a
// managed instance offers while leaving room for migrations, a second indexer
// instance, and an operator's psql session.
const (
	sqlitePoolMax   = 1
	sqlitePoolMin   = 1
	postgresPoolMax = 10
	postgresPoolMin = 2
)

// Driver is a resolved engine choice: which engine, where its migrations live,
// and the pool bounds to open it with.
type Driver struct {
	Kind Kind

	// MigrationsDir is the subdirectory of indexer/migrations holding this
	// engine's migration files. The two engines have separate directories
	// because they spell an autoincrementing 64-bit primary key differently
	// and SQLite accepts the Postgres spelling while writing NULL ids.
	// See ADR-029.
	MigrationsDir string

	PoolMax int
	PoolMin int
}

// Options are the configured values that influence resolution. A zero PoolMax
// or PoolMin means the operator did not set one and the engine's default
// applies.
type Options struct {
	DriverName string
	PoolMax    int
	PoolMin    int
}

// Resolve turns a configured driver name into a Driver.
//
// The switch is explicit and its default returns an error, so an unrecognised
// name fails closed. There is no fallback engine: silently opening SQLite
// because someone typoed "postgress" would write production data to a local
// file that disappears with the container.
func Resolve(opts Options) (Driver, error) {
	var d Driver

	switch Kind(opts.DriverName) {
	case KindSQLite:
		d = Driver{
			Kind:          KindSQLite,
			MigrationsDir: string(KindSQLite),
			PoolMax:       sqlitePoolMax,
			PoolMin:       sqlitePoolMin,
		}
	case KindPostgres:
		d = Driver{
			Kind:          KindPostgres,
			MigrationsDir: string(KindPostgres),
			PoolMax:       postgresPoolMax,
			PoolMin:       postgresPoolMin,
		}
	default:
		return Driver{}, fmt.Errorf(
			"database driver %q is not supported; expected one of %s",
			opts.DriverName, strings.Join(KindNames(), ", "))
	}

	if err := d.applyPoolOverrides(opts); err != nil {
		return Driver{}, err
	}
	return d, nil
}

// applyPoolOverrides layers the operator's pool bounds over the engine's
// defaults, rejecting combinations the engine cannot honour.
func (d *Driver) applyPoolOverrides(opts Options) error {
	if opts.PoolMax < 0 || opts.PoolMin < 0 {
		return fmt.Errorf("pool bounds must not be negative, got max %d and min %d", opts.PoolMax, opts.PoolMin)
	}

	if opts.PoolMax > 0 {
		if d.Kind == KindSQLite && opts.PoolMax != sqlitePoolMax {
			return fmt.Errorf(
				"pool max is %d, but the sqlite driver allows exactly %d because SQLite permits one writer at a time; a larger pool produces SQLITE_BUSY rather than concurrency",
				opts.PoolMax, sqlitePoolMax)
		}
		d.PoolMax = opts.PoolMax
	}

	if opts.PoolMin > 0 {
		if d.Kind == KindSQLite && opts.PoolMin != sqlitePoolMin {
			return fmt.Errorf(
				"pool min is %d, but the sqlite driver allows exactly %d",
				opts.PoolMin, sqlitePoolMin)
		}
		d.PoolMin = opts.PoolMin
	}

	if d.PoolMin > d.PoolMax {
		return fmt.Errorf("pool min %d is greater than pool max %d", d.PoolMin, d.PoolMax)
	}
	return nil
}

// KindNames lists the supported driver names, for error messages and for the
// config layer's validation.
func KindNames() []string {
	return []string{string(KindSQLite), string(KindPostgres)}
}

// String makes a Driver safe to log. It deliberately carries no DSN: a
// Postgres connection string embeds the password, so it is never logged at any
// level. See ADR-031.
func (d Driver) String() string {
	return fmt.Sprintf("driver=%s migrations=%s pool_max=%d pool_min=%d",
		d.Kind, d.MigrationsDir, d.PoolMax, d.PoolMin)
}

// ConnOptions are the values needed to actually connect, as opposed to the
// values Resolve needs to decide which engine is in play.
type ConnOptions struct {
	DSN string

	// AllowInsecureTLS permits a Postgres DSN that could complete an
	// unencrypted connection. Local development only. See ADR-031.
	AllowInsecureTLS bool
}

// Open connects to the database the Driver describes.
//
// The switch mirrors Resolve's and fails closed the same way, so a Driver that
// was built by hand rather than by Resolve cannot reach an engine that is not
// supported. The returned handle is lazy: both engines validate what they can
// without a round trip and leave the first connection to the caller's Ping.
func Open(d Driver, opts ConnOptions) (*sql.DB, error) {
	switch d.Kind {
	case KindSQLite:
		return openSQLite(d, opts.DSN)
	case KindPostgres:
		return openPostgres(d, opts.DSN, opts.AllowInsecureTLS)
	default:
		return nil, fmt.Errorf(
			"database driver %q is not supported; expected one of %s",
			d.Kind, strings.Join(KindNames(), ", "))
	}
}
