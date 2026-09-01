package db

import (
	"database/sql"
	"fmt"
	"net/url"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
)

// safeSSLModes are the sslmode values that cannot end on an unencrypted
// connection. Listed for the error message; the check itself asks the parsed
// config rather than reading the string.
var safeSSLModes = []string{"require", "verify-ca", "verify-full"}

// openPostgres opens the Postgres database at dsn.
//
// The DSN is validated eagerly and the handle is opened lazily, so a
// misconfigured connection string fails here, at startup, with a message the
// operator can act on, rather than on the first query.
func openPostgres(d Driver, dsn string, allowInsecureTLS bool) (*sql.DB, error) {
	if strings.TrimSpace(dsn) == "" {
		return nil, fmt.Errorf("postgres: the database URL is empty")
	}

	connConfig, err := pgx.ParseConfig(dsn)
	if err != nil {
		// The parse error can quote the DSN back, so it is not wrapped.
		return nil, fmt.Errorf("postgres: the database URL is not a valid connection string")
	}

	if !allowInsecureTLS {
		if err := requireEncryptedConnection(connConfig); err != nil {
			return nil, err
		}
	}

	handle := stdlib.OpenDB(*connConfig)
	handle.SetMaxOpenConns(d.PoolMax)
	handle.SetMaxIdleConns(d.PoolMin)

	return handle, nil
}

// requireEncryptedConnection rejects a config that could complete a
// connection without TLS.
//
// pgx follows libpq, whose default sslmode is "prefer": attempt TLS, then fall
// back to plaintext if the server declines. pgx represents that fallback as an
// entry in Fallbacks with a nil TLSConfig, and taking it is a successful
// connection carrying the password and every row in cleartext, with nothing
// reporting the downgrade.
//
// The check is structural rather than textual. Reading sslmode out of the
// string would need to handle the URL and keyword forms separately, would miss
// values pgx normalizes, and would need revisiting whenever libpq gains a mode.
// Asking the parsed config whether plaintext is reachable answers the question
// that matters, once, for every DSN shape. See ADR-031.
func requireEncryptedConnection(cfg *pgx.ConnConfig) error {
	insecure := cfg.TLSConfig == nil
	for _, fallback := range cfg.Fallbacks {
		if fallback.TLSConfig == nil {
			insecure = true
			break
		}
	}
	if !insecure {
		return nil
	}

	return fmt.Errorf(
		"postgres: PULSAR_INDEXER_DB_URL permits an unencrypted connection; set sslmode to one of %s. "+
			"Omitting sslmode means sslmode=prefer, which silently falls back to plaintext carrying the password. "+
			"Set PULSAR_INDEXER_DB_ALLOW_INSECURE_TLS=true only for local development",
		strings.Join(safeSSLModes, ", "))
}

// redactDSN renders a connection string safe to put in an error or a log line.
// A Postgres DSN embeds the password in its userinfo, and a SQLite DSN can
// carry a filesystem path worth keeping out of logs, so neither is ever
// rendered whole. See ADR-031.
func redactDSN(dsn string) string {
	parsed, err := url.Parse(dsn)
	if err != nil || parsed.Scheme == "" {
		return "[redacted]"
	}
	if parsed.User != nil {
		parsed.User = url.User("[redacted]")
	}
	parsed.RawQuery = ""
	parsed.Path = ""
	return parsed.Redacted()
}
