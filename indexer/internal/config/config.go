// Package config loads and validates the indexer's environment configuration.
//
// Every value the indexer needs comes from the environment and is validated
// once, at startup, before anything else runs. A missing required variable or
// an unusable value is a startup failure naming the variable, never a default
// quietly substituted for what the operator meant.
package config

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// MaxBatchSize is the largest page Soroban RPC will serve. getEvents rejects a
// limit above this with -32602, so a larger BatchSize would fail every call.
// See ADR-028.
const MaxBatchSize = 10000

// contractIDLength is the length of a Stellar contract strkey, which is a
// 'C' discriminant followed by 55 base32 characters.
const contractIDLength = 56

// Config is the indexer's validated configuration. Every field is populated by
// Load, and a Config that Load returned without error is safe to use as-is.
type Config struct {
	ListenAddr string
	LogLevel   string
	LogFormat  string

	DBDriver string
	DBURL    string

	// DBPoolMax and DBPoolMin are zero when the operator did not set them,
	// which means the driver's own default applies. internal/db owns those
	// defaults because they differ per engine.
	DBPoolMax int
	DBPoolMin int

	// DBAllowInsecureTLS permits a Postgres DSN that can fall back to an
	// unencrypted connection. Local use only. See ADR-031.
	DBAllowInsecureTLS bool

	RPCURL  string
	Network string

	PollInterval time.Duration
	BatchSize    int

	BootstrapContracts []string
}

// Getenv reads one environment variable, returning the empty string when it is
// unset. Load takes one so tests can supply an environment without mutating the
// process, which keeps them safe to run in parallel.
type Getenv func(string) string

var (
	validLogLevels  = []string{"debug", "info", "warn", "error"}
	validLogFormats = []string{"json", "text"}
	validDBDrivers  = []string{"sqlite", "postgres"}
	validNetworks   = []string{"testnet", "futurenet", "mainnet", "local"}
)

// Load reads the indexer's configuration from getenv and validates it.
//
// Three variables are required and have no default: PULSAR_INDEXER_DB_URL,
// PULSAR_INDEXER_RPC_URL, and PULSAR_INDEXER_NETWORK. Defaulting any of them
// would let a deployment run against the wrong database or the wrong network
// while looking healthy, which is the failure this project refuses to ship.
// The rest carry the defaults documented in .env.example.
func Load(getenv Getenv) (Config, error) {
	cfg := Config{
		ListenAddr: withDefault(getenv, "PULSAR_INDEXER_LISTEN_ADDR", ":8080"),
		LogLevel:   withDefault(getenv, "PULSAR_INDEXER_LOG_LEVEL", "info"),
		LogFormat:  withDefault(getenv, "PULSAR_INDEXER_LOG_FORMAT", "json"),
		DBDriver:   withDefault(getenv, "PULSAR_INDEXER_DB_DRIVER", "sqlite"),
	}

	if err := oneOf("PULSAR_INDEXER_LOG_LEVEL", cfg.LogLevel, validLogLevels); err != nil {
		return Config{}, err
	}
	if err := oneOf("PULSAR_INDEXER_LOG_FORMAT", cfg.LogFormat, validLogFormats); err != nil {
		return Config{}, err
	}
	if err := oneOf("PULSAR_INDEXER_DB_DRIVER", cfg.DBDriver, validDBDrivers); err != nil {
		return Config{}, err
	}

	dbURL, err := required(getenv, "PULSAR_INDEXER_DB_URL")
	if err != nil {
		return Config{}, err
	}
	cfg.DBURL = dbURL

	poolMax, err := optionalPositiveInt(getenv, "PULSAR_INDEXER_DB_POOL_MAX")
	if err != nil {
		return Config{}, err
	}
	cfg.DBPoolMax = poolMax

	poolMin, err := optionalPositiveInt(getenv, "PULSAR_INDEXER_DB_POOL_MIN")
	if err != nil {
		return Config{}, err
	}
	cfg.DBPoolMin = poolMin

	allowInsecure, err := boolean(getenv, "PULSAR_INDEXER_DB_ALLOW_INSECURE_TLS", false)
	if err != nil {
		return Config{}, err
	}
	cfg.DBAllowInsecureTLS = allowInsecure

	rpcURL, err := required(getenv, "PULSAR_INDEXER_RPC_URL")
	if err != nil {
		return Config{}, err
	}
	if err := httpURL("PULSAR_INDEXER_RPC_URL", rpcURL); err != nil {
		return Config{}, err
	}
	cfg.RPCURL = rpcURL

	network, err := required(getenv, "PULSAR_INDEXER_NETWORK")
	if err != nil {
		return Config{}, err
	}
	if err := oneOf("PULSAR_INDEXER_NETWORK", network, validNetworks); err != nil {
		return Config{}, err
	}
	cfg.Network = network

	pollSec, err := positiveInt(getenv, "PULSAR_INDEXER_POLL_INTERVAL_SEC", 5)
	if err != nil {
		return Config{}, err
	}
	cfg.PollInterval = time.Duration(pollSec) * time.Second

	batchSize, err := positiveInt(getenv, "PULSAR_INDEXER_BATCH_SIZE", 100)
	if err != nil {
		return Config{}, err
	}
	if batchSize > MaxBatchSize {
		return Config{}, fmt.Errorf(
			"PULSAR_INDEXER_BATCH_SIZE is %d, which exceeds the %d page limit Soroban RPC enforces; every getEvents call would be rejected",
			batchSize, MaxBatchSize)
	}
	cfg.BatchSize = batchSize

	contracts, err := contractList(getenv, "PULSAR_INDEXER_BOOTSTRAP_CONTRACTS")
	if err != nil {
		return Config{}, err
	}
	cfg.BootstrapContracts = contracts

	return cfg, nil
}

func withDefault(getenv Getenv, name, fallback string) string {
	if v := strings.TrimSpace(getenv(name)); v != "" {
		return v
	}
	return fallback
}

func required(getenv Getenv, name string) (string, error) {
	v := strings.TrimSpace(getenv(name))
	if v == "" {
		return "", fmt.Errorf("%s is required and is not set", name)
	}
	return v, nil
}

func oneOf(name, value string, allowed []string) error {
	for _, a := range allowed {
		if value == a {
			return nil
		}
	}
	return fmt.Errorf("%s is %q, which is not one of %s", name, value, strings.Join(allowed, ", "))
}

func httpURL(name, value string) error {
	u, err := url.Parse(value)
	if err != nil {
		return fmt.Errorf("%s is not a valid URL: %w", name, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("%s has scheme %q, but only http and https are supported", name, u.Scheme)
	}
	if u.Host == "" {
		return fmt.Errorf("%s has no host", name)
	}
	return nil
}

func positiveInt(getenv Getenv, name string, fallback int) (int, error) {
	raw := strings.TrimSpace(getenv(name))
	if raw == "" {
		return fallback, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("%s is %q, which is not a whole number: %w", name, raw, err)
	}
	if n <= 0 {
		return 0, fmt.Errorf("%s is %d, but it must be greater than zero", name, n)
	}
	return n, nil
}

// optionalPositiveInt reads a variable that may be unset. Unset returns zero,
// which callers read as "no override". A value that is set must still be a
// positive whole number, so a typo is an error rather than a silent default.
func optionalPositiveInt(getenv Getenv, name string) (int, error) {
	if strings.TrimSpace(getenv(name)) == "" {
		return 0, nil
	}
	return positiveInt(getenv, name, 0)
}

func boolean(getenv Getenv, name string, fallback bool) (bool, error) {
	raw := strings.TrimSpace(getenv(name))
	if raw == "" {
		return fallback, nil
	}
	v, err := strconv.ParseBool(raw)
	if err != nil {
		return false, fmt.Errorf("%s is %q, which is not a boolean; use true or false", name, raw)
	}
	return v, nil
}

// contractList parses a comma-separated list of contract IDs. The list is
// optional, but a malformed entry in a list the operator did supply is an
// error rather than something to drop, because a typo would otherwise mean the
// indexer silently tracks fewer contracts than it was told to.
func contractList(getenv Getenv, name string) ([]string, error) {
	raw := strings.TrimSpace(getenv(name))
	if raw == "" {
		return []string{}, nil
	}

	parts := strings.Split(raw, ",")
	ids := make([]string, 0, len(parts))
	for i, p := range parts {
		id := strings.TrimSpace(p)
		if id == "" {
			return nil, fmt.Errorf("%s has an empty entry at position %d", name, i+1)
		}
		if len(id) != contractIDLength || !strings.HasPrefix(id, "C") {
			return nil, fmt.Errorf(
				"%s entry %d is %q, which is not a contract ID; expected %d characters beginning with C",
				name, i+1, id, contractIDLength)
		}
		ids = append(ids, id)
	}
	return ids, nil
}
