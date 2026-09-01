package migrations

import (
	"strings"
	"testing"
	"testing/fstest"
)

func mapFS(dir string, files map[string]string) fstest.MapFS {
	fsys := fstest.MapFS{}
	for name, body := range files {
		fsys[dir+"/"+name] = &fstest.MapFile{Data: []byte(body)}
	}
	return fsys
}

func TestParseReadsAndOrdersMigrations(t *testing.T) {
	t.Parallel()

	fsys := mapFS("sqlite", map[string]string{
		"0002_events_index.up.sql":   "CREATE INDEX b ON t (x);",
		"0002_events_index.down.sql": "DROP INDEX b;",
		"0001_init.up.sql":           "CREATE TABLE t (x INTEGER);",
		"0001_init.down.sql":         "DROP TABLE t;",
	})

	got, err := parse(fsys, "sqlite")
	if err != nil {
		t.Fatalf("parse: unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("parse returned %d migrations, want 2", len(got))
	}

	if got[0].Version != 1 || got[0].Name != "init" {
		t.Errorf("first = %d/%q, want 1/init", got[0].Version, got[0].Name)
	}
	if got[1].Version != 2 || got[1].Name != "events_index" {
		t.Errorf("second = %d/%q, want 2/events_index", got[1].Version, got[1].Name)
	}
	if !strings.Contains(got[0].Up, "CREATE TABLE") || !strings.Contains(got[0].Down, "DROP TABLE") {
		t.Errorf("first migration bodies not loaded: %+v", got[0])
	}
}

func TestParseIgnoresNonSQLFiles(t *testing.T) {
	t.Parallel()

	fsys := mapFS("sqlite", map[string]string{
		".gitkeep":           "",
		"README.md":          "notes",
		"0001_init.up.sql":   "CREATE TABLE t (x INTEGER);",
		"0001_init.down.sql": "DROP TABLE t;",
	})

	got, err := parse(fsys, "sqlite")
	if err != nil {
		t.Fatalf("parse: unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("parse returned %d migrations, want 1", len(got))
	}
}

// An empty directory is not an error. It is the state this package ships in
// until the first migration is written, and the embed has to compile then too.
func TestParseAcceptsADirectoryWithNoMigrations(t *testing.T) {
	t.Parallel()

	got, err := parse(mapFS("sqlite", map[string]string{".gitkeep": ""}), "sqlite")
	if err != nil {
		t.Fatalf("parse: unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("parse returned %d migrations, want none", len(got))
	}
}

// Every rule fails closed. Skipping a malformed file means the schema silently
// differs from what the repository says it is.
func TestParseRejectsMalformedDirectories(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		files map[string]string
		want  string
	}{
		{
			"up with no down",
			map[string]string{"0001_init.up.sql": "CREATE TABLE t (x INTEGER);"},
			"no matching",
		},
		{
			"down with no up",
			map[string]string{"0001_init.down.sql": "DROP TABLE t;"},
			"no up file",
		},
		{
			"empty body",
			map[string]string{"0001_init.up.sql": "   \n", "0001_init.down.sql": "DROP TABLE t;"},
			"is empty",
		},
		{
			"missing version prefix",
			map[string]string{"init.up.sql": "x", "init.down.sql": "y"},
			"NNNN_label",
		},
		{
			"short version prefix",
			map[string]string{"1_init.up.sql": "x", "1_init.down.sql": "y"},
			"four digits",
		},
		{
			"non numeric version",
			map[string]string{"000a_init.up.sql": "x", "000a_init.down.sql": "y"},
			"not a number",
		},
		{
			"zero version",
			map[string]string{"0000_init.up.sql": "x", "0000_init.down.sql": "y"},
			"greater than zero",
		},
		{
			"no label",
			map[string]string{"0001_.up.sql": "x", "0001_.down.sql": "y"},
			"NNNN_label",
		},
		{
			"wrong suffix",
			map[string]string{"0001_init.sql": "x"},
			"must end in",
		},
		{
			"same version two labels",
			map[string]string{
				"0001_init.up.sql": "x", "0001_init.down.sql": "y",
				"0001_other.up.sql": "x", "0001_other.down.sql": "y",
			},
			"appears as both",
		},
		{
			"gap in versions",
			map[string]string{
				"0001_init.up.sql": "x", "0001_init.down.sql": "y",
				"0003_late.up.sql": "x", "0003_late.down.sql": "y",
			},
			"jumps to version",
		},
		{
			"does not start at one",
			map[string]string{"0002_init.up.sql": "x", "0002_init.down.sql": "y"},
			"jumps to version",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			_, err := parse(mapFS("sqlite", c.files), "sqlite")
			if err == nil {
				t.Fatalf("parse(%v) succeeded, want an error", c.files)
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("error %q does not contain %q", err.Error(), c.want)
			}
		})
	}
}

func TestParseRejectsASubdirectory(t *testing.T) {
	t.Parallel()

	fsys := fstest.MapFS{
		"sqlite/nested/0001_init.up.sql": &fstest.MapFile{Data: []byte("x")},
	}

	_, err := parse(fsys, "sqlite")
	if err == nil {
		t.Fatal("parse succeeded on a directory containing a subdirectory")
	}
	if !strings.Contains(err.Error(), "subdirectory") {
		t.Errorf("error %q does not mention the subdirectory", err.Error())
	}
}

func TestParseReportsAnUnreadableDirectory(t *testing.T) {
	t.Parallel()

	_, err := parse(fstest.MapFS{}, "nonexistent")
	if err == nil {
		t.Fatal("parse succeeded on a directory that does not exist")
	}
	if !strings.Contains(err.Error(), "nonexistent") {
		t.Errorf("error %q does not name the directory", err.Error())
	}
}

// The embedded tree is what actually ships, so it is exercised rather than
// only the parser. Today both directories are empty and that must still be a
// clean read, not a compile failure or a panic.
func TestForReadsBothEmbeddedDirectories(t *testing.T) {
	t.Parallel()

	for _, dir := range []string{DirSQLite, DirPostgres} {
		t.Run(dir, func(t *testing.T) {
			t.Parallel()

			got, err := For(dir)
			if err != nil {
				t.Fatalf("For(%q): unexpected error: %v", dir, err)
			}
			for i, m := range got {
				if m.Version != i+1 {
					t.Errorf("migration %d has version %d", i, m.Version)
				}
				if strings.TrimSpace(m.Up) == "" || strings.TrimSpace(m.Down) == "" {
					t.Errorf("%s has an empty body", m.Filename())
				}
			}
		})
	}
}

// ADR-029's split only holds if both engines carry the same set of versions.
// A migration added to one directory and forgotten in the other would leave
// the two schemas at different versions with nothing reporting it.
func TestBothEnginesCarryTheSameVersions(t *testing.T) {
	t.Parallel()

	sqlite, err := For(DirSQLite)
	if err != nil {
		t.Fatalf("For(sqlite): %v", err)
	}
	postgres, err := For(DirPostgres)
	if err != nil {
		t.Fatalf("For(postgres): %v", err)
	}

	if len(sqlite) != len(postgres) {
		t.Fatalf("sqlite has %d migrations and postgres has %d", len(sqlite), len(postgres))
	}
	for i := range sqlite {
		if sqlite[i].Version != postgres[i].Version || sqlite[i].Name != postgres[i].Name {
			t.Errorf("migration %d differs: sqlite %s, postgres %s",
				i, sqlite[i].Filename(), postgres[i].Filename())
		}
	}
}

func TestForRejectsUnknownDirectories(t *testing.T) {
	t.Parallel()

	for _, dir := range []string{"", "mysql", "sqlite/", "../sqlite", "SQLITE"} {
		if _, err := For(dir); err == nil {
			t.Errorf("For(%q) succeeded, want an error", dir)
		}
	}
}

func TestMigrationFilename(t *testing.T) {
	t.Parallel()

	m := Migration{Version: 7, Name: "add_thing"}
	if got, want := m.Filename(), "0007_add_thing.up.sql"; got != want {
		t.Errorf("Filename() = %q, want %q", got, want)
	}
}
