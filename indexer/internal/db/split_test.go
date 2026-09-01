package db

import (
	"strings"
	"testing"

	"github.com/pulsar-stellar/pulsar-app/indexer/migrations"
)

// Statements are split rather than handed to the driver whole because pgx's
// extended protocol rejects multiple commands in one Exec while SQLite accepts
// them. The splitter is therefore load-bearing for Postgres correctness and is
// tested on its own, since the Postgres path cannot be exercised here.
func TestSplitStatements(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		sql  string
		want []string
	}{
		{
			"two statements",
			"CREATE TABLE a (x INTEGER);\nCREATE TABLE b (y INTEGER);",
			[]string{"CREATE TABLE a (x INTEGER)", "CREATE TABLE b (y INTEGER)"},
		},
		{
			"trailing statement without a semicolon",
			"CREATE TABLE a (x INTEGER)",
			[]string{"CREATE TABLE a (x INTEGER)"},
		},
		{
			"comments are dropped",
			"-- a note\nCREATE TABLE a (x INTEGER);\n-- another\n",
			[]string{"CREATE TABLE a (x INTEGER)"},
		},
		{
			"blank statements are dropped",
			";;\nCREATE TABLE a (x INTEGER);;\n",
			[]string{"CREATE TABLE a (x INTEGER)"},
		},
		{
			"semicolon inside a string literal does not split",
			"INSERT INTO t (s) VALUES ('a;b');",
			[]string{"INSERT INTO t (s) VALUES ('a;b')"},
		},
		{
			"escaped quote inside a literal",
			"INSERT INTO t (s) VALUES ('it''s;fine');\nSELECT 1;",
			[]string{"INSERT INTO t (s) VALUES ('it''s;fine')", "SELECT 1"},
		},
		{
			"double dash inside a literal is not a comment",
			"INSERT INTO t (s) VALUES ('-- not a comment');",
			[]string{"INSERT INTO t (s) VALUES ('-- not a comment')"},
		},
		{
			"empty input",
			"",
			nil,
		},
		{
			"comments only",
			"-- just a note\n\n",
			nil,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			got, err := SplitStatements(c.sql)
			if err != nil {
				t.Fatalf("SplitStatements: unexpected error: %v", err)
			}
			if len(got) != len(c.want) {
				t.Fatalf("got %d statements %q, want %d %q", len(got), got, len(c.want), c.want)
			}
			for i := range got {
				if normalize(got[i]) != normalize(c.want[i]) {
					t.Errorf("statement %d = %q, want %q", i, got[i], c.want[i])
				}
			}
		})
	}
}

func TestSplitStatementsRejectsWhatItCannotParse(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		sql  string
		want string
	}{
		{"unterminated literal", "INSERT INTO t (s) VALUES ('oops;", "unterminated"},
		{"dollar quoted block", "CREATE FUNCTION f() AS $$ BEGIN; END; $$;", "dollar-quoted"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			_, err := SplitStatements(c.sql)
			if err == nil {
				t.Fatal("expected an error")
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("error %q does not mention %q", err.Error(), c.want)
			}
		})
	}
}

// The real migration files are the input this has to get right.
func TestSplitStatementsOnTheShippedMigrations(t *testing.T) {
	t.Parallel()

	cases := []struct {
		dir  string
		want map[string]int
	}{
		{"sqlite", map[string]int{"0001_init.up.sql": 2, "0002_events_index.up.sql": 4}},
		{"postgres", map[string]int{"0001_init.up.sql": 2, "0002_events_index.up.sql": 4}},
	}

	for _, c := range cases {
		t.Run(c.dir, func(t *testing.T) {
			t.Parallel()

			ms, err := loadForTest(t, c.dir)
			if err != nil {
				t.Fatalf("loading %s: %v", c.dir, err)
			}
			for _, m := range ms {
				up, err := SplitStatements(m.Up)
				if err != nil {
					t.Fatalf("%s: %v", m.Filename(), err)
				}
				if want, ok := c.want[m.Filename()]; ok && len(up) != want {
					t.Errorf("%s split into %d statements, want %d: %q", m.Filename(), len(up), want, up)
				}
				down, err := SplitStatements(m.Down)
				if err != nil {
					t.Fatalf("%s down: %v", m.Filename(), err)
				}
				if len(down) == 0 {
					t.Errorf("%s down split into no statements", m.Filename())
				}
			}
		})
	}
}

func normalize(s string) string { return strings.Join(strings.Fields(s), " ") }

func loadForTest(t *testing.T, dir string) ([]migrations.Migration, error) {
	t.Helper()
	return migrations.For(dir)
}
