package validate

import (
	"errors"
	"regexp"
	"strconv"
)

// The event-list query parameters, validated at the HTTP boundary before a
// request reaches the store. Each validator mirrors one field of
// @pulsar-stellar/sdk's EventQuerySchema, so the indexer accepts exactly what
// the published client sends and rejects, with a catalogued 400, what it does
// not. Every value arrives as a string from the query string, and a number
// travels as its decimal digits; these functions turn that raw text into the
// primitive the handler assembles into a store.EventQuery. They never touch the
// store, so this package keeps its narrow dependency set.

// eventLimitDefault, eventLimitMin, and eventLimitMax fix the page-size bounds
// the SDK fixes: EVENT_QUERY_DEFAULT_LIMIT and EVENT_QUERY_MAX_LIMIT in
// @pulsar-stellar/sdk. The ceiling is deliberately below the store's own 10000
// page cap (ADR-028), so the HTTP surface never asks for a page larger than the
// SDK would request. See ADR-041.
const (
	eventLimitDefault = 50
	eventLimitMin     = 1
	eventLimitMax     = 500
)

// The sentinels each validator returns, one per catalogued error code the
// events handlers map them to. A caller branches on these with errors.Is; the
// handler translates each to its apierror code and the message never has to be
// matched. ErrLedgerBound and ErrLedgerRange are distinct failures, a malformed
// bound and an inverted window, that share the one VALIDATION_LEDGER_RANGE code.
var (
	ErrLimit       = errors.New("limit is not an integer between 1 and 500")
	ErrOrder       = errors.New("order is not asc or desc")
	ErrCursor      = errors.New("cursor is not an event id")
	ErrLedgerBound = errors.New("ledger bound is not a non-negative integer")
	ErrLedgerRange = errors.New("from_ledger is greater than to_ledger")
	ErrEventID     = errors.New("event id is not a string of digits")
)

// digitsPattern matches a non-empty run of decimal digits and nothing else. It
// is how a number is validated here rather than by strconv's leniency, which
// would accept a leading sign or, with surrounding trimming elsewhere,
// whitespace: the SDK serializes a number as its plain decimal digits, so a
// value carrying a sign, a decimal point, or a space did not come from the
// client and is rejected. \A and \z anchor the whole string, so a trailing
// newline cannot slip through. It matches the /^\d+$/ of the SDK's EventIdSchema.
var digitsPattern = regexp.MustCompile(`\A\d+\z`)

// EventLimit resolves the limit query parameter. An empty value is the default
// page size; a present value must be a run of digits naming an integer within
// [1, 500]. A value that is not digits, is out of range, or overflows an int is
// ErrLimit.
func EventLimit(raw string) (int, error) {
	if raw == "" {
		return eventLimitDefault, nil
	}
	if !digitsPattern.MatchString(raw) {
		return 0, ErrLimit
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		// digitsPattern already excluded a bad shape, so the only error left is
		// a value too large for an int, which is out of range by definition.
		return 0, ErrLimit
	}
	if n < eventLimitMin || n > eventLimitMax {
		return 0, ErrLimit
	}
	return n, nil
}

// EventOrder resolves the order query parameter. An empty value defaults to
// descending, matching the SDK, whose EventQuerySchema defaults order to 'desc';
// "asc" and "desc" pass through. The match is case-sensitive to mirror the SDK's
// lowercase enum, so "ASC" is ErrOrder rather than a silent accept.
func EventOrder(raw string) (string, error) {
	switch raw {
	case "":
		return "desc", nil
	case "asc", "desc":
		return raw, nil
	default:
		return "", ErrOrder
	}
}

// EventCursor validates the cursor query parameter. An empty value means no
// cursor, the first page. A present value must be a run of digits naming an
// event id that fits an int64, since a cursor is an id and the store parses it
// as one; validating it here means a malformed cursor is a 400 rather than a
// store error the handler would have to treat as a 500.
func EventCursor(raw string) (string, error) {
	if raw == "" {
		return "", nil
	}
	if !digitsPattern.MatchString(raw) {
		return "", ErrCursor
	}
	if _, err := strconv.ParseInt(raw, 10, 64); err != nil {
		// A run of digits too long for an int64 names no storable id.
		return "", ErrCursor
	}
	return raw, nil
}

// LedgerBound resolves one ledger bound, from_ledger or to_ledger. An empty
// value is unset, returned as 0, which the store reads as no bound on that side.
// A present value must be a run of digits, which is non-negative by
// construction, matching the SDK's nonnegative ledger fields; anything else, or
// a value past an int64, is ErrLedgerBound.
func LedgerBound(raw string) (int64, error) {
	if raw == "" {
		return 0, nil
	}
	if !digitsPattern.MatchString(raw) {
		return 0, ErrLedgerBound
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, ErrLedgerBound
	}
	return n, nil
}

// LedgerRange checks that a set lower bound does not exceed a set upper bound.
// A bound of 0 is unset, so a window is inverted only when both bounds are set
// and from is above to; from=5,to=0 is "from ledger 5 with no ceiling", not an
// empty window. An inverted window is ErrLedgerRange.
func LedgerRange(from, to int64) error {
	if from > 0 && to > 0 && from > to {
		return ErrLedgerRange
	}
	return nil
}

// EventID validates the id path segment of GET /events/{id}. It must be a
// non-empty run of digits, the shape of the SDK's EventIdSchema, that fits a
// BIGSERIAL; the parsed id is returned for the store lookup. A non-numeric id,
// or one past an int64, is ErrEventID: it names no storable event and so is a
// malformed request rather than a well-formed id that finds nothing.
func EventID(raw string) (int64, error) {
	if !digitsPattern.MatchString(raw) {
		return 0, ErrEventID
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, ErrEventID
	}
	return n, nil
}
