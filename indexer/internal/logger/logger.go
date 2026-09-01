// Package logger builds the indexer's structured logger.
//
// Every line carries level, msg, timestamp, and component, per section 7.6.
// The component is what makes a log searchable once the RPC poller, the
// decoder, the store, and the HTTP API are all writing to the same stream, so
// it is attached at construction rather than left to each call site to
// remember.
package logger

import (
	"fmt"
	"io"
	"log/slog"
	"strings"
)

// The components that write logs. Section 7.6 names rpc, decoder, store, and
// api; the rest exist because startup and shutdown happen before any of those
// are running and still need somewhere to report from.
const (
	ComponentMain     = "main"
	ComponentAPI      = "api"
	ComponentRPC      = "rpc"
	ComponentDecoder  = "decoder"
	ComponentStore    = "store"
	ComponentMigrator = "migrator"
)

// timestampKey replaces slog's default "time" key. Section 7.6 specifies
// "timestamp", and a log pipeline keyed on one name silently drops the other.
const timestampKey = "timestamp"

// Options selects the handler. Both fields take the values validated by
// internal/config, and this package revalidates rather than trusting them,
// because an unrecognised level would otherwise degrade into slog's zero value
// and quietly enable debug logging.
type Options struct {
	Level  string
	Format string
}

var levels = map[string]slog.Level{
	"debug": slog.LevelDebug,
	"info":  slog.LevelInfo,
	"warn":  slog.LevelWarn,
	"error": slog.LevelError,
}

// New builds a logger writing to w. The returned logger carries no component;
// callers derive one with Component before logging.
func New(w io.Writer, opts Options) (*slog.Logger, error) {
	level, ok := levels[opts.Level]
	if !ok {
		return nil, fmt.Errorf("log level %q is not one of %s", opts.Level, strings.Join(levelNames(), ", "))
	}

	handlerOpts := &slog.HandlerOptions{
		Level:       level,
		ReplaceAttr: renameTimeKey,
	}

	var handler slog.Handler
	switch opts.Format {
	case "json":
		handler = slog.NewJSONHandler(w, handlerOpts)
	case "text":
		handler = slog.NewTextHandler(w, handlerOpts)
	default:
		return nil, fmt.Errorf("log format %q is not one of json, text", opts.Format)
	}

	return slog.New(handler), nil
}

// Component returns a logger that tags every line it writes with name. Call it
// once per logger: slog appends attributes rather than replacing them, so
// deriving a component from a logger that already has one emits both.
func Component(l *slog.Logger, name string) *slog.Logger {
	return l.With("component", name)
}

func renameTimeKey(groups []string, a slog.Attr) slog.Attr {
	if len(groups) == 0 && a.Key == slog.TimeKey {
		a.Key = timestampKey
	}
	return a
}

func levelNames() []string {
	return []string{"debug", "info", "warn", "error"}
}
