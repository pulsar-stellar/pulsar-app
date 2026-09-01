package models_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/pulsar-stellar/pulsar-app/indexer/internal/models"
	"github.com/pulsar-stellar/pulsar-app/indexer/migrations"
)

const showcase = "CDNWTVUDKCCGW7GOC6SBLUFXXUCD2YDHWRDUSXZ6CYBQKQWLCUYYWI5L"

func sample() models.Contract {
	first := int64(4430000)
	return models.Contract{
		ID:                 showcase,
		AddedAt:            time.Date(2026, 9, 1, 10, 53, 7, 0, time.UTC),
		FirstIndexedLedger: &first,
		LastIndexedLedger:  4446467,
		Status:             models.StatusActive,
	}
}

// The struct is only right if it matches the table it reads from. Comparing db
// tags against the migration catches a column added, removed or renamed on one
// side only, which would otherwise surface as a scan error at runtime.
//
// This parses the CREATE TABLE statement, which is correct only while nothing
// alters contracts. The moment a migration adds, drops or renames a column on
// this table, move to the approach in event_test.go, which applies the
// migrations and reads pragma_table_info from the result. Parsing 0001 alone
// after an ALTER would report agreement that is not there, which is the
// reassuring direction to be wrong in.
func TestStructTagsMatchTheContractsTable(t *testing.T) {
	t.Parallel()

	ms, err := migrations.For(migrations.DirPostgres)
	if err != nil {
		t.Fatalf("loading migrations: %v", err)
	}
	if len(ms) == 0 {
		t.Fatal("no migrations to read the schema from")
	}

	columns := contractColumns(t, ms[0].Up)
	tags := dbTags(models.Contract{})

	sort.Strings(columns)
	sort.Strings(tags)
	if !reflect.DeepEqual(columns, tags) {
		t.Errorf("contracts columns and Contract db tags differ.\n  columns: %v\n  tags:    %v", columns, tags)
	}
}

// contractColumns pulls the column names out of the CREATE TABLE contracts
// statement, ignoring table-level constraints.
func contractColumns(t *testing.T, sql string) []string {
	t.Helper()

	start := strings.Index(sql, "CREATE TABLE contracts (")
	if start < 0 {
		t.Fatal("the migration has no CREATE TABLE contracts statement")
	}
	body := sql[start+len("CREATE TABLE contracts ("):]
	end := strings.Index(body, ");")
	if end < 0 {
		t.Fatal("the CREATE TABLE contracts statement is not terminated")
	}

	name := regexp.MustCompile(`^[a-z_]+`)
	var columns []string
	for _, line := range strings.Split(body[:end], "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "--") {
			continue
		}
		upper := strings.ToUpper(line)
		if strings.HasPrefix(upper, "UNIQUE") || strings.HasPrefix(upper, "PRIMARY KEY") ||
			strings.HasPrefix(upper, "FOREIGN KEY") || strings.HasPrefix(upper, "CONSTRAINT") {
			continue
		}
		if match := name.FindString(line); match != "" {
			columns = append(columns, match)
		}
	}
	return columns
}

func dbTags(v any) []string {
	typ := reflect.TypeOf(v)
	tags := make([]string, 0, typ.NumField())
	for i := 0; i < typ.NumField(); i++ {
		if tag := typ.Field(i).Tag.Get("db"); tag != "" {
			tags = append(tags, tag)
		}
	}
	return tags
}

// The identifier is a strkey, so it is a plain JSON string. A ,string tag on
// an already-string field would double-encode it and break the SDK's parse.
func TestMarshalProducesTheWireShape(t *testing.T) {
	t.Parallel()

	encoded, err := json.Marshal(sample())
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var generic map[string]any
	if err := json.Unmarshal(encoded, &generic); err != nil {
		t.Fatalf("the encoded contract is not an object: %v", err)
	}

	wantKeys := []string{"added_at", "first_indexed_ledger", "id", "last_indexed_ledger", "status"}
	var gotKeys []string
	for k := range generic {
		gotKeys = append(gotKeys, k)
	}
	sort.Strings(gotKeys)
	if !reflect.DeepEqual(gotKeys, wantKeys) {
		t.Errorf("keys = %v, want %v", gotKeys, wantKeys)
	}

	id, ok := generic["id"].(string)
	if !ok {
		t.Fatalf("id is %T, want a string", generic["id"])
	}
	if id != showcase {
		t.Errorf("id = %q, want %q", id, showcase)
	}
	if strings.HasPrefix(id, `"`) {
		t.Errorf("id = %q is double-encoded; the strkey must not carry a ,string tag", id)
	}
	if status, ok := generic["status"].(string); !ok || status != "active" {
		t.Errorf("status = %v, want the plain string \"active\"", generic["status"])
	}
}

func TestUnmarshalRoundTrips(t *testing.T) {
	t.Parallel()

	original := sample()
	encoded, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var decoded models.Contract
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if decoded.ID != original.ID || decoded.Status != original.Status ||
		decoded.LastIndexedLedger != original.LastIndexedLedger {
		t.Errorf("round trip lost scalars: %+v", decoded)
	}
	if !decoded.AddedAt.Equal(original.AddedAt) {
		t.Errorf("AddedAt = %v, want %v", decoded.AddedAt, original.AddedAt)
	}
	if decoded.FirstIndexedLedger == nil || *decoded.FirstIndexedLedger != *original.FirstIndexedLedger {
		t.Errorf("FirstIndexedLedger = %v, want %d", decoded.FirstIndexedLedger, *original.FirstIndexedLedger)
	}
}

// The status values are the indexer's half of a contract the SDK enforces with
// Zod. Drift in either direction is rejected at the client, so the lists are
// compared rather than trusted.
func TestStatusConstantsMatchTheSDKEnum(t *testing.T) {
	t.Parallel()

	path := filepath.Join("..", "..", "..", "packages", "sdk", "src", "types.ts")
	source, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("the SDK source is not available at %s, so the enums cannot be compared: %v", path, err)
	}

	match := regexp.MustCompile(`ContractStatusSchema = z\.enum\(\[([^\]]*)\]\)`).FindSubmatch(source)
	if match == nil {
		t.Fatalf("ContractStatusSchema was not found in %s; the check needs updating", path)
	}

	var fromSDK []string
	for _, raw := range strings.Split(string(match[1]), ",") {
		if value := strings.Trim(strings.TrimSpace(raw), `'"`); value != "" {
			fromSDK = append(fromSDK, value)
		}
	}

	fromGo := []string{
		string(models.StatusActive),
		string(models.StatusPaused),
		string(models.StatusError),
	}

	sort.Strings(fromSDK)
	sort.Strings(fromGo)
	if !reflect.DeepEqual(fromSDK, fromGo) {
		t.Errorf("status values differ.\n  SDK: %v\n  Go:  %v", fromSDK, fromGo)
	}
}

// null means the first poll has not completed, which is a different claim from
// "indexed up to ledger 0". The pointer is what keeps those apart.
func TestNullableFirstIndexedLedger(t *testing.T) {
	t.Parallel()

	unindexed := sample()
	unindexed.FirstIndexedLedger = nil

	encoded, err := json.Marshal(unindexed)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if !strings.Contains(string(encoded), `"first_indexed_ledger":null`) {
		t.Errorf("an unindexed contract encoded as %s, want an explicit null", encoded)
	}
	if unindexed.Indexed() {
		t.Error("Indexed() reported true for a contract with no first ledger")
	}

	var decoded models.Contract
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if decoded.FirstIndexedLedger != nil {
		t.Errorf("null decoded to %d, want nil", *decoded.FirstIndexedLedger)
	}

	// Zero is a real value and must survive as one, distinct from null.
	zero := int64(0)
	atZero := sample()
	atZero.FirstIndexedLedger = &zero
	encodedZero, err := json.Marshal(atZero)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if !strings.Contains(string(encodedZero), `"first_indexed_ledger":0`) {
		t.Errorf("ledger 0 encoded as %s, want an explicit 0", encodedZero)
	}
	if !atZero.Indexed() {
		t.Error("Indexed() reported false for a contract indexed at ledger 0")
	}
}

// ADR-017's wire contract is ISO 8601 with an offset, which is what the SDK
// validates with z.iso.datetime({ offset: true }).
func TestTimestampsMarshalAsRFC3339(t *testing.T) {
	t.Parallel()

	encoded, err := json.Marshal(sample())
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var generic map[string]any
	if err := json.Unmarshal(encoded, &generic); err != nil {
		t.Fatalf("decoding: %v", err)
	}

	addedAt, ok := generic["added_at"].(string)
	if !ok {
		t.Fatalf("added_at is %T, want a string", generic["added_at"])
	}
	if _, err := time.Parse(time.RFC3339, addedAt); err != nil {
		t.Errorf("added_at %q does not parse as RFC3339: %v", addedAt, err)
	}
	if !strings.HasSuffix(addedAt, "Z") && !regexp.MustCompile(`[+-]\d{2}:\d{2}$`).MatchString(addedAt) {
		t.Errorf("added_at %q carries no UTC offset, which the SDK requires", addedAt)
	}
}
