package decoder_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/pulsar-stellar/pulsar-app/indexer/internal/decoder"
	"github.com/stellar/go-stellar-sdk/xdr"
)

func must(t *testing.T, kind xdr.ScValType, v any) xdr.ScVal {
	t.Helper()
	value, err := xdr.NewScVal(kind, v)
	if err != nil {
		t.Fatalf("building %v: %v", kind, err)
	}
	return value
}

func decodeOne(t *testing.T, value xdr.ScVal) decoder.DecodedValue {
	t.Helper()
	return decoder.New().Decode(value)
}

func base64Of(t *testing.T, value xdr.ScVal) string {
	t.Helper()
	encoded, err := xdr.MarshalBase64(value)
	if err != nil {
		t.Fatalf("marshalling: %v", err)
	}
	return encoded
}

// --- fixtures ---

// Every fixture pair is decoded and compared. The live ones carry provenance a
// constructed value cannot: their expected amounts were checked against what
// the CLI reported when the transactions were submitted.
func TestFixtures(t *testing.T) {
	t.Parallel()

	paths, err := filepath.Glob(filepath.Join("testdata", "fixtures", "*.xdr"))
	if err != nil {
		t.Fatalf("listing fixtures: %v", err)
	}
	if len(paths) < 20 {
		t.Fatalf("found %d fixtures, want the full set; the directory looks wrong", len(paths))
	}

	for _, path := range paths {
		t.Run(strings.TrimSuffix(filepath.Base(path), ".xdr"), func(t *testing.T) {
			t.Parallel()

			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("reading %s: %v", path, err)
			}
			var value xdr.ScVal
			if err := xdr.SafeUnmarshal(raw, &value); err != nil {
				t.Fatalf("%s is not valid ScVal XDR: %v", path, err)
			}

			wantRaw, err := os.ReadFile(strings.TrimSuffix(path, ".xdr") + ".json")
			if err != nil {
				t.Fatalf("reading the expected JSON: %v", err)
			}

			got, err := json.Marshal(decodeOne(t, value))
			if err != nil {
				t.Fatalf("marshalling the decoded value: %v", err)
			}

			var wantAny, gotAny any
			if err := json.Unmarshal(wantRaw, &wantAny); err != nil {
				t.Fatalf("the expected JSON does not parse: %v", err)
			}
			if err := json.Unmarshal(got, &gotAny); err != nil {
				t.Fatalf("the decoded JSON does not parse: %v", err)
			}

			wantCanonical, _ := json.Marshal(wantAny)
			gotCanonical, _ := json.Marshal(gotAny)
			if string(wantCanonical) != string(gotCanonical) {
				t.Errorf("decoded to\n  %s\nwant\n  %s", gotCanonical, wantCanonical)
			}
		})
	}
}

// Every fixture must have both halves, or a pair added with one file missing
// would silently drop out of the suite.
func TestEveryFixtureHasBothHalves(t *testing.T) {
	t.Parallel()

	xdrs, _ := filepath.Glob(filepath.Join("testdata", "fixtures", "*.xdr"))
	jsons, _ := filepath.Glob(filepath.Join("testdata", "fixtures", "*.json"))

	if len(xdrs) != len(jsons) {
		t.Errorf("%d .xdr files and %d .json files", len(xdrs), len(jsons))
	}
	for _, path := range xdrs {
		if _, err := os.Stat(strings.TrimSuffix(path, ".xdr") + ".json"); err != nil {
			t.Errorf("%s has no expected JSON", filepath.Base(path))
		}
	}
}

// --- per-type decoding ---

func TestIntegerWidthsCarryTheRightJSONType(t *testing.T) {
	t.Parallel()

	// ADR-023: 32-bit values are JSON numbers because they always fit; every
	// wider width is a string because a JSON number rounds past 2^53.
	cases := []struct {
		name     string
		value    xdr.ScVal
		wantType string
		wantJSON string
	}{
		{"u32 max", must(t, xdr.ScValTypeScvU32, xdr.Uint32(4294967295)), "u32", `4294967295`},
		{"i32 min", must(t, xdr.ScValTypeScvI32, xdr.Int32(-2147483648)), "i32", `-2147483648`},
		{"u64 max", must(t, xdr.ScValTypeScvU64, xdr.Uint64(18446744073709551615)), "u64", `"18446744073709551615"`},
		{"i64 min", must(t, xdr.ScValTypeScvI64, xdr.Int64(-9223372036854775808)), "i64", `"-9223372036854775808"`},
		{"u128 past 2^64", must(t, xdr.ScValTypeScvU128, xdr.UInt128Parts{Hi: 1, Lo: 5}), "u128", `"18446744073709551621"`},
		{"i128 min", must(t, xdr.ScValTypeScvI128, xdr.Int128Parts{Hi: -1 << 63, Lo: 0}), "i128", `"-170141183460469231731687303715884105728"`},
		{"i128 negative one", must(t, xdr.ScValTypeScvI128, xdr.Int128Parts{Hi: -1, Lo: 1<<64 - 1}), "i128", `"-1"`},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			got := decodeOne(t, c.value)
			if got.Type != c.wantType {
				t.Fatalf("Type = %q, want %q", got.Type, c.wantType)
			}

			encoded, err := json.Marshal(got)
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}
			var envelope struct {
				Value json.RawMessage `json:"value"`
			}
			if err := json.Unmarshal(encoded, &envelope); err != nil {
				t.Fatalf("decoding: %v", err)
			}
			if string(envelope.Value) != c.wantJSON {
				t.Errorf("value = %s, want %s", envelope.Value, c.wantJSON)
			}
		})
	}
}

// ADR-033: ScVal.String() renders a timepoint as a formatted local date, which
// would carry this machine's timezone into stored data. The value is the second
// count.
func TestTimepointIsASecondCountNotADate(t *testing.T) {
	t.Parallel()

	got := decodeOne(t, must(t, xdr.ScValTypeScvTimepoint, xdr.TimePoint(1788255912)))

	if got.Type != "timepoint" {
		t.Fatalf("Type = %q, want timepoint", got.Type)
	}
	if got.Value != "1788255912" {
		t.Errorf("value = %v, want the second count \"1788255912\"", got.Value)
	}
	if s, ok := got.Value.(string); ok {
		for _, marker := range []string{"-", ":", " ", "UTC", "WAT"} {
			if strings.Contains(s, marker) {
				t.Errorf("value %q looks like a formatted date rather than a count", s)
			}
		}
	}

	duration := decodeOne(t, must(t, xdr.ScValTypeScvDuration, xdr.Duration(3600)))
	if duration.Type != "duration" || duration.Value != "3600" {
		t.Errorf("duration decoded to %+v, want 3600", duration)
	}
}

func TestBytesAreHexAndStringsAreText(t *testing.T) {
	t.Parallel()

	bytes := decodeOne(t, must(t, xdr.ScValTypeScvBytes, xdr.ScBytes([]byte{0xde, 0xad, 0xbe, 0xef})))
	if bytes.Type != "bytes" || bytes.Value != "deadbeef" {
		t.Errorf("bytes decoded to %+v, want hex deadbeef", bytes)
	}

	empty := decodeOne(t, must(t, xdr.ScValTypeScvBytes, xdr.ScBytes{}))
	if empty.Type != "bytes" || empty.Value != "" {
		t.Errorf("empty bytes decoded to %+v", empty)
	}

	text := decodeOne(t, must(t, xdr.ScValTypeScvString, xdr.ScString("héllo wörld")))
	if text.Type != "string" || text.Value != "héllo wörld" {
		t.Errorf("string decoded to %+v, want the text unchanged", text)
	}

	symbol := decodeOne(t, must(t, xdr.ScValTypeScvSymbol, xdr.ScSymbol("transfer")))
	if symbol.Type != "symbol" || symbol.Value != "transfer" {
		t.Errorf("symbol decoded to %+v", symbol)
	}
}

func TestBoolAndVoid(t *testing.T) {
	t.Parallel()

	for _, want := range []bool{true, false} {
		got := decodeOne(t, must(t, xdr.ScValTypeScvBool, want))
		if got.Type != "bool" || got.Value != want {
			t.Errorf("bool %v decoded to %+v", want, got)
		}
	}

	void := decodeOne(t, must(t, xdr.ScValTypeScvVoid, nil))
	if void.Type != "void" {
		t.Fatalf("Type = %q, want void", void.Type)
	}
	encoded, err := json.Marshal(void)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if string(encoded) != `{"type":"void"}` {
		t.Errorf("void encoded as %s, want just its type", encoded)
	}
}

// ADR-023 records that address carries account addresses as well as contract
// ids, which is why the variant is not named contractId.
func TestAddressCarriesAccountsAndContracts(t *testing.T) {
	t.Parallel()

	raw, err := os.ReadFile(filepath.Join("testdata", "fixtures", "deposit_topic1.xdr"))
	if err != nil {
		t.Fatalf("reading the fixture: %v", err)
	}
	var value xdr.ScVal
	if err := xdr.SafeUnmarshal(raw, &value); err != nil {
		t.Fatalf("unmarshalling: %v", err)
	}

	got := decodeOne(t, value)
	if got.Type != "address" {
		t.Fatalf("Type = %q, want address", got.Type)
	}
	account, ok := got.Value.(string)
	if !ok || !strings.HasPrefix(account, "G") {
		t.Errorf("value = %v, want a G-prefixed account address", got.Value)
	}
	if len(account) != 56 {
		t.Errorf("address %q is %d characters, want 56", account, len(account))
	}
}

// --- recursion ---

func TestVecDecodesRecursivelyAndKeepsOrder(t *testing.T) {
	t.Parallel()

	inner := xdr.ScVec{
		must(t, xdr.ScValTypeScvU32, xdr.Uint32(1)),
		must(t, xdr.ScValTypeScvSymbol, xdr.ScSymbol("inner")),
	}
	outer := xdr.ScVec{
		must(t, xdr.ScValTypeScvBool, true),
		must(t, xdr.ScValTypeScvVec, &inner),
		must(t, xdr.ScValTypeScvVoid, nil),
	}

	got := decodeOne(t, must(t, xdr.ScValTypeScvVec, &outer))
	if got.Type != "vec" {
		t.Fatalf("Type = %q, want vec", got.Type)
	}
	items, ok := got.Value.([]decoder.DecodedValue)
	if !ok {
		t.Fatalf("value is %T, want a slice of decoded values", got.Value)
	}
	if len(items) != 3 {
		t.Fatalf("got %d items, want 3", len(items))
	}
	if items[0].Type != "bool" || items[1].Type != "vec" || items[2].Type != "void" {
		t.Errorf("order or types wrong: %v %v %v", items[0].Type, items[1].Type, items[2].Type)
	}

	nested, ok := items[1].Value.([]decoder.DecodedValue)
	if !ok || len(nested) != 2 {
		t.Fatalf("the nested vec did not decode: %+v", items[1])
	}
	if nested[0].Type != "u32" || nested[1].Value != "inner" {
		t.Errorf("nested contents wrong: %+v", nested)
	}

	// An empty vec is a vec, not a void or an unknown.
	empty := xdr.ScVec{}
	if got := decodeOne(t, must(t, xdr.ScValTypeScvVec, &empty)); got.Type != "vec" {
		t.Errorf("an empty vec decoded to %q", got.Type)
	}
}

// ADR-023: a map is an ordered slice of entries because keys are values rather
// than strings, ordering is meaningful, and duplicates must survive.
func TestMapKeepsKeyTypesOrderAndDuplicates(t *testing.T) {
	t.Parallel()

	m := xdr.ScMap{
		xdr.ScMapEntry{Key: must(t, xdr.ScValTypeScvSymbol, xdr.ScSymbol("b")), Val: must(t, xdr.ScValTypeScvU32, xdr.Uint32(1))},
		xdr.ScMapEntry{Key: must(t, xdr.ScValTypeScvU32, xdr.Uint32(7)), Val: must(t, xdr.ScValTypeScvU32, xdr.Uint32(2))},
		xdr.ScMapEntry{Key: must(t, xdr.ScValTypeScvSymbol, xdr.ScSymbol("a")), Val: must(t, xdr.ScValTypeScvU32, xdr.Uint32(3))},
	}

	got := decodeOne(t, must(t, xdr.ScValTypeScvMap, &m))
	if got.Type != "map" {
		t.Fatalf("Type = %q, want map", got.Type)
	}
	if len(got.Entries) != 3 {
		t.Fatalf("got %d entries, want 3", len(got.Entries))
	}

	// Wire order is preserved rather than sorted.
	if got.Entries[0].Key.Value != "b" || got.Entries[2].Key.Value != "a" {
		t.Errorf("entries were reordered: %+v", got.Entries)
	}
	// A non-string key keeps its type, which a Record<string, T> could not do.
	if got.Entries[1].Key.Type != "u32" {
		t.Errorf("key 1 is %q, want u32", got.Entries[1].Key.Type)
	}

	encoded, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if !strings.Contains(string(encoded), `"key"`) || !strings.Contains(string(encoded), `"value"`) {
		t.Errorf("a map encoded as %s, want entries with key and value", encoded)
	}

	empty := xdr.ScMap{}
	emptyDecoded := decodeOne(t, must(t, xdr.ScValTypeScvMap, &empty))
	out, _ := json.Marshal(emptyDecoded)
	if string(out) != `{"type":"map","value":[]}` {
		t.Errorf("an empty map encoded as %s, want an empty array", out)
	}
}

// --- the unknown fallback ---

// ADR-023: an ScVal this decoder cannot name degrades rather than failing, so a
// protocol upgrade does not break a working client at read time on events
// already indexed.
func TestUnhandledTypesDegradeToUnknown(t *testing.T) {
	t.Parallel()

	contractCode := xdr.Uint32(42)
	wasmCode := xdr.ScErrorCodeScecInvalidInput

	cases := []struct {
		name  string
		value xdr.ScVal
	}{
		{"contract error", must(t, xdr.ScValTypeScvError, xdr.ScError{Type: xdr.ScErrorTypeSceContract, ContractCode: &contractCode})},
		{"wasm error", must(t, xdr.ScValTypeScvError, xdr.ScError{Type: xdr.ScErrorTypeSceWasmVm, Code: &wasmCode})},
		{"contract instance", must(t, xdr.ScValTypeScvContractInstance, xdr.ScContractInstance{Executable: xdr.ContractExecutable{Type: xdr.ContractExecutableTypeContractExecutableStellarAsset}})},
		{"ledger key nonce", must(t, xdr.ScValTypeScvLedgerKeyNonce, xdr.ScNonceKey{Nonce: 7})},
		{"ledger key contract instance", must(t, xdr.ScValTypeScvLedgerKeyContractInstance, nil)},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			got := decodeOne(t, c.value)
			if got.Type != "unknown" {
				t.Fatalf("Type = %q, want unknown", got.Type)
			}
			if got.XDR == "" {
				t.Fatal("the fallback carries no XDR; the SDK requires a non-empty string")
			}
			if got.XDR != base64Of(t, c.value) {
				t.Errorf("XDR = %q, want the value's own base64 %q", got.XDR, base64Of(t, c.value))
			}
			if got.Value != nil {
				t.Errorf("the fallback carries a value as well as XDR: %+v", got.Value)
			}
		})
	}
}

// The fallback is silent by design, so the count is what makes it visible.
func TestUnknownCountTracksFallbacks(t *testing.T) {
	t.Parallel()

	d := decoder.New()
	if d.UnknownCount() != 0 {
		t.Fatalf("a fresh decoder reports %d fallbacks", d.UnknownCount())
	}

	d.Decode(must(t, xdr.ScValTypeScvU32, xdr.Uint32(1)))
	d.Decode(must(t, xdr.ScValTypeScvSymbol, xdr.ScSymbol("ok")))
	if d.UnknownCount() != 0 {
		t.Errorf("handled values counted as fallbacks: %d", d.UnknownCount())
	}

	nonce := must(t, xdr.ScValTypeScvLedgerKeyNonce, xdr.ScNonceKey{Nonce: 1})
	d.Decode(nonce)
	d.Decode(nonce)
	if d.UnknownCount() != 2 {
		t.Errorf("UnknownCount = %d, want 2", d.UnknownCount())
	}

	// Nested unknowns are counted individually, since each is a value a
	// consumer cannot read.
	inner := xdr.ScVec{nonce, nonce, must(t, xdr.ScValTypeScvU32, xdr.Uint32(1))}
	d.Decode(must(t, xdr.ScValTypeScvVec, &inner))
	if d.UnknownCount() != 4 {
		t.Errorf("UnknownCount = %d, want 4 after two nested fallbacks", d.UnknownCount())
	}
}

func TestUnknownCountIsSafeUnderConcurrency(t *testing.T) {
	t.Parallel()

	d := decoder.New()
	nonce := must(t, xdr.ScValTypeScvLedgerKeyNonce, xdr.ScNonceKey{Nonce: 1})

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			d.Decode(nonce)
		}()
	}
	wg.Wait()

	if d.UnknownCount() != 50 {
		t.Errorf("UnknownCount = %d, want 50", d.UnknownCount())
	}
}

// --- entry points ---

func TestDecodeBase64RejectsWhatIsNotXDR(t *testing.T) {
	t.Parallel()

	d := decoder.New()
	for _, input := range []string{"", "not base64 at all", "AAAA", "!!!!"} {
		if _, err := d.DecodeBase64(input); err == nil {
			t.Errorf("DecodeBase64(%q) succeeded, want an error", input)
		}
	}

	// Unparseable bytes are an error, not a fallback: nothing can be said about
	// them, not even their type.
	if d.UnknownCount() != 0 {
		t.Errorf("a parse failure was counted as a fallback: %d", d.UnknownCount())
	}

	got, err := d.DecodeBase64("AAAADwAAAANmZWUA")
	if err != nil {
		t.Fatalf("DecodeBase64 on a real value: %v", err)
	}
	if got.Type != "symbol" || got.Value != "fee" {
		t.Errorf("decoded to %+v, want the symbol fee", got)
	}
}

func TestDecodeTopicsPreservesOrderAndReportsWhichFailed(t *testing.T) {
	t.Parallel()

	d := decoder.New()
	topics, err := d.DecodeTopics([]string{"AAAADwAAAAdkZXBvc2l0AA==", "AAAADwAAAANmZWUA"})
	if err != nil {
		t.Fatalf("DecodeTopics: %v", err)
	}
	if len(topics) != 2 || topics[0].Value != "deposit" || topics[1].Value != "fee" {
		t.Errorf("topics decoded to %+v", topics)
	}

	if _, err := d.DecodeTopics([]string{"AAAADwAAAANmZWUA", "garbage"}); err == nil {
		t.Fatal("DecodeTopics succeeded with a malformed topic")
	} else if !strings.Contains(err.Error(), "topic 1") {
		t.Errorf("error %q does not say which topic failed", err.Error())
	}

	empty, err := d.DecodeTopics(nil)
	if err != nil {
		t.Fatalf("DecodeTopics(nil): %v", err)
	}
	if empty == nil || len(empty) != 0 {
		t.Errorf("DecodeTopics(nil) = %v, want an empty slice", empty)
	}
}

// A stored value has to read back as what was stored, or the indexer's JSON
// columns cannot be served without re-decoding from XDR.
func TestDecodedValuesRoundTripThroughJSON(t *testing.T) {
	t.Parallel()

	paths, _ := filepath.Glob(filepath.Join("testdata", "fixtures", "*.xdr"))
	for _, path := range paths {
		t.Run(strings.TrimSuffix(filepath.Base(path), ".xdr"), func(t *testing.T) {
			t.Parallel()

			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("reading: %v", err)
			}
			var value xdr.ScVal
			if err := xdr.SafeUnmarshal(raw, &value); err != nil {
				t.Fatalf("unmarshalling: %v", err)
			}

			original := decodeOne(t, value)
			encoded, err := json.Marshal(original)
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}

			var back decoder.DecodedValue
			if err := json.Unmarshal(encoded, &back); err != nil {
				t.Fatalf("Unmarshal: %v", err)
			}

			again, err := json.Marshal(back)
			if err != nil {
				t.Fatalf("re-Marshal: %v", err)
			}
			if string(again) != string(encoded) {
				t.Errorf("round trip changed the value:\n  first  %s\n  second %s", encoded, again)
			}
		})
	}
}

// The decoder is a wire contract shared with the SDK, so the variant names have
// to match the shipped Zod schema exactly.
func TestVariantNamesMatchTheSDKSchema(t *testing.T) {
	t.Parallel()

	source, err := os.ReadFile(filepath.Join("..", "..", "..", "packages", "sdk", "src", "types.ts"))
	if err != nil {
		t.Skipf("the SDK source is not available, so the taxonomies cannot be compared: %v", err)
	}

	for _, name := range []string{
		decoder.TypeAddress, decoder.TypeSymbol, decoder.TypeString, decoder.TypeBool,
		decoder.TypeBytes, decoder.TypeU32, decoder.TypeI32, decoder.TypeU64,
		decoder.TypeI64, decoder.TypeU128, decoder.TypeI128, decoder.TypeU256,
		decoder.TypeI256, decoder.TypeTimepoint, decoder.TypeDuration,
		decoder.TypeVec, decoder.TypeMap, decoder.TypeVoid, decoder.TypeUnknown,
	} {
		if !strings.Contains(string(source), `z.literal('`+name+`')`) {
			t.Errorf("the SDK schema has no variant %q", name)
		}
	}

	// tuple is in the SDK's union but this decoder never emits it: Soroban
	// encodes a tuple as a vector, so only a decoder holding the contract spec
	// can tell them apart. See ADR-023.
	if !strings.Contains(string(source), `z.literal('tuple')`) {
		t.Error("the SDK dropped the tuple variant; ADR-023 keeps it for the spec-aware path")
	}
}
