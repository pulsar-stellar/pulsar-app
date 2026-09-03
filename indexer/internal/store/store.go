// Package store reads and writes the indexer's tables.
//
// One file per table, with the places the two engines differ handled inside
// rather than by a second implementation. Those places are few and specific:
// timestamp scanning, per ADR-032, and the raw_topics column's TEXT[] against
// TEXT split, per ADR-029.
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// ErrNotFound reports a row that is not there. Callers map it to a 404 with a
// not_found envelope, per ADR-019.
var ErrNotFound = errors.New("not found")

// Querier is the subset of *sql.DB and *sql.Tx the stores use, so every method
// works the same inside a transaction as outside one.
type Querier interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// sqliteTimeLayout is what SQLite's CURRENT_TIMESTAMP produces: no T and no
// offset, so it parses as UTC rather than as a local time.
const sqliteTimeLayout = "2006-01-02 15:04:05"

// scanTime normalises the three shapes a timestamp column can arrive in.
//
// Postgres returns a time.Time. SQLite returns a string, either its own
// CURRENT_TIMESTAMP format for a column filled by the default, or RFC3339 for
// one this package wrote. Anything else is an error rather than a guess. See
// ADR-032.
func scanTime(src any) (time.Time, error) {
	switch v := src.(type) {
	case time.Time:
		return v.UTC(), nil
	case string:
		return parseTimeString(v)
	case []byte:
		return parseTimeString(string(v))
	case nil:
		return time.Time{}, errors.New("store: timestamp is null, but every timestamp column is NOT NULL")
	default:
		return time.Time{}, fmt.Errorf("store: timestamp arrived as %T, which is not a time or a string", src)
	}
}

func parseTimeString(raw string) (time.Time, error) {
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, sqliteTimeLayout} {
		if parsed, err := time.Parse(layout, raw); err == nil {
			return parsed.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("store: timestamp %q matches none of RFC3339 or %q", raw, sqliteTimeLayout)
}

// formatTime renders a timestamp for binding.
//
// Never pass a time.Time to Exec in this package. SQLite accepts it without
// complaint and stores Go's String() output, which is neither RFC3339 nor
// anything a reader expects. See ADR-032.
func formatTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}
