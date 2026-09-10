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
