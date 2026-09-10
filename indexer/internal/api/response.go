// Package api serves the indexer's read HTTP surface: the handlers the SDK and
// the explorer call, the chi router that mounts them, and the middleware that
// logs every request and turns a panic into a clean response.
//
// Every JSON response, success or failure, is the ADR-017 envelope. This file
// owns the failure half: rendering an apierror.Error as the error envelope,
// with the wire class in error.code and the catalog code in
// error.details.code, per ADR-038. The success half arrives with the first
// handler that needs it.
package api

import (
	"encoding/json"
	"net/http"

	"github.com/pulsar-stellar/pulsar-app/indexer/internal/apierror"
)

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
