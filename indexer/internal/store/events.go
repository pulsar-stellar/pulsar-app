package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/pulsar-stellar/pulsar-app/indexer/internal/models"
)

// maxPageSize bounds a single page of events. It is the same ceiling Soroban
// RPC enforces on getEvents, per ADR-028, so no caller can ask this API for a
// page the upstream could not have produced.
const maxPageSize = 10000

// defaultPageSize is what a query with no limit returns.
const defaultPageSize = 100

// Events reads and writes the events table.
type Events struct {
	q Querier
	// dialect selects the SQL for the one query that cannot be written once for
	// both engines, topic_contains. Insert and Get are portable and ignore it.
	dialect Dialect
}

// NewEvents builds a store over a database handle or a transaction. The dialect
// is required rather than optional so every Events unconditionally knows its
// engine, and Query never has to branch on a maybe-unset value. See ADR-041.
func NewEvents(q Querier, dialect Dialect) *Events { return &Events{q: q, dialect: dialect} }

const eventColumns = `id, contract_id, ledger, tx_hash, event_index, name,
	topics_json, data_json, raw_topics, raw_data, emitted_at, in_successful_contract_call`

// EventsPage is one page of results and the cursor for the next one.
//
// NextCursor is nil when the query is exhausted, per ADR-021. It is a pointer
// rather than a string so exhaustion is structural: a caller pages until it is
// nil, and there is no empty-string sentinel that could be mistaken for a
// usable cursor or passed back by accident.
type EventsPage struct {
	Events     []*models.Event
	NextCursor *string
}

// EventQuery filters a page of events. A zero value asks for the first page of
// every event on a contract in ascending emission order.
type EventQuery struct {
	ContractID string
	Name       string
	FromLedger int64
	ToLedger   int64
	Limit      int
	Cursor     string

	// Order is "asc", "desc", or "" for the ascending default. The store
	// defaults to ascending to preserve its existing callers; the HTTP layer
	// defaults to descending to match the SDK. See ADR-041.
	Order string

	// TopicContains, when set, keeps only events with a decoded topic whose
	// value contains it as a literal, case-sensitive substring. See ADR-041.
	TopicContains string
}

// Insert writes a batch of events in one statement.
//
// Batching matters because a poll returns a page of events at a time, and one
// round trip per event turns a 100-event ledger into 100 of them. A single
// event is a batch of one and costs a slice allocation.
//
// Events already stored are left alone rather than failing the batch. A poll
// that overlaps the previous one, which is the normal case at a ledger
// boundary, would otherwise abort on its first repeated (ledger, event_index).
func (e *Events) Insert(ctx context.Context, events []*models.Event) (int, error) {
	if len(events) == 0 {
		return 0, nil
	}
	if len(events) > maxPageSize {
		return 0, fmt.Errorf("store: %d events exceeds the %d batch ceiling", len(events), maxPageSize)
	}

	const columnsPerRow = 11
	placeholders := make([]string, 0, len(events))
	args := make([]any, 0, len(events)*columnsPerRow)

	for i, event := range events {
		if event == nil {
			return 0, fmt.Errorf("store: event %d in the batch is nil", i)
		}
		if event.ContractID == "" {
			return 0, fmt.Errorf("store: event %d has no contract id", i)
		}

		rawTopics, err := encodeRawTopics(event.RawTopics)
		if err != nil {
			return 0, fmt.Errorf("store: event %d: %w", i, err)
		}

		base := i * columnsPerRow
		slots := make([]string, columnsPerRow)
		for j := range slots {
			slots[j] = "$" + strconv.Itoa(base+j+1)
		}
		placeholders = append(placeholders, "("+strings.Join(slots, ", ")+")")

		args = append(args,
			event.ContractID,
			event.Ledger,
			event.TxHash,
			event.EventIndex,
			event.Name,
			jsonOrNull(event.TopicsJSON),
			jsonOrNull(event.DataJSON),
			rawTopics,
			event.RawData,
			// ADR-032: never bind a time.Time. SQLite stores Go's String()
			// output for one, with no error at all.
			formatTime(event.EmittedAt),
			event.InSuccessfulContractCall,
		)
	}

	query := `INSERT INTO events
		(contract_id, ledger, tx_hash, event_index, name, topics_json, data_json,
		 raw_topics, raw_data, emitted_at, in_successful_contract_call)
		VALUES ` + strings.Join(placeholders, ", ") +
		` ON CONFLICT (ledger, event_index) DO NOTHING`

	result, err := e.q.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, fmt.Errorf("store: inserting %d events: %w", len(events), err)
	}

	inserted, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("store: inserting %d events: %w", len(events), err)
	}
	return int(inserted), nil
}

// Get returns one event by its identifier, or ErrNotFound.
func (e *Events) Get(ctx context.Context, id int64) (*models.Event, error) {
	row := e.q.QueryRowContext(ctx, `SELECT `+eventColumns+` FROM events WHERE id = $1`, id)

	event, err := scanEvent(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("store: event %d: %w", id, ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("store: reading event %d: %w", id, err)
	}
	return event, nil
}

// Query returns one page of a contract's events with the cursor for the next
// page. Ordering is by emission order (ledger, event_index) per ADR-022, with
// the id as a final tiebreak, ascending by default or descending when the
// query asks. See ADR-041 for the descending keyset.
func (e *Events) Query(ctx context.Context, q EventQuery) (EventsPage, error) {
	limit, err := resolvePageSize(q.Limit)
	if err != nil {
		return EventsPage{}, err
	}

	query, args, err := buildEventsQuery(q, e.dialect, limit)
	if err != nil {
		return EventsPage{}, err
	}

	rows, err := e.q.QueryContext(ctx, query, args...)
	if err != nil {
		return EventsPage{}, fmt.Errorf("store: querying events: %w", err)
	}
	defer func() { _ = rows.Close() }()

	events := []*models.Event{}
	for rows.Next() {
		event, err := scanEvent(rows)
		if err != nil {
			return EventsPage{}, fmt.Errorf("store: querying events: %w", err)
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return EventsPage{}, fmt.Errorf("store: querying events: %w", err)
	}

	page := EventsPage{Events: events}
	if len(events) > limit {
		page.Events = events[:limit]
		cursor := formatCursor(page.Events[limit-1].ID)
		page.NextCursor = &cursor
	}
	return page, nil
}

// resolvePageSize turns a requested limit into the page size to fetch. Zero
// asks for the default; a negative or over-ceiling limit is an error. The
// ceiling is the RPC page cap per ADR-028, so no caller can ask this API for a
// page the upstream could not have produced.
func resolvePageSize(limit int) (int, error) {
	switch {
	case limit == 0:
		return defaultPageSize, nil
	case limit < 0:
		return 0, fmt.Errorf("store: limit %d is negative", limit)
	case limit > maxPageSize:
		return 0, fmt.Errorf("store: limit %d exceeds the %d ceiling", limit, maxPageSize)
	default:
		return limit, nil
	}
}

// buildEventsQuery assembles the SELECT for one page, parameterizing every
// value, and returns it with its argument list. It is pure and separate from
// Query so the per-dialect topic_contains SQL can be asserted without a live
// database of that engine, which the live suite does not have for Postgres.
//
// The limit is bound as limit+1: one row beyond the page tells Query whether
// another page exists without a second count query that could disagree under
// concurrent writes.
func buildEventsQuery(q EventQuery, dialect Dialect, limit int) (string, []any, error) {
	if q.ContractID == "" {
		return "", nil, errors.New("store: a contract id is required")
	}
	if dialect != DialectSQLite && dialect != DialectPostgres {
		return "", nil, fmt.Errorf("store: unknown dialect %s", dialect)
	}
	ascending, err := orderIsAscending(q.Order)
	if err != nil {
		return "", nil, err
	}

	conditions := []string{"contract_id = $1"}
	args := []any{q.ContractID}

	add := func(clause string, value any) {
		args = append(args, value)
		conditions = append(conditions, fmt.Sprintf(clause, len(args)))
	}

	if q.Name != "" {
		add("name = $%d", q.Name)
	}
	if q.FromLedger > 0 {
		add("ledger >= $%d", q.FromLedger)
	}
	if q.ToLedger > 0 {
		add("ledger <= $%d", q.ToLedger)
	}
	if q.TopicContains != "" {
		args = append(args, q.TopicContains)
		conditions = append(conditions, topicContainsClause(dialect, len(args)))
	}
	if q.Cursor != "" {
		after, err := parseCursor(q.Cursor)
		if err != nil {
			return "", nil, err
		}
		// The id keyset is a valid proxy for (ledger, event_index) because the
		// poller inserts in emission order, so id ascends with it. Ascending
		// resumes after the cursor, descending before it. See ADR-041.
		if ascending {
			add("id > $%d", after)
		} else {
			add("id < $%d", after)
		}
	}

	args = append(args, limit+1)

	order := "ORDER BY ledger, event_index, id"
	if !ascending {
		order = "ORDER BY ledger DESC, event_index DESC, id DESC"
	}

	query := `SELECT ` + eventColumns + ` FROM events WHERE ` +
		strings.Join(conditions, " AND ") + ` ` + order +
		` LIMIT $` + strconv.Itoa(len(args))
	return query, args, nil
}

// orderIsAscending reads the query's order. The empty string is the ascending
// default; "asc" and "desc" are explicit; anything else is an error. The match
// is case-sensitive to mirror the SDK's lowercase enum.
func orderIsAscending(order string) (bool, error) {
	switch order {
	case "", "asc":
		return true, nil
	case "desc":
		return false, nil
	default:
		return false, fmt.Errorf("store: order %q is not one of asc or desc", order)
	}
}

// topicContainsClause is the one query fragment the two engines spell
// differently. Both do a literal, case-sensitive substring test (strpos and
// instr, not LIKE, so % and _ are ordinary characters) over the value of every
// topic in the array. Each is guarded by an array-type check: a topics_json
// that is the scalar JSON null, which jsonOrNull writes for an event with no
// topics, would otherwise make Postgres' jsonb_array_elements raise rather than
// match nothing. The GIN index from migration 0002 does not serve this scan; a
// trigram index would, and is deferred. See ADR-041.
func topicContainsClause(dialect Dialect, n int) string {
	switch dialect {
	case DialectPostgres:
		return fmt.Sprintf(
			`(jsonb_typeof(topics_json) = 'array' AND EXISTS (SELECT 1 FROM jsonb_array_elements(topics_json) AS t WHERE strpos(t->>'value', $%d) > 0))`, n)
	default:
		return fmt.Sprintf(
			`(json_type(topics_json) = 'array' AND EXISTS (SELECT 1 FROM json_each(topics_json) WHERE instr(json_extract(json_each.value, '$.value'), $%d) > 0))`, n)
	}
}

// formatCursor renders an event id as a cursor. The cursor is the id, as a
// string of digits, matching how the id travels on the wire per ADR-021.
func formatCursor(id int64) string { return strconv.FormatInt(id, 10) }

// parseCursor reads a cursor back. An empty cursor never reaches here: absence
// is signalled by a nil NextCursor, and an empty string is not a valid value to
// pass back, per ADR-021.
func parseCursor(cursor string) (int64, error) {
	id, err := strconv.ParseInt(cursor, 10, 64)
	if err != nil || id < 0 {
		return 0, fmt.Errorf("store: cursor %q is not an event id", cursor)
	}
	return id, nil
}

// encodeRawTopics renders the base64 XDR topics for storage.
//
// The column is TEXT holding a JSON array on both engines. It was TEXT[] on
// Postgres until the store was written: pgx returns such a column as the
// Postgres array literal string through database/sql, not as a []string, so
// the value could be written and not read. See ADR-029.
func encodeRawTopics(topics []string) (string, error) {
	if topics == nil {
		topics = []string{}
	}
	encoded, err := json.Marshal(topics)
	if err != nil {
		return "", fmt.Errorf("encoding raw topics: %w", err)
	}
	return string(encoded), nil
}

// decodeRawTopics reads the column back.
//
// A value that is not a JSON array is an error rather than an empty slice.
// Returning empty would present an event as having emitted no topics, which is
// a claim about the ledger rather than a report of a storage problem.
func decodeRawTopics(raw string) ([]string, error) {
	var topics []string
	if err := json.Unmarshal([]byte(raw), &topics); err != nil {
		return nil, fmt.Errorf("store: raw_topics is not a JSON array: %w", err)
	}
	if topics == nil {
		topics = []string{}
	}
	return topics, nil
}

// jsonOrNull keeps an unset decoded column from being written as the literal
// bytes "null", which would read back as a JSON null rather than as absent.
func jsonOrNull(raw json.RawMessage) string {
	if len(raw) == 0 {
		return "null"
	}
	return string(raw)
}

func scanEvent(s scanner) (*models.Event, error) {
	var (
		event     models.Event
		topics    []byte
		data      []byte
		rawTopics string
		emittedAt any
	)

	if err := s.Scan(
		&event.ID, &event.ContractID, &event.Ledger, &event.TxHash, &event.EventIndex,
		&event.Name, &topics, &data, &rawTopics, &event.RawData, &emittedAt,
		&event.InSuccessfulContractCall,
	); err != nil {
		return nil, err
	}

	event.TopicsJSON = json.RawMessage(topics)
	event.DataJSON = json.RawMessage(data)

	decoded, err := decodeRawTopics(rawTopics)
	if err != nil {
		return nil, err
	}
	event.RawTopics = decoded

	parsed, err := scanTime(emittedAt)
	if err != nil {
		return nil, err
	}
	event.EmittedAt = parsed

	return &event, nil
}
