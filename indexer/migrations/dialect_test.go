package migrations

import (
	"fmt"
	"strings"
	"testing"
)

// allowedSubstitutions are the token pairs the two engines may differ by, in
// Postgres/SQLite order. Every entry is a divergence the engines force rather
// than a preference, and each is recorded in ADR-029 with the reason it exists.
//
// Adding to this table is a decision, not a convenience. Each new entry widens
// what the diff test will accept, so it belongs in the ADR first.
var allowedSubstitutions = []struct {
	postgres string
	sqlite   string
	why      string
}{
	{"BIGSERIAL", "INTEGER", "SQLite accepts BIGSERIAL and then writes NULL into every id"},
	{"now()", "CURRENT_TIMESTAMP", "now() is a syntax error on SQLite, so the migration cannot apply"},
	{"JSONB", "TEXT", "SQLite has no JSONB; it accepts the token and stores TEXT affinity"},
	{"TEXT[]", "TEXT", "SQLite has no array type; it accepts TEXT[] as a plain text column"},
	{"USING GIN (topics_json)", "(topics_json)",
		"SQLite has no GIN and cannot index JSON containment at all; it gets a plain index that serves ordering but not topic_contains"},
}

// The per-driver files must stay structurally identical: the same statements in
// the same order, over the same tables, with the same columns in the same order
// and the same constraints. Only the substitutions above may differ. See
// ADR-029.
//
// Comments are excluded from the comparison. The two files need dialect
// specific prose, and a comment is not schema.
func TestMigrationsDifferOnlyByAllowedSubstitutions(t *testing.T) {
	t.Parallel()

	postgres, err := For(DirPostgres)
	if err != nil {
		t.Fatalf("For(postgres): %v", err)
	}
	sqlite, err := For(DirSQLite)
	if err != nil {
		t.Fatalf("For(sqlite): %v", err)
	}
	if len(postgres) != len(sqlite) {
		t.Fatalf("postgres has %d migrations and sqlite has %d", len(postgres), len(sqlite))
	}
	if len(postgres) == 0 {
		t.Skip("no migrations to compare yet")
	}

	for i := range postgres {
		p, s := postgres[i], sqlite[i]

		t.Run(fmt.Sprintf("%04d_%s/up", p.Version, p.Name), func(t *testing.T) {
			t.Parallel()
			compareDialects(t, p.Up, s.Up)
		})
		t.Run(fmt.Sprintf("%04d_%s/down", p.Version, p.Name), func(t *testing.T) {
			t.Parallel()
			compareDialects(t, p.Down, s.Down)
		})
	}
}

// compareDialects reports every unexplained difference, naming the line, the
// two tokens, and why the pair was not accepted. A reviewer should not have to
// run a diff themselves to learn what changed.
func compareDialects(t *testing.T, postgresSQL, sqliteSQL string) {
	t.Helper()

	p := significantLines(postgresSQL)
	s := significantLines(sqliteSQL)

	if len(p) != len(s) {
		t.Fatalf("the two files have different numbers of significant lines: postgres %d, sqlite %d.\n"+
			"postgres:\n%s\nsqlite:\n%s",
			len(p), len(s), strings.Join(p, "\n"), strings.Join(s, "\n"))
	}

	for i := range p {
		if p[i] == s[i] {
			continue
		}

		// Some divergences are whole clauses rather than single tokens, so
		// substitutions are applied to the line until it either matches or
		// stops changing. A line that still differs is reported in full.
		reconciled, applied := applySubstitutions(p[i])
		if reconciled == s[i] {
			for _, why := range applied {
				t.Logf("line %d: %s", i+1, why)
			}
			continue
		}

		pWords, sWords := strings.Fields(p[i]), strings.Fields(s[i])
		if len(pWords) == len(sWords) {
			for j := range pWords {
				pTok, sTok := trimPunctuation(pWords[j]), trimPunctuation(sWords[j])
				if pTok == sTok {
					continue
				}
				t.Errorf("line %d: postgres has %q, sqlite has %q, which is not an allowed substitution.\n"+
					"  postgres: %s\n  sqlite:   %s\n"+
					"  Allowed pairs are %s. Adding one is an ADR-029 amendment, not a test edit.",
					i+1, pTok, sTok, p[i], s[i], allowedPairsList())
			}
			continue
		}

		t.Errorf("line %d: the two lines differ and no allowed substitution reconciles them.\n"+
			"  postgres: %s\n  sqlite:   %s\n"+
			"  After applying every allowed substitution the postgres line reads:\n    %s\n"+
			"  Allowed pairs are %s. Adding one is an ADR-029 amendment, not a test edit.",
			i+1, p[i], s[i], reconciled, allowedPairsList())
	}
}

// applySubstitutions rewrites a Postgres line into its SQLite form, applying
// every allowed substitution that matches and reporting which ones fired.
func applySubstitutions(line string) (string, []string) {
	var applied []string
	for _, sub := range allowedSubstitutions {
		if !strings.Contains(line, sub.postgres) {
			continue
		}
		line = strings.ReplaceAll(line, sub.postgres, sub.sqlite)
		applied = append(applied, fmt.Sprintf("%s -> %s (%s)", sub.postgres, sub.sqlite, sub.why))
	}
	return line, applied
}

// significantLines drops comments and blank lines and collapses runs of
// whitespace, so alignment inside a column list is not a difference.
func significantLines(sql string) []string {
	var out []string
	for _, raw := range strings.Split(sql, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "--") {
			continue
		}
		out = append(out, strings.Join(strings.Fields(line), " "))
	}
	return out
}

// trimPunctuation removes trailing separators so that "now()," and "now()"
// compare as the same token.
func trimPunctuation(token string) string {
	return strings.TrimRight(token, ",;")
}

func allowedPairsList() string {
	pairs := make([]string, 0, len(allowedSubstitutions))
	for _, sub := range allowedSubstitutions {
		pairs = append(pairs, fmt.Sprintf("%s/%s", sub.postgres, sub.sqlite))
	}
	return strings.Join(pairs, ", ")
}

// The comparison must actually reject things, or it passes by observing
// nothing. Each case is a realistic drift: a column added to one file only, a
// type changed on one side, a constraint dropped from one side, and a
// substitution used in the wrong direction.
func TestDialectComparisonRejectsRealDrift(t *testing.T) {
	t.Parallel()

	const base = `CREATE TABLE events (
    id          BIGSERIAL NOT NULL PRIMARY KEY,
    ledger      INTEGER NOT NULL,
    topics_json JSONB NOT NULL
);`
	const twin = `CREATE TABLE events (
    id          INTEGER NOT NULL PRIMARY KEY,
    ledger      INTEGER NOT NULL,
    topics_json TEXT NOT NULL
);`

	cases := []struct {
		name     string
		postgres string
		sqlite   string
		wantFail bool
	}{
		{"identical but for allowed substitutions", base, twin, false},
		{
			"column present in only one file",
			base,
			strings.Replace(twin, "    ledger      INTEGER NOT NULL,\n", "", 1),
			true,
		},
		{
			"unlisted type change",
			base,
			strings.Replace(twin, "ledger      INTEGER", "ledger      BIGINT", 1),
			true,
		},
		{
			"constraint dropped on one side",
			base,
			strings.Replace(twin, "ledger      INTEGER NOT NULL,", "ledger      INTEGER,", 1),
			true,
		},
		{
			"substitution used backwards",
			strings.Replace(base, "topics_json JSONB", "topics_json TEXT", 1),
			strings.Replace(twin, "topics_json TEXT", "topics_json JSONB", 1),
			true,
		},
		{
			"columns reordered",
			base,
			`CREATE TABLE events (
    ledger      INTEGER NOT NULL,
    id          INTEGER NOT NULL PRIMARY KEY,
    topics_json TEXT NOT NULL
);`,
			true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			// compareDialects may call Fatalf, which is runtime.Goexit and
			// cannot be recovered. Running it in its own goroutine keeps the
			// exit local to that goroutine; deferred calls still run, so the
			// channel closes either way.
			probe := &testing.T{}
			done := make(chan struct{})
			go func() {
				defer close(done)
				compareDialects(probe, c.postgres, c.sqlite)
			}()
			<-done

			if probe.Failed() != c.wantFail {
				t.Errorf("comparison failed = %v, want %v", probe.Failed(), c.wantFail)
			}
		})
	}
}

// A phrase substitution rewrites a whole clause, so it has more scope to mask
// drift than a single-token pair does. These cases pin what it must still
// reject on the line the GIN entry exists for.
func TestPhraseSubstitutionDoesNotMaskDrift(t *testing.T) {
	t.Parallel()

	const pg = "CREATE INDEX idx_events_topics ON events USING GIN (topics_json);"

	cases := []struct {
		name     string
		sqlite   string
		wantFail bool
	}{
		{"the substitution itself", "CREATE INDEX idx_events_topics ON events (topics_json);", false},
		{"different column", "CREATE INDEX idx_events_topics ON events (data_json);", true},
		{"different index name", "CREATE INDEX idx_events_topics_x ON events (topics_json);", true},
		{"different table", "CREATE INDEX idx_events_topics ON contracts (topics_json);", true},
		{"statement missing", "", true},

		// Identical files are not a difference, so this comparison passes and
		// should: a SQLite file carrying USING GIN is caught by actually
		// applying the migration, where SQLite rejects it as a syntax error.
		// This test checks that the two files agree; step 49 checks that what
		// they say can run.
		{"GIN left in place on sqlite, caught on apply instead", pg, false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			probe := &testing.T{}
			done := make(chan struct{})
			go func() {
				defer close(done)
				compareDialects(probe, pg, c.sqlite)
			}()
			<-done

			if probe.Failed() != c.wantFail {
				t.Errorf("comparison failed = %v, want %v", probe.Failed(), c.wantFail)
			}
		})
	}
}

// Whitespace and comments are not differences, but they must not hide one
// either.
func TestDialectComparisonIgnoresFormattingButNotContent(t *testing.T) {
	t.Parallel()

	probe := &testing.T{}
	compareDialects(probe,
		"-- postgres notes\nCREATE TABLE t (\n    id   BIGSERIAL NOT NULL\n);",
		"-- entirely different sqlite notes\n\nCREATE TABLE t (\n  id BIGSERIAL   NOT NULL\n);")
	if probe.Failed() {
		t.Error("comparison treated comments or alignment as a difference")
	}
}
