// Package decoder turns Soroban's ScVal wire values into the DecodedValue
// taxonomy the SDK and the explorer read.
//
// The taxonomy is fixed by ADR-023 and is a wire contract shared with
// packages/sdk, so a change here is a coordinated change across both.
//
// Nothing in this package calls xdr.ScVal.String(). That method renders a
// timepoint as a formatted local date rather than as its second count, which
// would carry the indexer's timezone into stored data. See ADR-033.
package decoder

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"strconv"
	"sync/atomic"

	"github.com/stellar/go-stellar-sdk/xdr"
)

// The variant names carried in DecodedValue.Type. They match the SDK's
// DecodedValueSchema exactly, which validates them with Zod at the client.
const (
	TypeAddress   = "address"
	TypeSymbol    = "symbol"
	TypeString    = "string"
	TypeBool      = "bool"
	TypeBytes     = "bytes"
	TypeU32       = "u32"
	TypeI32       = "i32"
	TypeU64       = "u64"
	TypeI64       = "i64"
	TypeU128      = "u128"
	TypeI128      = "i128"
	TypeU256      = "u256"
	TypeI256      = "i256"
	TypeTimepoint = "timepoint"
	TypeDuration  = "duration"
	TypeVec       = "vec"
	TypeMap       = "map"
	TypeVoid      = "void"
	TypeUnknown   = "unknown"
)

// DecodedValue is one decoded Soroban value.
//
// Exactly one of Value, Entries or XDR carries content, chosen by Type. The
// shape is a discriminated union on the wire, so the zero-valued fields are
// omitted rather than serialized as nulls.
//
// There is no tuple variant here. ADR-023 keeps tuple in the taxonomy, but a
// tuple is indistinguishable from a vec in XDR alone: Soroban encodes both as
// a vector, and only a decoder holding the contract's spec can tell them
// apart. This decoder works from XDR and always emits vec.
type DecodedValue struct {
	Type    string     `json:"type"`
	Value   any        `json:"value,omitempty"`
	Entries []MapEntry `json:"-"`
	XDR     string     `json:"xdr,omitempty"`
}

// MapEntry is one key and value of a Soroban map.
//
// A map is an ordered slice rather than a Go map, per ADR-023: keys are
// themselves values and need not be strings, wire ordering is meaningful, and
// duplicate keys must survive rather than collapse.
type MapEntry struct {
	Key   DecodedValue `json:"key"`
	Value DecodedValue `json:"value"`
}

// MarshalJSON renders the union. The map variant carries its entries under
// value, matching the SDK, while keeping them typed in Go.
func (v DecodedValue) MarshalJSON() ([]byte, error) {
	type payload struct {
		Type  string `json:"type"`
		Value any    `json:"value,omitempty"`
		XDR   string `json:"xdr,omitempty"`
	}

	out := payload{Type: v.Type, XDR: v.XDR}
	switch v.Type {
	case TypeMap:
		entries := v.Entries
		if entries == nil {
			entries = []MapEntry{}
		}
		out.Value = entries
	case TypeVoid:
		// void carries nothing at all.
	default:
		out.Value = v.Value
	}
	return json.Marshal(out)
}

// UnmarshalJSON reads the union back, so a stored value round trips.
func (v *DecodedValue) UnmarshalJSON(data []byte) error {
	var raw struct {
		Type  string          `json:"type"`
		Value json.RawMessage `json:"value"`
		XDR   string          `json:"xdr"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	v.Type = raw.Type
	v.XDR = raw.XDR
	v.Value = nil
	v.Entries = nil

	if len(raw.Value) == 0 {
		return nil
	}

	switch raw.Type {
	case TypeMap:
		return json.Unmarshal(raw.Value, &v.Entries)
	case TypeVec:
		var items []DecodedValue
		if err := json.Unmarshal(raw.Value, &items); err != nil {
			return err
		}
		v.Value = items
	case TypeU32, TypeI32:
		var n int64
		if err := json.Unmarshal(raw.Value, &n); err != nil {
			return err
		}
		v.Value = n
	case TypeBool:
		var b bool
		if err := json.Unmarshal(raw.Value, &b); err != nil {
			return err
		}
		v.Value = b
	default:
		var s string
		if err := json.Unmarshal(raw.Value, &s); err != nil {
			return err
		}
		v.Value = s
	}
	return nil
}

// Decoder converts ScVals and counts how often it falls back.
//
// The counter exists because degrading is silent by design: a protocol upgrade
// adding an ScVal variant produces correct-looking events full of unknowns, and
// nothing else would say so. The poller reports the count so a sudden rise is
// visible in operation rather than discovered by a consumer.
type Decoder struct {
	unknownCount atomic.Int64
}

// New returns a Decoder. The zero value is usable too.
func New() *Decoder { return &Decoder{} }

// UnknownCount reports how many values have degraded to the unknown fallback
// over this Decoder's lifetime.
func (d *Decoder) UnknownCount() int64 { return d.unknownCount.Load() }

// DecodeBase64 decodes one base64 XDR value, as it arrives from RPC.
//
// A value that is not parseable XDR is an error: nothing can be said about
// bytes that do not decode, not even their type. A value that parses but names
// a variant this decoder cannot handle is not an error, it is the unknown
// fallback, per ADR-023.
func (d *Decoder) DecodeBase64(encoded string) (DecodedValue, error) {
	var value xdr.ScVal
	if err := xdr.SafeUnmarshalBase64(encoded, &value); err != nil {
		return DecodedValue{}, fmt.Errorf("decoder: value is not valid ScVal XDR: %w", err)
	}
	return d.Decode(value), nil
}

// DecodeTopics decodes an event's topic list, preserving order.
func (d *Decoder) DecodeTopics(encoded []string) ([]DecodedValue, error) {
	topics := make([]DecodedValue, 0, len(encoded))
	for i, raw := range encoded {
		value, err := d.DecodeBase64(raw)
		if err != nil {
			return nil, fmt.Errorf("decoder: topic %d: %w", i, err)
		}
		topics = append(topics, value)
	}
	return topics, nil
}

// Decode converts one parsed ScVal. It never returns an error and never
// panics: an unhandled variant becomes the unknown fallback carrying its XDR,
// so one value this decoder cannot name does not discard the rest of a page.
func (d *Decoder) Decode(value xdr.ScVal) DecodedValue {
	switch value.Type {
	case xdr.ScValTypeScvBool:
		if b, ok := value.GetB(); ok {
			return DecodedValue{Type: TypeBool, Value: b}
		}

	case xdr.ScValTypeScvVoid:
		return DecodedValue{Type: TypeVoid}

	case xdr.ScValTypeScvU32:
		if n, ok := value.GetU32(); ok {
			return DecodedValue{Type: TypeU32, Value: int64(n)}
		}

	case xdr.ScValTypeScvI32:
		if n, ok := value.GetI32(); ok {
			return DecodedValue{Type: TypeI32, Value: int64(n)}
		}

	case xdr.ScValTypeScvU64:
		if n, ok := value.GetU64(); ok {
			return DecodedValue{Type: TypeU64, Value: strconv.FormatUint(uint64(n), 10)}
		}

	case xdr.ScValTypeScvI64:
		if n, ok := value.GetI64(); ok {
			return DecodedValue{Type: TypeI64, Value: strconv.FormatInt(int64(n), 10)}
		}

	case xdr.ScValTypeScvTimepoint:
		// Read from the raw count. String() would render a formatted local
		// date here and carry this machine's timezone into stored data. See
		// ADR-033.
		if t, ok := value.GetTimepoint(); ok {
			return DecodedValue{Type: TypeTimepoint, Value: strconv.FormatUint(uint64(t), 10)}
		}

	case xdr.ScValTypeScvDuration:
		if t, ok := value.GetDuration(); ok {
			return DecodedValue{Type: TypeDuration, Value: strconv.FormatUint(uint64(t), 10)}
		}

	case xdr.ScValTypeScvU128:
		if parts, ok := value.GetU128(); ok {
			return DecodedValue{Type: TypeU128, Value: uint128String(parts)}
		}

	case xdr.ScValTypeScvI128:
		if parts, ok := value.GetI128(); ok {
			return DecodedValue{Type: TypeI128, Value: int128String(parts)}
		}

	case xdr.ScValTypeScvU256:
		if parts, ok := value.GetU256(); ok {
			return DecodedValue{Type: TypeU256, Value: uint256String(parts)}
		}

	case xdr.ScValTypeScvI256:
		if parts, ok := value.GetI256(); ok {
			return DecodedValue{Type: TypeI256, Value: int256String(parts)}
		}

	case xdr.ScValTypeScvBytes:
		if b, ok := value.GetBytes(); ok {
			return DecodedValue{Type: TypeBytes, Value: hex.EncodeToString(b)}
		}

	case xdr.ScValTypeScvString:
		if s, ok := value.GetStr(); ok {
			return DecodedValue{Type: TypeString, Value: string(s)}
		}

	case xdr.ScValTypeScvSymbol:
		if s, ok := value.GetSym(); ok {
			return DecodedValue{Type: TypeSymbol, Value: string(s)}
		}

	case xdr.ScValTypeScvAddress:
		if address, ok := value.GetAddress(); ok {
			// An address that will not strkey-encode is malformed rather than
			// unnameable, so it degrades with its XDR like any other value
			// this decoder cannot represent.
			if encoded, err := address.String(); err == nil && encoded != "" {
				return DecodedValue{Type: TypeAddress, Value: encoded}
			}
		}

	case xdr.ScValTypeScvVec:
		if vec, ok := value.GetVec(); ok && vec != nil {
			items := make([]DecodedValue, 0, len(*vec))
			for _, item := range *vec {
				items = append(items, d.Decode(item))
			}
			return DecodedValue{Type: TypeVec, Value: items}
		}

	case xdr.ScValTypeScvMap:
		if m, ok := value.GetMap(); ok && m != nil {
			entries := make([]MapEntry, 0, len(*m))
			for _, entry := range *m {
				entries = append(entries, MapEntry{
					Key:   d.Decode(entry.Key),
					Value: d.Decode(entry.Val),
				})
			}
			return DecodedValue{Type: TypeMap, Entries: entries}
		}
	}

	return d.unknown(value)
}

// unknown produces the fallback, re-marshalling the parsed value rather than
// threading the original base64 through every call. Verified in ADR-033 to
// reproduce the input byte for byte.
func (d *Decoder) unknown(value xdr.ScVal) DecodedValue {
	d.unknownCount.Add(1)

	encoded, err := xdr.MarshalBase64(value)
	if err != nil {
		// Nothing else can be said about a value that will not re-encode. An
		// empty xdr field is still a valid unknown, and the SDK requires the
		// field to be non-empty, so a marker is used rather than a blank.
		return DecodedValue{Type: TypeUnknown, XDR: unencodableMarker}
	}
	return DecodedValue{Type: TypeUnknown, XDR: encoded}
}

// unencodableMarker stands in when a parsed value will not re-encode. It is
// valid base64 so the field stays a base64 string, and decodes to the empty
// slice, which no real ScVal does.
const unencodableMarker = "="

func int128String(parts xdr.Int128Parts) string {
	n := new(big.Int).SetInt64(int64(parts.Hi))
	n.Lsh(n, 64)
	return n.Add(n, new(big.Int).SetUint64(uint64(parts.Lo))).String()
}

func uint128String(parts xdr.UInt128Parts) string {
	n := new(big.Int).SetUint64(uint64(parts.Hi))
	n.Lsh(n, 64)
	return n.Add(n, new(big.Int).SetUint64(uint64(parts.Lo))).String()
}

func int256String(parts xdr.Int256Parts) string {
	n := new(big.Int).SetInt64(int64(parts.HiHi))
	for _, limb := range []uint64{uint64(parts.HiLo), uint64(parts.LoHi), uint64(parts.LoLo)} {
		n.Lsh(n, 64)
		n.Add(n, new(big.Int).SetUint64(limb))
	}
	return n.String()
}

func uint256String(parts xdr.UInt256Parts) string {
	n := new(big.Int).SetUint64(uint64(parts.HiHi))
	for _, limb := range []uint64{uint64(parts.HiLo), uint64(parts.LoHi), uint64(parts.LoLo)} {
		n.Lsh(n, 64)
		n.Add(n, new(big.Int).SetUint64(limb))
	}
	return n.String()
}

var _ = base64.StdEncoding
