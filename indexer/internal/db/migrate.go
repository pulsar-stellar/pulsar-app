package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/pulsar-stellar/pulsar-app/indexer/migrations"
)

// schemaMigrationsDDL records which migrations have been applied.
//
// It carries no DEFAULT, so the statement is valid on both engines unchanged
// and does not need a per-driver pair of its own. The timestamp is written from
// Go, which also makes it deterministic in tests.
const schemaMigrationsDDL = `CREATE TABLE IF NOT EXISTS schema_migrations (
    version    INTEGER NOT NULL PRIMARY KEY,
    name       TEXT NOT NULL,
    applied_at TEXT NOT NULL
)`

// ErrDirtySchema reports a schema_migrations table with a gap in it, which
// means a migration was recorded without its predecessor.
var ErrDirtySchema = errors.New("schema_migrations has a gap")

// Up applies every migration not yet recorded, in order, and returns the
// versions it applied.
//
// Each migration runs in its own transaction together with the row recording
// it, so a failure leaves the database at the previous version with nothing
// half-applied and nothing falsely recorded. Both engines support
// transactional DDL, which is what makes this hold rather than being a hope.
func Up(ctx context.Context, handle *sql.DB, d Driver) ([]int, error) {
	available, err := migrations.For(d.MigrationsDir)
	if err != nil {
		return nil, err
	}
	if err := ensureSchemaMigrations(ctx, handle); err != nil {
		return nil, err
	}
	applied, err := appliedVersions(ctx, handle)
	if err != nil {
		return nil, err
	}

	var ran []int
	for _, m := range available {
		if applied[m.Version] {
			continue
		}
		if err := applyOne(ctx, handle, m); err != nil {
			return ran, err
		}
		ran = append(ran, m.Version)
	}
	return ran, nil
}

// Down reverses applied migrations, newest first, until the schema is at
// target. A target of zero reverses everything.
func Down(ctx context.Context, handle *sql.DB, d Driver, target int) ([]int, error) {
	if target < 0 {
		return nil, fmt.Errorf("migrate: target version %d is negative", target)
	}

	available, err := migrations.For(d.MigrationsDir)
	if err != nil {
		return nil, err
	}
	if err := ensureSchemaMigrations(ctx, handle); err != nil {
		return nil, err
	}
	applied, err := appliedVersions(ctx, handle)
	if err != nil {
		return nil, err
	}

	var reverted []int
	for i := len(available) - 1; i >= 0; i-- {
		m := available[i]
		if m.Version <= target || !applied[m.Version] {
			continue
		}
		if err := revertOne(ctx, handle, m); err != nil {
			return reverted, err
		}
		reverted = append(reverted, m.Version)
	}
	return reverted, nil
}

// Version reports the highest applied migration, or zero for an empty schema.
// It returns ErrDirtySchema if the recorded versions have a gap, because a gap
// means the schema is not any version at all and guessing which one would be
// worse than refusing.
func Version(ctx context.Context, handle *sql.DB) (int, error) {
	if err := ensureSchemaMigrations(ctx, handle); err != nil {
		return 0, err
	}
	applied, err := appliedVersions(ctx, handle)
	if err != nil {
		return 0, err
	}
	if len(applied) == 0 {
		return 0, nil
	}

	highest := 0
	for version := range applied {
		if version > highest {
			highest = version
		}
	}
	for version := 1; version <= highest; version++ {
		if !applied[version] {
			return 0, fmt.Errorf("migrate: %w: version %d is recorded but %d is not",
				ErrDirtySchema, highest, version)
		}
	}
	return highest, nil
}

func applyOne(ctx context.Context, handle *sql.DB, m migrations.Migration) error {
	return inTransaction(ctx, handle, m, m.Up, func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx,
			"INSERT INTO schema_migrations (version, name, applied_at) VALUES ($1, $2, $3)",
			m.Version, m.Name, time.Now().UTC().Format(time.RFC3339Nano))
		return err
	})
}

func revertOne(ctx context.Context, handle *sql.DB, m migrations.Migration) error {
	return inTransaction(ctx, handle, m, m.Down, func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "DELETE FROM schema_migrations WHERE version = $1", m.Version)
		return err
	})
}

// inTransaction runs one migration's statements and its bookkeeping in a single
// transaction. Anything that fails rolls the whole thing back, so the recorded
// version and the actual schema cannot disagree.
func inTransaction(ctx context.Context, handle *sql.DB, m migrations.Migration, body string,
	record func(context.Context, *sql.Tx) error) error {

	statements, err := SplitStatements(body)
	if err != nil {
		return fmt.Errorf("migrate: %s: %w", m.Filename(), err)
	}
	if len(statements) == 0 {
		return fmt.Errorf("migrate: %s contains no statements", m.Filename())
	}

	tx, err := handle.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("migrate: %s: beginning transaction: %w", m.Filename(), err)
	}
	defer func() { _ = tx.Rollback() }()

	for i, statement := range statements {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("migrate: %s: statement %d of %d failed, rolled back: %w",
				m.Filename(), i+1, len(statements), err)
		}
	}
	if err := record(ctx, tx); err != nil {
		return fmt.Errorf("migrate: %s: recording the migration failed, rolled back: %w", m.Filename(), err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("migrate: %s: commit failed: %w", m.Filename(), err)
	}
	return nil
}

func ensureSchemaMigrations(ctx context.Context, handle *sql.DB) error {
	if _, err := handle.ExecContext(ctx, schemaMigrationsDDL); err != nil {
		return fmt.Errorf("migrate: creating schema_migrations: %w", err)
	}
	return nil
}

func appliedVersions(ctx context.Context, handle *sql.DB) (map[int]bool, error) {
	rows, err := handle.QueryContext(ctx, "SELECT version FROM schema_migrations")
	if err != nil {
		return nil, fmt.Errorf("migrate: reading schema_migrations: %w", err)
	}
	defer func() { _ = rows.Close() }()

	applied := map[int]bool{}
	for rows.Next() {
		var version int
		if err := rows.Scan(&version); err != nil {
			return nil, fmt.Errorf("migrate: reading schema_migrations: %w", err)
		}
		applied[version] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("migrate: reading schema_migrations: %w", err)
	}
	return applied, nil
}

// SplitStatements breaks a migration file into individual statements.
//
// Statements are executed one at a time rather than handed to the driver as a
// single string, because pgx's extended protocol rejects multiple commands in
// one Exec while SQLite accepts them. Splitting here means the runner does not
// depend on a driver behaviour that differs between the two engines.
//
// The splitter tracks single-quoted literals so a semicolon inside one does not
// end a statement, and drops line comments. It does not attempt to understand
// dollar-quoted bodies, so it rejects them rather than splitting them wrongly.
func SplitStatements(sql string) ([]string, error) {
	if strings.Contains(sql, "$$") {
		return nil, errors.New("dollar-quoted blocks are not supported; the splitter would break them")
	}

	var statements []string
	var current strings.Builder
	inString := false

	for _, line := range strings.Split(sql, "\n") {
		if !inString {
			if trimmed := strings.TrimSpace(line); strings.HasPrefix(trimmed, "--") {
				continue
			}
		}

		for i := 0; i < len(line); i++ {
			c := line[i]

			if c == '\'' {
				// '' inside a literal is an escaped quote, not a terminator.
				if inString && i+1 < len(line) && line[i+1] == '\'' {
					current.WriteString("''")
					i++
					continue
				}
				inString = !inString
				current.WriteByte(c)
				continue
			}

			if c == ';' && !inString {
				if s := strings.TrimSpace(current.String()); s != "" {
					statements = append(statements, s)
				}
				current.Reset()
				continue
			}

			current.WriteByte(c)
		}
		current.WriteByte('\n')
	}

	if inString {
		return nil, errors.New("unterminated string literal")
	}
	if s := strings.TrimSpace(current.String()); s != "" {
		statements = append(statements, s)
	}
	return statements, nil
}
