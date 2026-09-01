// Package migrations holds the indexer's schema migrations and embeds them
// into the binary, so a deployed build carries the schema changes it needs and
// a release cannot arrive without them.
//
// The two engines have separate directories. SQLite accepts Postgres's
// BIGSERIAL as a generic column type and then writes NULL into every id, so a
// single shared file would apply cleanly on both and silently corrupt one of
// them. See ADR-029.
package migrations

import (
	"embed"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strconv"
	"strings"
)

// The all: prefix embeds a directory whose only contents are placeholders,
// which is what lets this package compile before the first migration is
// written. Filtering to .sql happens in parse, so nothing else that lands in
// these directories can become a migration by accident.
//
//go:embed all:sqlite all:postgres
var files embed.FS

// Directories are the per-engine subdirectories, matching Driver.MigrationsDir.
const (
	DirSQLite   = "sqlite"
	DirPostgres = "postgres"
)

const (
	upSuffix   = ".up.sql"
	downSuffix = ".down.sql"
)

// Migration is one numbered schema change and its reversal.
type Migration struct {
	Version int
	Name    string
	Up      string
	Down    string
}

// Filename renders the migration's up-file name, for error messages.
func (m Migration) Filename() string {
	return fmt.Sprintf("%04d_%s%s", m.Version, m.Name, upSuffix)
}

// For returns the migrations for one engine, ordered by version.
func For(dir string) ([]Migration, error) {
	switch dir {
	case DirSQLite, DirPostgres:
	default:
		return nil, fmt.Errorf("migrations: %q is not a migration directory; expected %s or %s",
			dir, DirSQLite, DirPostgres)
	}
	return parse(files, dir)
}

// parse reads and validates one directory of migration files.
//
// Every rule here fails closed. A file that does not parse, an up without a
// down, or a duplicate version is an error rather than something to skip,
// because skipping it means the schema silently differs from what the
// repository says it is.
func parse(fsys fs.FS, dir string) ([]Migration, error) {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return nil, fmt.Errorf("migrations: reading %s: %w", dir, err)
	}

	ups := map[int]*Migration{}
	downs := map[int]string{}
	names := map[int]string{}

	for _, entry := range entries {
		name := entry.Name()

		if entry.IsDir() {
			return nil, fmt.Errorf("migrations: %s contains a subdirectory %q; migrations are flat", dir, name)
		}
		if !strings.HasSuffix(name, ".sql") {
			// Placeholders and editor droppings are not migrations.
			continue
		}

		version, label, isUp, err := parseName(name)
		if err != nil {
			return nil, fmt.Errorf("migrations: %s/%s: %w", dir, name, err)
		}

		body, err := fs.ReadFile(fsys, path.Join(dir, name))
		if err != nil {
			return nil, fmt.Errorf("migrations: reading %s/%s: %w", dir, name, err)
		}
		if strings.TrimSpace(string(body)) == "" {
			return nil, fmt.Errorf("migrations: %s/%s is empty", dir, name)
		}

		if existing, ok := names[version]; ok && existing != label {
			return nil, fmt.Errorf("migrations: version %d appears as both %q and %q in %s",
				version, existing, label, dir)
		}
		names[version] = label

		// A duplicate up or down file cannot occur: two entries with the same
		// version and the same label would be the same filename, and two with
		// the same version and different labels are rejected just above.
		if isUp {
			ups[version] = &Migration{Version: version, Name: label, Up: string(body)}
			continue
		}
		downs[version] = string(body)
	}

	out := make([]Migration, 0, len(ups))
	for version, m := range ups {
		down, ok := downs[version]
		if !ok {
			return nil, fmt.Errorf("migrations: %s/%s has no matching %s file", dir, m.Filename(), downSuffix)
		}
		m.Down = down
		out = append(out, *m)
	}

	for version := range downs {
		if _, ok := ups[version]; !ok {
			return nil, fmt.Errorf("migrations: %s has a down file for version %d with no up file", dir, version)
		}
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Version < out[j].Version })

	if err := requireContiguous(out, dir); err != nil {
		return nil, err
	}
	return out, nil
}

// requireContiguous rejects a gap in the version sequence. A gap means a
// migration was deleted rather than reversed, and the two engines would then
// disagree about what version the schema is at.
func requireContiguous(ms []Migration, dir string) error {
	for i, m := range ms {
		if want := i + 1; m.Version != want {
			return fmt.Errorf("migrations: %s jumps to version %d where %d was expected; versions start at 1 and have no gaps",
				dir, m.Version, want)
		}
	}
	return nil
}

// parseName splits NNNN_label.up.sql or NNNN_label.down.sql.
func parseName(name string) (version int, label string, isUp bool, err error) {
	var stem string
	switch {
	case strings.HasSuffix(name, upSuffix):
		stem, isUp = strings.TrimSuffix(name, upSuffix), true
	case strings.HasSuffix(name, downSuffix):
		stem, isUp = strings.TrimSuffix(name, downSuffix), false
	default:
		return 0, "", false, fmt.Errorf("name must end in %s or %s", upSuffix, downSuffix)
	}

	prefix, label, found := strings.Cut(stem, "_")
	if !found || label == "" {
		return 0, "", false, fmt.Errorf("name must be NNNN_label%s", upSuffix)
	}
	if len(prefix) != 4 {
		return 0, "", false, fmt.Errorf("version prefix %q must be exactly four digits", prefix)
	}
	version, convErr := strconv.Atoi(prefix)
	if convErr != nil {
		return 0, "", false, fmt.Errorf("version prefix %q is not a number", prefix)
	}
	if version < 1 {
		return 0, "", false, fmt.Errorf("version %d must be greater than zero", version)
	}
	return version, label, isUp, nil
}
