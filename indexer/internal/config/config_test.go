package config_test

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/pulsar-stellar/pulsar-app/indexer/internal/config"
)

const showcaseContract = "CDNWTVUDKCCGW7GOC6SBLUFXXUCD2YDHWRDUSXZ6CYBQKQWLCUYYWI5L"

// validEnv is the smallest environment Load accepts: the three required
// variables and nothing else. Tests copy it and mutate the copy, so no test
// can affect another.
func validEnv() map[string]string {
	return map[string]string{
		"PULSAR_INDEXER_DB_URL":  "file:./pulsar.db",
		"PULSAR_INDEXER_RPC_URL": "https://soroban-testnet.stellar.org",
		"PULSAR_INDEXER_NETWORK": "testnet",
	}
}

func getenvFrom(env map[string]string) config.Getenv {
	return func(name string) string { return env[name] }
}

func loadWith(t *testing.T, mutate func(map[string]string)) (config.Config, error) {
	t.Helper()
	env := validEnv()
	if mutate != nil {
		mutate(env)
	}
	return config.Load(getenvFrom(env))
}

func TestLoadAppliesDocumentedDefaults(t *testing.T) {
	t.Parallel()

	cfg, err := loadWith(t, nil)
	if err != nil {
		t.Fatalf("Load with the required variables set: unexpected error: %v", err)
	}

	if cfg.ListenAddr != ":8080" {
		t.Errorf("ListenAddr = %q, want %q", cfg.ListenAddr, ":8080")
	}
	if cfg.LogLevel != "info" {
		t.Errorf("LogLevel = %q, want %q", cfg.LogLevel, "info")
	}
	if cfg.LogFormat != "json" {
		t.Errorf("LogFormat = %q, want %q", cfg.LogFormat, "json")
	}
	if cfg.DBDriver != "sqlite" {
		t.Errorf("DBDriver = %q, want %q", cfg.DBDriver, "sqlite")
	}
	if cfg.PollInterval != 5*time.Second {
		t.Errorf("PollInterval = %v, want %v", cfg.PollInterval, 5*time.Second)
	}
	if cfg.BatchSize != 100 {
		t.Errorf("BatchSize = %d, want 100", cfg.BatchSize)
	}
	if len(cfg.BootstrapContracts) != 0 {
		t.Errorf("BootstrapContracts = %v, want empty", cfg.BootstrapContracts)
	}
}

func TestLoadReadsEveryVariable(t *testing.T) {
	t.Parallel()

	cfg, err := loadWith(t, func(env map[string]string) {
		env["PULSAR_INDEXER_LISTEN_ADDR"] = "127.0.0.1:9999"
		env["PULSAR_INDEXER_LOG_LEVEL"] = "debug"
		env["PULSAR_INDEXER_LOG_FORMAT"] = "text"
		env["PULSAR_INDEXER_DB_DRIVER"] = "postgres"
		env["PULSAR_INDEXER_DB_URL"] = "postgres://u:p@h:5432/pulsar?sslmode=require"
		env["PULSAR_INDEXER_RPC_URL"] = "http://localhost:8000"
		env["PULSAR_INDEXER_NETWORK"] = "local"
		env["PULSAR_INDEXER_POLL_INTERVAL_SEC"] = "30"
		env["PULSAR_INDEXER_BATCH_SIZE"] = "250"
		env["PULSAR_INDEXER_BOOTSTRAP_CONTRACTS"] = showcaseContract
	})
	if err != nil {
		t.Fatalf("Load with every variable set: unexpected error: %v", err)
	}

	want := config.Config{
		ListenAddr:         "127.0.0.1:9999",
		LogLevel:           "debug",
		LogFormat:          "text",
		DBDriver:           "postgres",
		DBURL:              "postgres://u:p@h:5432/pulsar?sslmode=require",
		RPCURL:             "http://localhost:8000",
		Network:            "local",
		PollInterval:       30 * time.Second,
		BatchSize:          250,
		BootstrapContracts: []string{showcaseContract},
	}

	if cfg.ListenAddr != want.ListenAddr || cfg.LogLevel != want.LogLevel ||
		cfg.LogFormat != want.LogFormat || cfg.DBDriver != want.DBDriver ||
		cfg.DBURL != want.DBURL || cfg.RPCURL != want.RPCURL ||
		cfg.Network != want.Network || cfg.PollInterval != want.PollInterval ||
		cfg.BatchSize != want.BatchSize {
		t.Errorf("Load = %+v, want %+v", cfg, want)
	}
	if len(cfg.BootstrapContracts) != 1 || cfg.BootstrapContracts[0] != showcaseContract {
		t.Errorf("BootstrapContracts = %v, want %v", cfg.BootstrapContracts, want.BootstrapContracts)
	}
}

// A required variable that is missing must fail startup with a message naming
// the variable, per section 7.5. A message that does not name it leaves the
// operator to guess which of three it was.
func TestLoadRequiresVariablesAndNamesTheMissingOne(t *testing.T) {
	t.Parallel()

	for _, name := range []string{
		"PULSAR_INDEXER_DB_URL",
		"PULSAR_INDEXER_RPC_URL",
		"PULSAR_INDEXER_NETWORK",
	} {
		t.Run(name+"/unset", func(t *testing.T) {
			t.Parallel()
			_, err := loadWith(t, func(env map[string]string) { delete(env, name) })
			assertErrorNaming(t, err, name)
		})

		t.Run(name+"/blank", func(t *testing.T) {
			t.Parallel()
			_, err := loadWith(t, func(env map[string]string) { env[name] = "   " })
			assertErrorNaming(t, err, name)
		})
	}
}

func TestLoadRejectsValuesOutsideTheDocumentedSets(t *testing.T) {
	t.Parallel()

	cases := []struct{ name, value string }{
		{"PULSAR_INDEXER_LOG_LEVEL", "verbose"},
		{"PULSAR_INDEXER_LOG_FORMAT", "yaml"},
		{"PULSAR_INDEXER_DB_DRIVER", "mysql"},
		{"PULSAR_INDEXER_NETWORK", "pubnet"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			_, err := loadWith(t, func(env map[string]string) { env[c.name] = c.value })
			assertErrorNaming(t, err, c.name)
		})
	}
}

func TestLoadRejectsRPCURLsThatAreNotHTTP(t *testing.T) {
	t.Parallel()

	for _, value := range []string{
		"ws://soroban-testnet.stellar.org",
		"file:///etc/passwd",
		"soroban-testnet.stellar.org",
		"https://",
		"://nope",
	} {
		t.Run(value, func(t *testing.T) {
			t.Parallel()
			_, err := loadWith(t, func(env map[string]string) {
				env["PULSAR_INDEXER_RPC_URL"] = value
			})
			assertErrorNaming(t, err, "PULSAR_INDEXER_RPC_URL")
		})
	}
}

func TestLoadRejectsNonPositiveAndUnparseableNumbers(t *testing.T) {
	t.Parallel()

	cases := []struct{ name, value string }{
		{"PULSAR_INDEXER_POLL_INTERVAL_SEC", "0"},
		{"PULSAR_INDEXER_POLL_INTERVAL_SEC", "-1"},
		{"PULSAR_INDEXER_POLL_INTERVAL_SEC", "5s"},
		{"PULSAR_INDEXER_BATCH_SIZE", "0"},
		{"PULSAR_INDEXER_BATCH_SIZE", "-100"},
		{"PULSAR_INDEXER_BATCH_SIZE", "lots"},
	}

	for _, c := range cases {
		t.Run(c.name+"="+c.value, func(t *testing.T) {
			t.Parallel()
			_, err := loadWith(t, func(env map[string]string) { env[c.name] = c.value })
			assertErrorNaming(t, err, c.name)
		})
	}
}

// ADR-028 measured getEvents rejecting a limit above 10000 with -32602. A batch
// size past the cap is caught at startup rather than on every poll.
func TestLoadRejectsBatchSizeAboveTheRPCPageLimit(t *testing.T) {
	t.Parallel()

	_, err := loadWith(t, func(env map[string]string) {
		env["PULSAR_INDEXER_BATCH_SIZE"] = "10001"
	})
	assertErrorNaming(t, err, "PULSAR_INDEXER_BATCH_SIZE")

	cfg, err := loadWith(t, func(env map[string]string) {
		env["PULSAR_INDEXER_BATCH_SIZE"] = "10000"
	})
	if err != nil {
		t.Fatalf("BATCH_SIZE at the cap: unexpected error: %v", err)
	}
	if cfg.BatchSize != config.MaxBatchSize {
		t.Errorf("BatchSize = %d, want %d", cfg.BatchSize, config.MaxBatchSize)
	}
}

func TestLoadParsesBootstrapContracts(t *testing.T) {
	t.Parallel()

	second := "C" + strings.Repeat("A", 55)

	cfg, err := loadWith(t, func(env map[string]string) {
		env["PULSAR_INDEXER_BOOTSTRAP_CONTRACTS"] = "  " + showcaseContract + " , " + second + "  "
	})
	if err != nil {
		t.Fatalf("Load with two bootstrap contracts: unexpected error: %v", err)
	}

	want := []string{showcaseContract, second}
	if len(cfg.BootstrapContracts) != len(want) {
		t.Fatalf("BootstrapContracts = %v, want %v", cfg.BootstrapContracts, want)
	}
	for i := range want {
		if cfg.BootstrapContracts[i] != want[i] {
			t.Errorf("BootstrapContracts[%d] = %q, want %q", i, cfg.BootstrapContracts[i], want[i])
		}
	}
}

// A typo in a supplied list is an error, not an entry to drop. Dropping it
// would leave the indexer tracking fewer contracts than it was told to, with
// nothing anywhere saying so.
func TestLoadRejectsMalformedBootstrapContracts(t *testing.T) {
	t.Parallel()

	for _, value := range []string{
		showcaseContract + ",",
		"," + showcaseContract,
		showcaseContract + ",,%s",
		"GDNWTVUDKCCGW7GOC6SBLUFXXUCD2YDHWRDUSXZ6CYBQKQWLCUYYWI5L",
		"CDNW",
		showcaseContract + "EXTRA",
	} {
		t.Run(value, func(t *testing.T) {
			t.Parallel()
			_, err := loadWith(t, func(env map[string]string) {
				env["PULSAR_INDEXER_BOOTSTRAP_CONTRACTS"] = value
			})
			assertErrorNaming(t, err, "PULSAR_INDEXER_BOOTSTRAP_CONTRACTS")
		})
	}
}

func TestLoadReturnsTheZeroConfigOnError(t *testing.T) {
	t.Parallel()

	cfg, err := loadWith(t, func(env map[string]string) {
		delete(env, "PULSAR_INDEXER_RPC_URL")
	})
	if err == nil {
		t.Fatal("expected an error")
	}
	if !reflect.DeepEqual(cfg, config.Config{}) {
		t.Errorf("Load returned %+v alongside an error, want the zero Config", cfg)
	}
}

func TestLoadLeavesPoolBoundsUnsetByDefault(t *testing.T) {
	t.Parallel()

	cfg, err := loadWith(t, nil)
	if err != nil {
		t.Fatalf("Load: unexpected error: %v", err)
	}

	// Zero means the operator set nothing, and internal/db supplies the
	// engine's default. It is not a pool of size zero.
	if cfg.DBPoolMax != 0 || cfg.DBPoolMin != 0 {
		t.Errorf("pool bounds = (%d, %d), want (0, 0) meaning unset", cfg.DBPoolMax, cfg.DBPoolMin)
	}
	if cfg.DBAllowInsecureTLS {
		t.Error("DBAllowInsecureTLS defaulted to true, want false")
	}
}

func TestLoadReadsPoolBounds(t *testing.T) {
	t.Parallel()

	cfg, err := loadWith(t, func(env map[string]string) {
		env["PULSAR_INDEXER_DB_POOL_MAX"] = "25"
		env["PULSAR_INDEXER_DB_POOL_MIN"] = "5"
	})
	if err != nil {
		t.Fatalf("Load: unexpected error: %v", err)
	}
	if cfg.DBPoolMax != 25 || cfg.DBPoolMin != 5 {
		t.Errorf("pool bounds = (%d, %d), want (25, 5)", cfg.DBPoolMax, cfg.DBPoolMin)
	}
}

func TestLoadRejectsUnusablePoolBounds(t *testing.T) {
	t.Parallel()

	cases := []struct{ name, value string }{
		{"PULSAR_INDEXER_DB_POOL_MAX", "0"},
		{"PULSAR_INDEXER_DB_POOL_MAX", "-1"},
		{"PULSAR_INDEXER_DB_POOL_MAX", "ten"},
		{"PULSAR_INDEXER_DB_POOL_MIN", "0"},
		{"PULSAR_INDEXER_DB_POOL_MIN", "-1"},
		{"PULSAR_INDEXER_DB_POOL_MIN", "two"},
	}

	for _, c := range cases {
		t.Run(c.name+"="+c.value, func(t *testing.T) {
			t.Parallel()
			_, err := loadWith(t, func(env map[string]string) { env[c.name] = c.value })
			assertErrorNaming(t, err, c.name)
		})
	}
}

// ADR-031: this flag permits a Postgres DSN that can fall back to plaintext,
// so an unparseable value must fail rather than resolve to either extreme.
func TestLoadParsesAllowInsecureTLS(t *testing.T) {
	t.Parallel()

	for _, c := range []struct {
		value string
		want  bool
	}{
		{"true", true}, {"TRUE", true}, {"1", true}, {"t", true},
		{"false", false}, {"FALSE", false}, {"0", false}, {"f", false},
	} {
		t.Run(c.value, func(t *testing.T) {
			t.Parallel()
			cfg, err := loadWith(t, func(env map[string]string) {
				env["PULSAR_INDEXER_DB_ALLOW_INSECURE_TLS"] = c.value
			})
			if err != nil {
				t.Fatalf("Load with %q: unexpected error: %v", c.value, err)
			}
			if cfg.DBAllowInsecureTLS != c.want {
				t.Errorf("DBAllowInsecureTLS = %v, want %v", cfg.DBAllowInsecureTLS, c.want)
			}
		})
	}

	for _, value := range []string{"yes", "no", "on", "maybe"} {
		t.Run("invalid/"+value, func(t *testing.T) {
			t.Parallel()
			_, err := loadWith(t, func(env map[string]string) {
				env["PULSAR_INDEXER_DB_ALLOW_INSECURE_TLS"] = value
			})
			assertErrorNaming(t, err, "PULSAR_INDEXER_DB_ALLOW_INSECURE_TLS")
		})
	}
}

func assertErrorNaming(t *testing.T, err error, name string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected an error naming %s, got nil", name)
	}
	if !strings.Contains(err.Error(), name) {
		t.Errorf("error %q does not name %s", err.Error(), name)
	}
}
