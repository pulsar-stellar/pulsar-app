// Package apierror is the indexer's error-code catalog.
//
// Every failure the HTTP surface returns carries two codes. The wire class
// (Class) is one of the four values ADR-017 fixed and @pulsar-stellar/sdk
// validates, so a published client keeps working. The catalog code (Code) is a
// stable UPPER_SNAKE identifier that rides in the envelope's
// error.details.code, per ADR-038, giving a catalog-aware consumer a finer
// signal than four classes can express. This package owns the mapping from a
// code to its class and HTTP status; the api package renders the envelope.
//
// The catalog grows with the surface. A code is registered here only when a
// shipped path returns it, per ADR-037: the count follows the real failure
// surface rather than a target.
package apierror

import (
	"fmt"
	"net/http"
	"sort"
)

// Class is the coarse error category carried on the wire as error.code. It is
// the contract @pulsar-stellar/sdk@0.1.0 validates, fixed to these four values
// by ADR-017; a fifth would be a breaking SDK change.
type Class string

// The four wire classes. A consumer branches on these; the catalog Code in
// error.details.code refines them.
const (
	ClassValidation  Class = "validation"
	ClassNotFound    Class = "not_found"
	ClassRateLimited Class = "rate_limited"
	ClassInternal    Class = "internal"
)

// Code is a stable, catalogued error identifier carried in the envelope's
// error.details.code, per ADR-038. Its string value is a wire contract: it
// does not change without an ADR, the same discipline ADR-023 applies to the
// DecodedValue union.
type Code string

// CodeInternalPanic is returned when a handler panics and the recovery
// middleware turns the recovered value into a response. The value is logged
// server-side and never sent, so the message stays generic.
const CodeInternalPanic Code = "INTERNAL_PANIC"

// CodeNotFoundRoute is returned when a request matches no route. It is the
// router's not-found path, distinct from a request that reaches a handler and
// finds the resource itself absent, which gets its own code.
const CodeNotFoundRoute Code = "NOT_FOUND_ROUTE"

// CodeNotFoundMethod is returned when a request matches a route's path but not
// its method. ADR-017 fixes the status set at {200,400,404,429,500}, so the 405
// this would otherwise be collapses to a 404; the catalog code keeps it
// distinguishable from a genuinely unknown path.
const CodeNotFoundMethod Code = "NOT_FOUND_METHOD"

// CodeInternalStore is returned when a handler cannot read or write the store
// and so cannot answer. The underlying error is logged server-side and never
// sent, since a store error can carry a DSN or host detail; the message stays
// generic.
const CodeInternalStore Code = "INTERNAL_STORE"

// CodeValidationContractID is returned when a contract ID a request supplies,
// in the path of /contracts/:id or the contract_id field of a registration
// body, is not a well-formed Soroban contract ID. It is validate.ContractID
// failing at the HTTP boundary, before the store is touched.
const CodeValidationContractID Code = "VALIDATION_CONTRACT_ID"

// CodeValidationBody is returned when a request body is absent, is not the JSON
// object the route expects, or exceeds the size the handler will read. It
// covers the body envelope itself; a malformed value inside a well-formed body
// carries its own field code, such as CodeValidationContractID.
const CodeValidationBody Code = "VALIDATION_BODY"

// CodeNotFoundContract is returned when a request names a contract the indexer
// is not tracking: the absent case of GET and DELETE /contracts/:id. Per
// ADR-019 it rides on a 404 so absence is a structured signal, and it stays
// distinct from CodeNotFoundRoute, which means the path itself matched nothing.
const CodeNotFoundContract Code = "NOT_FOUND_CONTRACT"

// CodeValidationLimit is returned when the limit query parameter of the events
// list is not a positive integer within the range the SDK fixes, 1 to 500. The
// ceiling matches @pulsar-stellar/sdk's EVENT_QUERY_MAX_LIMIT and is lower than
// the store's own 10000 page cap, so the HTTP surface rejects a page the SDK
// would never ask for. See ADR-041.
const CodeValidationLimit Code = "VALIDATION_LIMIT"

// CodeValidationCursor is returned when the cursor query parameter of the
// events list is not a run of digits, the shape a cursor takes on the wire per
// ADR-021. A cursor the client did not receive from a prior next_cursor is
// rejected here rather than silently returning the first page.
const CodeValidationCursor Code = "VALIDATION_CURSOR"

// CodeValidationOrder is returned when the order query parameter of the events
// list is neither "asc" nor "desc". The match is case-sensitive to mirror the
// SDK's lowercase enum; an empty order is not this error, since the handler
// defaults it to descending. See ADR-041.
const CodeValidationOrder Code = "VALIDATION_ORDER"

// CodeValidationLedgerRange is returned when the from_ledger or to_ledger query
// parameter of the events list is negative, is not an integer, or names an
// empty window with from_ledger greater than to_ledger. It is the ledger-window
// counterpart of CodeValidationLimit, caught at the boundary before the store
// is touched.
const CodeValidationLedgerRange Code = "VALIDATION_LEDGER_RANGE"

// CodeValidationEventID is returned when the id path segment of GET /events/{id}
// is not a run of digits. An event id travels as a string of digits per ADR-021,
// so a non-numeric id cannot name an event and is rejected before the store is
// queried, distinct from a well-formed id that finds no event.
const CodeValidationEventID Code = "VALIDATION_EVENT_ID"

// CodeNotFoundEvent is returned when GET /events/{id} names a well-formed event
// id the indexer has not stored. Per ADR-021 it rides on a 404 so the SDK's
// event() can return null, and it stays distinct from CodeValidationEventID,
// which means the id was not a number in the first place.
const CodeNotFoundEvent Code = "NOT_FOUND_EVENT"

// entry is a code's registered metadata: the wire class a consumer switches
// on, the HTTP status the response carries, and the one-line meaning
// docs/error-codes.md publishes.
type entry struct {
	class   Class
	status  int
	meaning string
}

// registry is the catalog. Codes are added alongside the handler that first
// returns them, so an entry here always has a shipped path behind it.
var registry = map[Code]entry{
	CodeInternalPanic: {
		class:   ClassInternal,
		status:  http.StatusInternalServerError,
		meaning: "The indexer hit an unexpected condition and recovered. The failure is logged server-side; retrying may succeed.",
	},
	CodeNotFoundRoute: {
		class:   ClassNotFound,
		status:  http.StatusNotFound,
		meaning: "No endpoint matches the request path.",
	},
	CodeNotFoundMethod: {
		class:   ClassNotFound,
		status:  http.StatusNotFound,
		meaning: "The request path exists but does not accept this method.",
	},
	CodeInternalStore: {
		class:   ClassInternal,
		status:  http.StatusInternalServerError,
		meaning: "The indexer could not reach its store to answer the request. The failure is logged server-side; retrying may succeed.",
	},
	CodeValidationContractID: {
		class:   ClassValidation,
		status:  http.StatusBadRequest,
		meaning: "The contract ID is not a well-formed Soroban contract ID: the letter C followed by 55 base32 characters.",
	},
	CodeValidationBody: {
		class:   ClassValidation,
		status:  http.StatusBadRequest,
		meaning: "The request body is missing, is not the JSON object the endpoint expects, or is too large.",
	},
	CodeNotFoundContract: {
		class:   ClassNotFound,
		status:  http.StatusNotFound,
		meaning: "The indexer is not tracking the requested contract.",
	},
	CodeValidationLimit: {
		class:   ClassValidation,
		status:  http.StatusBadRequest,
		meaning: "The limit query parameter is not an integer between 1 and 500.",
	},
	CodeValidationCursor: {
		class:   ClassValidation,
		status:  http.StatusBadRequest,
		meaning: "The cursor query parameter is not a value returned by a prior page's next_cursor.",
	},
	CodeValidationOrder: {
		class:   ClassValidation,
		status:  http.StatusBadRequest,
		meaning: "The order query parameter is neither asc nor desc.",
	},
	CodeValidationLedgerRange: {
		class:   ClassValidation,
		status:  http.StatusBadRequest,
		meaning: "A ledger bound is negative or not an integer, or from_ledger is greater than to_ledger.",
	},
	CodeValidationEventID: {
		class:   ClassValidation,
		status:  http.StatusBadRequest,
		meaning: "The event id is not a run of digits.",
	},
	CodeNotFoundEvent: {
		class:   ClassNotFound,
		status:  http.StatusNotFound,
		meaning: "The indexer has not stored an event with the requested id.",
	},
}

// Class returns the wire class for c. An unregistered code degrades to
// ClassInternal rather than an empty class, so an unforeseen value can never
// reach the wire as a class @pulsar-stellar/sdk cannot validate.
func (c Code) Class() Class {
	if e, ok := registry[c]; ok {
		return e.class
	}
	return ClassInternal
}

// Status returns the HTTP status for c. An unregistered code degrades to 500
// for the same reason Class degrades to internal.
func (c Code) Status() int {
	if e, ok := registry[c]; ok {
		return e.status
	}
	return http.StatusInternalServerError
}

// Meaning returns the one-line description docs/error-codes.md publishes for c,
// or the empty string if c is not registered.
func (c Code) Meaning() string {
	return registry[c].meaning
}

// Registered reports whether c is in the catalog. It exists for the CI check
// that no handler returns a code the catalog does not document.
func (c Code) Registered() bool {
	_, ok := registry[c]
	return ok
}

// Codes returns every registered code, sorted, so docs generation and tests
// see a stable order rather than the map's random iteration.
func Codes() []Code {
	out := make([]Code, 0, len(registry))
	for c := range registry {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// Error is a catalogued failure. It carries a registered Code, a
// human-readable message that may change without notice, and an optional
// wrapped cause that keeps the Go error chain intact. It is the type handlers
// return and the responder renders into the ADR-017 error envelope.
type Error struct {
	code    Code
	message string
	cause   error
}

// New builds a catalogued error for code with message. It does not validate
// that code is registered: construction happens on request paths where a panic
// is forbidden, so an unregistered code degrades through the Code methods to an
// internal 500 instead. A test asserts every code a handler uses is registered.
func New(code Code, message string) *Error {
	return &Error{code: code, message: message}
}

// Wrap is New with cause preserved, so the underlying failure stays walkable
// with errors.Is and errors.As rather than flattened into a string.
func Wrap(code Code, message string, cause error) *Error {
	return &Error{code: code, message: message, cause: cause}
}

// Error renders the failure for a log line: the catalog code, the message, and
// the wrapped cause when there is one. It is not what reaches a client; the
// responder sends the message alone.
func (e *Error) Error() string {
	if e.cause != nil {
		return fmt.Sprintf("%s: %s: %v", e.code, e.message, e.cause)
	}
	return fmt.Sprintf("%s: %s", e.code, e.message)
}

// Unwrap exposes the wrapped cause to errors.Is and errors.As.
func (e *Error) Unwrap() error { return e.cause }

// Code returns the catalog code, for the envelope's error.details.code.
func (e *Error) Code() Code { return e.code }

// Message returns the client-facing message, for the envelope's error.message.
func (e *Error) Message() string { return e.message }

// Class returns the wire class, for the envelope's error.code.
func (e *Error) Class() Class { return e.code.Class() }

// Status returns the HTTP status the response carries.
func (e *Error) Status() int { return e.code.Status() }
