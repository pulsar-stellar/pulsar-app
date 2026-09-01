package db

import (
	"strings"
	"testing"
)

// redactDSN is only reached from an error path that is hard to provoke through
// the public surface, so it is tested directly. An untested redaction helper is
// the same defect as an assertion that observes nothing: it reports success by
// existing, and nobody finds out it was wrong until a password is in a log.
func TestRedactDSNRemovesCredentials(t *testing.T) {
	t.Parallel()

	const password = "sup3rs3cr3t"

	cases := []struct {
		name string
		dsn  string
	}{
		{"postgres url", "postgres://admin:" + password + "@db.internal:5432/pulsar?sslmode=require"},
		{"postgres url without password", "postgres://admin@db.internal:5432/pulsar"},
		{"sqlite file url", "file:/srv/data/pulsar.db?_pragma=foreign_keys(1)"},
		{"keyword form", "host=db.internal user=admin password=" + password + " dbname=pulsar"},
		{"not a url", "::nonsense::" + password},
		{"empty", ""},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			got := redactDSN(c.dsn)

			for _, secret := range []string{password, "admin"} {
				if strings.Contains(got, secret) {
					t.Errorf("redactDSN(%q) = %q, which still contains %q", c.dsn, got, secret)
				}
			}
			// The query string can carry credentials of its own and is dropped
			// wholesale rather than filtered key by key.
			if strings.Contains(got, "?") {
				t.Errorf("redactDSN(%q) = %q, which kept a query string", c.dsn, got)
			}
		})
	}
}

func TestRedactDSNKeepsEnoughToBeUseful(t *testing.T) {
	t.Parallel()

	got := redactDSN("postgres://admin:pw@db.internal:5432/pulsar?sslmode=require")
	if !strings.Contains(got, "db.internal:5432") {
		t.Errorf("redactDSN = %q, want it to keep the host so the error says which server", got)
	}
}

// Anything that does not parse as a URL is replaced outright rather than
// partially cleaned, because a partial clean on an unknown shape is a guess.
func TestRedactDSNReplacesUnparseableInput(t *testing.T) {
	t.Parallel()

	for _, dsn := range []string{"::nonsense::", "host=h password=pw", ""} {
		if got := redactDSN(dsn); got != "[redacted]" {
			t.Errorf("redactDSN(%q) = %q, want [redacted]", dsn, got)
		}
	}
}
