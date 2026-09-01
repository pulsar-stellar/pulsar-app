package db

import (
	"database/sql"
	"fmt"
	"net/url"
	"strings"

	// Registers the "sqlite" driver. Pure Go, so CGO_ENABLED=0 still builds.
	// See ADR-008.
	_ "modernc.org/sqlite"
)

// sqliteDriverName is what modernc.org/sqlite registers with database/sql.
const sqliteDriverName = "sqlite"

// Pragmas applied to every SQLite connection.
//
// foreign_keys is the load-bearing one. SQLite ships with foreign key
// enforcement OFF, verified as PRAGMA foreign_keys reporting 0 on a fresh
// connection, so the events table's ON DELETE CASCADE reference to contracts
// would parse, apply, and then do nothing at all. Deleting a contract would
// leave its events behind pointing at a row that no longer exists.
//
// journal_mode=WAL lets the HTTP read path work while the poller writes.
// busy_timeout gives a blocked statement five seconds to acquire the write
// lock instead of failing immediately with SQLITE_BUSY.
var sqlitePragmas = []string{
	"foreign_keys(1)",
	"journal_mode(WAL)",
	"busy_timeout(5000)",
}

// openSQLite opens the SQLite database at dsn.
//
// The pool is pinned to one connection. SQLite permits a single writer, and a
// larger pool turns that constraint into SQLITE_BUSY errors distributed across
// callers rather than into throughput. Resolve already rejects any other pool
// size for this engine; this is where the constraint is actually applied.
func openSQLite(d Driver, dsn string) (*sql.DB, error) {
	if strings.TrimSpace(dsn) == "" {
		return nil, fmt.Errorf("sqlite: the database URL is empty")
	}

	withPragmas, err := appendPragmas(dsn)
	if err != nil {
		return nil, fmt.Errorf("sqlite: %w", err)
	}

	handle, err := sql.Open(sqliteDriverName, withPragmas)
	if err != nil {
		return nil, fmt.Errorf("sqlite: opening %s: %w", redactDSN(dsn), err)
	}

	handle.SetMaxOpenConns(d.PoolMax)
	handle.SetMaxIdleConns(d.PoolMin)

	return handle, nil
}

// appendPragmas adds the pragmas to the DSN's query string without disturbing
// anything the operator already set. A pragma the operator specified wins,
// because overriding it silently would be its own surprise.
func appendPragmas(dsn string) (string, error) {
	parsed, err := url.Parse(dsn)
	if err != nil {
		// url.Parse quotes the whole input back in its error, and a DSN can
		// carry credentials or a filesystem path, so the cause is not wrapped.
		return "", fmt.Errorf("the database URL is not parseable")
	}

	query := parsed.Query()
	existing := strings.Join(query["_pragma"], " ")

	for _, pragma := range sqlitePragmas {
		name := pragma[:strings.Index(pragma, "(")]
		if strings.Contains(existing, name+"(") {
			continue
		}
		query.Add("_pragma", pragma)
	}

	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}
