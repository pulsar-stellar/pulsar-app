package store

import (
	"strings"
	"testing"
)

// buildEventsQuery is the pure SQL builder behind Query. These tests are in the
// store package itself so they can reach it, and they exist because the live
// suite runs on SQLite only: the Postgres topic_contains fragment is proven by
// construction here rather than by a round trip. See ADR-041.

func TestBuildEventsQueryTopicContainsPerDialect(t *testing.T) {
	t.Parallel()

	const term = "transfer"
	q := EventQuery{ContractID: "CDNWTVUDKCCGW7GOC6SBLUFXXUCD2YDHWRDUSXZ6CYBQKQWLCUYYWI5L", TopicContains: term}

	cases := []struct {
		name string
		// dialect selects the branch under test.
		dialect Dialect
		// present are fragments the built SQL must contain; the search term is
		// bound as $2, since contract_id is $1 and the term is appended next.
		present []string
		// absent is the other dialect's element function, to prove the branch is
		// exclusive rather than emitting both.
		absent string
	}{
		{
			name:    "postgres uses a guarded jsonb_array_elements scan with strpos",
			dialect: DialectPostgres,
			present: []string{
				"jsonb_typeof(topics_json) = 'array'",
				"jsonb_array_elements(topics_json)",
				"strpos(t->>'value', $2)",
			},
			absent: "json_each",
		},
		{
			name:    "sqlite uses a guarded json_each scan with instr",
			dialect: DialectSQLite,
			present: []string{
				"json_type(topics_json) = 'array'",
				"json_each(topics_json)",
				"instr(json_extract(json_each.value, '$.value'), $2)",
			},
			absent: "jsonb_array_elements",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			sql, args, err := buildEventsQuery(q, c.dialect, 50)
			if err != nil {
				t.Fatalf("buildEventsQuery: %v", err)
			}
			for _, fragment := range c.present {
				if !strings.Contains(sql, fragment) {
					t.Errorf("SQL does not contain %q:\n%s", fragment, sql)
				}
			}
			if strings.Contains(sql, c.absent) {
				t.Errorf("SQL leaked the other dialect's %q fragment:\n%s", c.absent, sql)
			}
			if len(args) < 2 || args[1] != term {
				t.Errorf("args[1] = %v, want the bound term %q; args=%v", argAt(args, 1), term, args)
			}
			// The term is bound, never interpolated: it must not appear in the SQL.
			if strings.Contains(sql, term) {
				t.Errorf("search term was interpolated into the SQL rather than bound:\n%s", sql)
			}
		})
	}
}

func TestBuildEventsQueryOrderAndKeyset(t *testing.T) {
	t.Parallel()

	base := EventQuery{ContractID: "C", Cursor: "42"}

	asc := base
	asc.Order = "asc"
	sqlAsc, _, err := buildEventsQuery(asc, DialectSQLite, 10)
	if err != nil {
		t.Fatalf("asc: %v", err)
	}
	if !strings.Contains(sqlAsc, "ORDER BY ledger, event_index, id LIMIT") {
		t.Errorf("ascending order clause missing:\n%s", sqlAsc)
	}
	if !strings.Contains(sqlAsc, "id > $") {
		t.Errorf("ascending keyset should resume with id > cursor:\n%s", sqlAsc)
	}

	desc := base
	desc.Order = "desc"
	sqlDesc, _, err := buildEventsQuery(desc, DialectSQLite, 10)
	if err != nil {
		t.Fatalf("desc: %v", err)
	}
	if !strings.Contains(sqlDesc, "ORDER BY ledger DESC, event_index DESC, id DESC LIMIT") {
		t.Errorf("descending order clause missing:\n%s", sqlDesc)
	}
	if !strings.Contains(sqlDesc, "id < $") {
		t.Errorf("descending keyset should resume with id < cursor:\n%s", sqlDesc)
	}

	// The empty order is the ascending default, not an error.
	if _, _, err := buildEventsQuery(EventQuery{ContractID: "C"}, DialectSQLite, 10); err != nil {
		t.Errorf("the empty order should default to ascending, got %v", err)
	}
	if _, _, err := buildEventsQuery(EventQuery{ContractID: "C", Order: "sideways"}, DialectSQLite, 10); err == nil {
		t.Error("buildEventsQuery accepted an invalid order")
	}
	if _, _, err := buildEventsQuery(EventQuery{ContractID: "C", TopicContains: "x"}, Dialect(99), 10); err == nil {
		t.Error("buildEventsQuery accepted an unknown dialect")
	}
}

func argAt(args []any, i int) any {
	if i < len(args) {
		return args[i]
	}
	return nil
}
