// Package api serves the indexer's read HTTP surface: the handlers the SDK and
// the explorer call, the chi router that mounts them, and the middleware that
// logs every request and turns a panic into a clean response.
//
// Every JSON response, success or failure, is the ADR-017 envelope. This file
// owns both halves: writeData wraps a success payload under data with an
// optional meta sibling, and writeError renders an apierror.Error as the error
// envelope, with the wire class in error.code and the catalog code in
// error.details.code, per ADR-038.
package api

import (
	"encoding/json"
	"net/http"

	"github.com/pulsar-stellar/pulsar-app/indexer/internal/apierror"
)

// dataEnvelope is the ADR-017 success wire shape. meta and next_cursor are
// siblings of data, not nested inside it. next_cursor is deliberately absent
// here: it belongs only to a paginated route and arrives with the events
// handler, so a non-paginated response omits the field rather than sending a
// null the SDK would have to treat as "no page".
type dataEnvelope struct {
	Data any   `json:"data"`
	Meta *meta `json:"meta,omitempty"`
}

// meta carries per-response metadata. took_ms is how long the handler spent
// producing the response, which the SDK's envelope schema accepts as a
// non-negative number.
type meta struct {
	TookMs float64 `json:"took_ms"`
}

// writeData renders data as the ADR-017 success envelope at status, with
// tookMs under meta.took_ms. It is the success counterpart to writeError; a
// handler measures its own elapsed time and passes it here.
func writeData(w http.ResponseWriter, status int, data any, tookMs float64) {
	writeJSON(w, status, dataEnvelope{Data: data, Meta: &meta{TookMs: tookMs}})
}

// errorEnvelope is the ADR-017 error wire shape. error.code is the four-value
// class @pulsar-stellar/sdk validates; error.details.code is the catalog code
// a catalog-aware consumer branches on, per ADR-038.
type errorEnvelope struct {
	Error errorBody `json:"error"`
}

type errorBody struct {
	Code    apierror.Class `json:"code"`
	Message string         `json:"message"`
	Details errorDetails   `json:"details"`
}

type errorDetails struct {
	Code apierror.Code `json:"code"`
}

// lastResortError is a hand-written internal envelope for the one path that
// cannot build a normal one: a marshal failure of our own fixed structs, which
// is a programmer error rather than a runtime condition. It keeps the client
// from hanging on a response that never comes.
const lastResortError = `{"error":{"code":"internal","message":"response encoding failed","details":{"code":"INTERNAL_PANIC"}}}`

// writeError renders err as the ADR-017 error envelope at err's registered HTTP
// status. The client sees only err.Message(); the catalog code and class carry
// the machine-readable signal. Caller logs the error; this function does not,
// so it stays usable from any layer.
func writeError(w http.ResponseWriter, err *apierror.Error) {
	writeJSON(w, err.Status(), errorEnvelope{Error: errorBody{
		Code:    err.Class(),
		Message: err.Message(),
		Details: errorDetails{Code: err.Code()},
	}})
}

// writeJSON marshals body before touching the response, so a marshal failure
// can still choose the status rather than corrupting a half-sent response. A
// write failure after the header is sent is unrecoverable and dropped: the
// client is gone, and the request logger has already captured the status.
func writeJSON(w http.ResponseWriter, status int, body any) {
	payload, err := json.Marshal(body)
	if err != nil {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(lastResortError))
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(payload)
}
