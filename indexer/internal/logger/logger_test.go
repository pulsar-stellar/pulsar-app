package logger_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/pulsar-stellar/pulsar-app/indexer/internal/logger"
)

func newJSON(t *testing.T, level string) (*bytes.Buffer, func(string) *slogLine) {
	t.Helper()

	var buf bytes.Buffer
	l, err := logger.New(&buf, logger.Options{Level: level, Format: "json"})
	if err != nil {
		t.Fatalf("New: unexpected error: %v", err)
	}

	log := logger.Component(l, logger.ComponentRPC)
	return &buf, func(msg string) *slogLine {
		buf.Reset()
		log.Info(msg)
		return decodeOne(t, buf.Bytes())
	}
}

type slogLine map[string]any

func decodeOne(t *testing.T, b []byte) *slogLine {
	t.Helper()
	if len(bytes.TrimSpace(b)) == 0 {
		return nil
	}
	var line slogLine
	if err := json.Unmarshal(b, &line); err != nil {
		t.Fatalf("output is not JSON: %v\noutput: %s", err, b)
	}
	return &line
}

// Section 7.6 requires level, msg, timestamp, and component on every line.
func TestEveryLineCarriesTheRequiredFields(t *testing.T) {
	t.Parallel()

	_, emit := newJSON(t, "info")
	line := *emit("polling")

	for _, key := range []string{"level", "msg", "timestamp", "component"} {
		if _, ok := line[key]; !ok {
			t.Errorf("line is missing %q: %v", key, line)
		}
	}
	if line["msg"] != "polling" {
		t.Errorf("msg = %v, want %q", line["msg"], "polling")
	}
	if line["level"] != "INFO" {
		t.Errorf("level = %v, want INFO", line["level"])
	}
	if line["component"] != logger.ComponentRPC {
		t.Errorf("component = %v, want %q", line["component"], logger.ComponentRPC)
	}
}

// slog's own key is "time". A pipeline keyed on "timestamp" reads nothing from
// a line carrying "time", so the rename is behaviour and not cosmetics.
func TestTimestampReplacesSlogsTimeKey(t *testing.T) {
	t.Parallel()

	_, emit := newJSON(t, "info")
	line := *emit("started")

	if _, present := line["time"]; present {
		t.Errorf("line still carries slog's \"time\" key: %v", line)
	}

	ts, ok := line["timestamp"].(string)
	if !ok {
		t.Fatalf("timestamp is %T, want string: %v", line["timestamp"], line)
	}
	if _, err := time.Parse(time.RFC3339Nano, ts); err != nil {
		t.Errorf("timestamp %q does not parse as RFC3339: %v", ts, err)
	}
}

func TestLevelFiltersQuieterLines(t *testing.T) {
	t.Parallel()

	cases := []struct {
		level      string
		wantDebug  bool
		wantInfo   bool
		wantWarn   bool
		wantErrorL bool
	}{
		{"debug", true, true, true, true},
		{"info", false, true, true, true},
		{"warn", false, false, true, true},
		{"error", false, false, false, true},
	}

	for _, c := range cases {
		t.Run(c.level, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer
			l, err := logger.New(&buf, logger.Options{Level: c.level, Format: "json"})
			if err != nil {
				t.Fatalf("New(%q): unexpected error: %v", c.level, err)
			}
			log := logger.Component(l, logger.ComponentStore)

			emitted := func(fn func(string, ...any)) bool {
				buf.Reset()
				fn("x")
				return buf.Len() > 0
			}

			if got := emitted(log.Debug); got != c.wantDebug {
				t.Errorf("debug emitted = %v, want %v", got, c.wantDebug)
			}
			if got := emitted(log.Info); got != c.wantInfo {
				t.Errorf("info emitted = %v, want %v", got, c.wantInfo)
			}
			if got := emitted(log.Warn); got != c.wantWarn {
				t.Errorf("warn emitted = %v, want %v", got, c.wantWarn)
			}
			if got := emitted(log.Error); got != c.wantErrorL {
				t.Errorf("error emitted = %v, want %v", got, c.wantErrorL)
			}
		})
	}
}

func TestTextFormatIsPlainAndStillCarriesTheFields(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	l, err := logger.New(&buf, logger.Options{Level: "info", Format: "text"})
	if err != nil {
		t.Fatalf("New: unexpected error: %v", err)
	}
	logger.Component(l, logger.ComponentAPI).Info("serving")

	out := buf.String()
	if json.Valid(bytes.TrimSpace(buf.Bytes())) {
		t.Errorf("text format produced JSON: %s", out)
	}
	for _, want := range []string{"timestamp=", "level=INFO", `msg=serving`, "component=" + logger.ComponentAPI} {
		if !strings.Contains(out, want) {
			t.Errorf("text output %q does not contain %q", out, want)
		}
	}
	if strings.Contains(out, "time=") {
		t.Errorf("text output still carries slog's \"time\" key: %s", out)
	}
}

// An unrecognised level must not degrade into slog's zero value, which is
// LevelInfo's numeric neighbour and would silently turn debug logging on.
func TestNewRejectsUnknownLevelsAndFormats(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		opts     logger.Options
		wantWord string
	}{
		{"empty level", logger.Options{Level: "", Format: "json"}, "level"},
		{"unknown level", logger.Options{Level: "verbose", Format: "json"}, "verbose"},
		{"uppercase level", logger.Options{Level: "INFO", Format: "json"}, "INFO"},
		{"empty format", logger.Options{Level: "info", Format: ""}, "format"},
		{"unknown format", logger.Options{Level: "info", Format: "logfmt"}, "logfmt"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			l, err := logger.New(&bytes.Buffer{}, c.opts)
			if err == nil {
				t.Fatalf("expected an error for %+v", c.opts)
			}
			if l != nil {
				t.Errorf("New returned a logger alongside an error: %v", l)
			}
			if !strings.Contains(err.Error(), c.wantWord) {
				t.Errorf("error %q does not mention %q", err.Error(), c.wantWord)
			}
		})
	}
}

func TestComponentTagsOnlyTheDerivedLogger(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	root, err := logger.New(&buf, logger.Options{Level: "info", Format: "json"})
	if err != nil {
		t.Fatalf("New: unexpected error: %v", err)
	}

	logger.Component(root, logger.ComponentDecoder).Info("decoded")
	if got := (*decodeOne(t, buf.Bytes()))["component"]; got != logger.ComponentDecoder {
		t.Errorf("component = %v, want %q", got, logger.ComponentDecoder)
	}

	// The root is untouched by the derivation, so two components can be taken
	// from one root without either seeing the other's tag.
	buf.Reset()
	root.Info("bare")
	if _, tagged := (*decodeOne(t, buf.Bytes()))["component"]; tagged {
		t.Error("deriving a component tagged the root logger too")
	}
}

func TestComponentsAreDistinctPerDerivation(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	root, err := logger.New(&buf, logger.Options{Level: "info", Format: "json"})
	if err != nil {
		t.Fatalf("New: unexpected error: %v", err)
	}

	rpc := logger.Component(root, logger.ComponentRPC)
	store := logger.Component(root, logger.ComponentStore)

	buf.Reset()
	rpc.Info("a")
	if got := (*decodeOne(t, buf.Bytes()))["component"]; got != logger.ComponentRPC {
		t.Errorf("component = %v, want %q", got, logger.ComponentRPC)
	}

	buf.Reset()
	store.Info("b")
	if got := (*decodeOne(t, buf.Bytes()))["component"]; got != logger.ComponentStore {
		t.Errorf("component = %v, want %q", got, logger.ComponentStore)
	}
}

// Attributes a caller adds must survive alongside the component, since the
// HTTP layer attaches request_id, method, path, status and duration_ms this way.
func TestCallerAttributesSurviveAlongsideComponent(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	l, err := logger.New(&buf, logger.Options{Level: "info", Format: "json"})
	if err != nil {
		t.Fatalf("New: unexpected error: %v", err)
	}

	logger.Component(l, logger.ComponentAPI).Info("request",
		"request_id", "abc123", "method", "GET", "path", "/health", "status", 200, "duration_ms", 4)

	line := *decodeOne(t, buf.Bytes())
	if line["component"] != logger.ComponentAPI {
		t.Errorf("component = %v, want %q", line["component"], logger.ComponentAPI)
	}
	if line["request_id"] != "abc123" || line["method"] != "GET" || line["path"] != "/health" {
		t.Errorf("request fields lost: %v", line)
	}
	if line["status"] != float64(200) || line["duration_ms"] != float64(4) {
		t.Errorf("numeric fields lost: %v", line)
	}
}
