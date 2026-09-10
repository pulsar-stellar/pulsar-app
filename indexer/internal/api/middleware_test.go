package api

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pulsar-stellar/pulsar-app/indexer/internal/apierror"
)

// decodedError mirrors the ADR-017 error envelope for assertions: the wire
// class in error.code, the human message, and the catalog code in
// error.details.code per ADR-038.
type decodedError struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Details struct {
			Code string `json:"code"`
		} `json:"details"`
	} `json:"error"`
}

// newTestLogger returns a JSON logger writing to the returned buffer, so a test
// can read back the lines the middleware emitted. It carries no component; the
// caller scopes that in production and the middleware does not depend on it.
func newTestLogger() (*slog.Logger, *bytes.Buffer) {
	buf := &bytes.Buffer{}
	return slog.New(slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug})), buf
}

// findLine returns the first emitted log line whose msg equals msg, failing the
// test when none matches.
func findLine(t *testing.T, buf *bytes.Buffer, msg string) map[string]any {
	t.Helper()
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if line == "" {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("log line %q is not JSON: %v", line, err)
		}
		if m["msg"] == msg {
			return m
		}
	}
	t.Fatalf("no log line with msg=%q; got:\n%s", msg, buf.String())
	return nil
}

func assertJSONContentType(t *testing.T, rr *httptest.ResponseRecorder) {
	t.Helper()
	if ct := rr.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Errorf("Content-Type = %q, want it to contain application/json", ct)
	}
}

func TestWriteErrorRendersTheADR017Envelope(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		err        *apierror.Error
		wantStatus int
		wantClass  string
		wantDetail string
	}{
		{
			name:       "registered code carries its class and status",
			err:        apierror.New(apierror.CodeInternalPanic, "Internal server error"),
			wantStatus: http.StatusInternalServerError,
			wantClass:  "internal",
			wantDetail: "INTERNAL_PANIC",
		},
		{
			// ADR-038: the catalog code reaches error.details.code verbatim even
			// when unregistered, while the class and status degrade to internal.
			name:       "unregistered code degrades class but keeps its detail",
			err:        apierror.New(apierror.Code("VALIDATION_FUTURE"), "bad request"),
			wantStatus: http.StatusInternalServerError,
			wantClass:  "internal",
			wantDetail: "VALIDATION_FUTURE",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rr := httptest.NewRecorder()
			writeError(rr, tt.err)

			if rr.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rr.Code, tt.wantStatus)
			}
			assertJSONContentType(t, rr)

			var env decodedError
			if err := json.Unmarshal(rr.Body.Bytes(), &env); err != nil {
				t.Fatalf("body is not JSON: %v; body=%s", err, rr.Body.String())
			}
			if env.Error.Code != tt.wantClass {
				t.Errorf("error.code = %q, want %q", env.Error.Code, tt.wantClass)
			}
			if env.Error.Details.Code != tt.wantDetail {
				t.Errorf("error.details.code = %q, want %q", env.Error.Details.Code, tt.wantDetail)
			}
			if env.Error.Message != tt.err.Message() {
				t.Errorf("error.message = %q, want %q", env.Error.Message, tt.err.Message())
			}
		})
	}
}

// A marshal failure of our own fixed structs is a programmer error, but it must
// still produce a well-formed internal envelope rather than a half-sent body.
func TestWriteJSONFallsBackWhenMarshalFails(t *testing.T) {
	t.Parallel()

	rr := httptest.NewRecorder()
	// A channel cannot be marshalled, so json.Marshal fails; the requested 200
	// must be overridden by the 500 fallback.
	writeJSON(rr, http.StatusOK, map[string]any{"bad": make(chan int)})

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusInternalServerError)
	}
	assertJSONContentType(t, rr)
	if got := strings.TrimSpace(rr.Body.String()); got != lastResortError {
		t.Errorf("body = %q, want the last-resort envelope %q", got, lastResortError)
	}
}

func TestNewRequestIDIsUniqueHex(t *testing.T) {
	t.Parallel()

	a, b := newRequestID(), newRequestID()
	if a == b {
		t.Errorf("two ids were equal: %q", a)
	}
	if len(a) != 32 {
		t.Errorf("id length = %d, want 32 hex chars", len(a))
	}
	if _, err := hex.DecodeString(a); err != nil {
		t.Errorf("id %q is not hex: %v", a, err)
	}
}

func TestRequestIDFromContextIsEmptyWhenAbsent(t *testing.T) {
	t.Parallel()

	if got := RequestIDFromContext(context.Background()); got != "" {
		t.Errorf("RequestIDFromContext = %q, want empty for a bare context", got)
	}
}

func TestResponseRecorderGuardsDoubleWriteHeader(t *testing.T) {
	t.Parallel()

	rr := httptest.NewRecorder()
	rec := &responseRecorder{ResponseWriter: rr, status: http.StatusOK}

	rec.WriteHeader(http.StatusNotFound)
	rec.WriteHeader(http.StatusInternalServerError) // must be ignored

	if rec.status != http.StatusNotFound {
		t.Errorf("recorded status = %d, want %d", rec.status, http.StatusNotFound)
	}
	if rr.Code != http.StatusNotFound {
		t.Errorf("underlying status = %d, want %d", rr.Code, http.StatusNotFound)
	}
	if !rec.wroteHeader {
		t.Error("wroteHeader = false after WriteHeader")
	}
}

func TestResponseRecorderWriteDefaultsToOK(t *testing.T) {
	t.Parallel()

	rr := httptest.NewRecorder()
	rec := &responseRecorder{ResponseWriter: rr, status: http.StatusOK}

	n, err := rec.Write([]byte("hi"))
	if err != nil {
		t.Fatalf("Write returned error: %v", err)
	}
	if n != 2 {
		t.Errorf("Write returned n = %d, want 2", n)
	}
	if rec.status != http.StatusOK || !rec.wroteHeader {
		t.Errorf("after Write: status = %d, wroteHeader = %v; want 200, true", rec.status, rec.wroteHeader)
	}
	if rr.Body.String() != "hi" {
		t.Errorf("body = %q, want %q", rr.Body.String(), "hi")
	}
}

func TestRequestLoggerAssignsAndPropagatesTheID(t *testing.T) {
	t.Parallel()

	log, buf := newTestLogger()
	var seen string
	chain := RequestLogger(log)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = RequestIDFromContext(r.Context())
		w.WriteHeader(http.StatusNoContent)
	}))

	rr := httptest.NewRecorder()
	chain.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/health", nil))

	header := rr.Header().Get("X-Request-Id")
	if header == "" {
		t.Fatal("no X-Request-Id response header")
	}
	if seen != header {
		t.Errorf("handler saw id %q, header is %q; they must match", seen, header)
	}

	line := findLine(t, buf, "http_request")
	if line["level"] != "INFO" {
		t.Errorf("level = %v, want INFO", line["level"])
	}
	if line["request_id"] != header {
		t.Errorf("logged request_id = %v, want %q", line["request_id"], header)
	}
	if line["method"] != http.MethodGet {
		t.Errorf("logged method = %v, want %q", line["method"], http.MethodGet)
	}
	if line["path"] != "/health" {
		t.Errorf("logged path = %v, want /health", line["path"])
	}
	if line["status"].(float64) != float64(http.StatusNoContent) {
		t.Errorf("logged status = %v, want %d", line["status"], http.StatusNoContent)
	}
	if _, ok := line["duration_ms"].(float64); !ok {
		t.Errorf("duration_ms = %v, want a number", line["duration_ms"])
	}
}

func TestRequestLoggerCapturesStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		handler http.HandlerFunc
		want    int
	}{
		{
			name:    "explicit non-200 status",
			handler: func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNotFound) },
			want:    http.StatusNotFound,
		},
		{
			name:    "implicit 200 via Write",
			handler: func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) },
			want:    http.StatusOK,
		},
		{
			name:    "implicit 200 with no write",
			handler: func(http.ResponseWriter, *http.Request) {},
			want:    http.StatusOK,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			log, buf := newTestLogger()
			chain := RequestLogger(log)(tt.handler)

			rr := httptest.NewRecorder()
			chain.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/x", nil))

			line := findLine(t, buf, "http_request")
			if line["status"].(float64) != float64(tt.want) {
				t.Errorf("logged status = %v, want %d", line["status"], tt.want)
			}
		})
	}
}

// A path is URL-decoded before it reaches the log, so a crafted path can hold
// control characters. slog's JSON encoding must escape them: a newline must not
// forge a second log line, and the field must round-trip intact.
func TestRequestLoggerEscapesControlCharsInPath(t *testing.T) {
	t.Parallel()

	log, buf := newTestLogger()
	chain := RequestLogger(log)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))

	rr := httptest.NewRecorder()
	// %0A decodes to a newline, %09 to a tab, in r.URL.Path.
	chain.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/a%0Afake%09b", nil))

	nonEmpty := 0
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if line != "" {
			nonEmpty++
		}
	}
	if nonEmpty != 1 {
		t.Errorf("emitted %d JSON lines, want 1; a control char forged a line:\n%s", nonEmpty, buf.String())
	}

	line := findLine(t, buf, "http_request")
	if line["path"] != "/a\nfake\tb" {
		t.Errorf("logged path = %q, want the decoded control chars preserved intact", line["path"])
	}
}

func TestRecovererPassesThroughWithoutPanic(t *testing.T) {
	t.Parallel()

	log, buf := newTestLogger()
	called := false
	h := Recoverer(log)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		_, _ = w.Write([]byte("fine"))
	}))

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))

	if !called {
		t.Error("handler was not called")
	}
	if rr.Code != http.StatusOK || rr.Body.String() != "fine" {
		t.Errorf("response = %d %q, want 200 %q", rr.Code, rr.Body.String(), "fine")
	}
	if buf.Len() != 0 {
		t.Errorf("recoverer logged on a clean request: %s", buf.String())
	}
}

// A panic must become a logged event and a clean internal envelope, and the
// recovered value must reach the log but never the client.
func TestRecovererTurnsPanicIntoInternalEnvelope(t *testing.T) {
	t.Parallel()

	const secret = "secret detail 42"
	log, buf := newTestLogger()
	chain := RequestLogger(log)(Recoverer(log)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic(secret)
	})))

	rr := httptest.NewRecorder()
	chain.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/boom", nil))

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusInternalServerError)
	}
	assertJSONContentType(t, rr)
	if strings.Contains(rr.Body.String(), secret) {
		t.Errorf("panic value leaked to the client: %s", rr.Body.String())
	}

	var env decodedError
	if err := json.Unmarshal(rr.Body.Bytes(), &env); err != nil {
		t.Fatalf("body is not JSON: %v; body=%s", err, rr.Body.String())
	}
	if env.Error.Code != "internal" {
		t.Errorf("error.code = %q, want internal", env.Error.Code)
	}
	if env.Error.Details.Code != string(apierror.CodeInternalPanic) {
		t.Errorf("error.details.code = %q, want %q", env.Error.Details.Code, apierror.CodeInternalPanic)
	}
	if env.Error.Message != "Internal server error" {
		t.Errorf("error.message = %q, want the generic message", env.Error.Message)
	}

	panicLine := findLine(t, buf, "panic")
	if panicLine["level"] != "ERROR" {
		t.Errorf("panic line level = %v, want ERROR", panicLine["level"])
	}
	if stack, _ := panicLine["stack"].(string); stack == "" {
		t.Error("panic line carries no stack trace")
	}
	if pv, _ := panicLine["panic"].(string); !strings.Contains(pv, secret) {
		t.Errorf("panic line panic field = %q, want it to hold the recovered value", pv)
	}

	// The request log and the panic log must share one request_id, and the
	// request log must show the recovered 500.
	reqLine := findLine(t, buf, "http_request")
	if reqLine["request_id"] != panicLine["request_id"] {
		t.Errorf("request_id differs: request=%v panic=%v", reqLine["request_id"], panicLine["request_id"])
	}
	if reqLine["status"].(float64) != float64(http.StatusInternalServerError) {
		t.Errorf("request log status = %v, want 500", reqLine["status"])
	}
}

// A panic that fires after the handler already began the response cannot be
// cleanly replaced, so the recoverer logs it but leaves the started bytes.
func TestRecovererLeavesAnAlreadyStartedResponse(t *testing.T) {
	t.Parallel()

	log, buf := newTestLogger()
	chain := RequestLogger(log)(Recoverer(log)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
		_, _ = w.Write([]byte("partial"))
		panic("after write")
	})))

	rr := httptest.NewRecorder()
	chain.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/x", nil))

	if rr.Code != http.StatusTeapot {
		t.Errorf("status = %d, want %d (the started response, not a replacement)", rr.Code, http.StatusTeapot)
	}
	if rr.Body.String() != "partial" {
		t.Errorf("body = %q, want the already-written %q", rr.Body.String(), "partial")
	}
	// The panic is still recorded even though the response could not change.
	findLine(t, buf, "panic")
}

// http.ErrAbortHandler is the sentinel a handler panics with to abort silently;
// the recoverer must re-panic it rather than turn it into a 500.
func TestRecovererRepanicsErrAbortHandler(t *testing.T) {
	t.Parallel()

	log, _ := newTestLogger()
	h := Recoverer(log)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic(http.ErrAbortHandler)
	}))

	defer func() {
		if v := recover(); v != http.ErrAbortHandler {
			t.Fatalf("recovered %v, want http.ErrAbortHandler to propagate", v)
		}
	}()

	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	t.Fatal("ErrAbortHandler did not propagate")
}
