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
}

// NewEvents builds a store over a database handle or a transaction.
func NewEvents(q Querier) *Events { return &Events{q: q} }

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
// every event on a contract, newest last.
type EventQuery struct {
	ContractID string
	Name       string
	FromLedger int64
	ToLedger   int64
	Limit      int
	Cursor     string
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

// Query returns one page of a contract's events, oldest first, with the cursor
// for the next page.
//
// Ordering is by (ledger, event_index), which is the global emission order per
// ADR-022, with the id as a final tiebreak so the order is total and paging
// cannot skip or repeat a row.
func (e *Events) Query(ctx context.Context, q EventQuery) (EventsPage, error) {
	if q.ContractID == "" {
		return EventsPage{}, errors.New("store: a contract id is required")
	}

	limit := q.Limit
	switch {
	case limit == 0:
		limit = defaultPageSize
	case limit < 0:
		return EventsPage{}, fmt.Errorf("store: limit %d is negative", limit)
	case limit > maxPageSize:
		return EventsPage{}, fmt.Errorf("store: limit %d exceeds the %d ceiling", limit, maxPageSize)
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
	if q.Cursor != "" {
		after, err := parseCursor(q.Cursor)
		if err != nil {
			return EventsPage{}, err
		}
		add("id > $%d", after)
	}

	// One row beyond the page tells us whether another page exists, without a
	// second count query that could disagree with this one under concurrent
	// writes.
	args = append(args, limit+1)

	query := `SELECT ` + eventColumns + ` FROM events WHERE ` +
		strings.Join(conditions, " AND ") +
		` ORDER BY ledger, event_index, id LIMIT $` + strconv.Itoa(len(args))

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
