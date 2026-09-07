package rpc

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/creachadair/jrpc2"
	protocol "github.com/stellar/go-stellar-sdk/protocols/rpc"
)

// showcaseContractID is pulsar-core's deployed showcase contract on testnet,
// recorded in CLAUDE.md and .env.example. Only the live check uses it.
const showcaseContractID = "CDNWTVUDKCCGW7GOC6SBLUFXXUCD2YDHWRDUSXZ6CYBQKQWLCUYYWI5L"

// rpcErr builds a *jrpc2.Error carrying code, mirroring what the SDK client
// returns for a JSON-RPC error response (filterError passes every code
// except Cancelled/DeadlineExceeded through unchanged).
func rpcErr(code jrpc2.Code) error {
	return &jrpc2.Error{Code: code, Message: "synthetic"}
}

func TestIsRangeError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil is not a range error", nil, false},
		{"a plain error is not a range error", errors.New("boom"), false},
		{"a canceled context is not a range error", context.Canceled, false},
		{"the -32600 code is a range error", rpcErr(rangeErrorCode), true},
		{
			"a -32600 wrapped once, as the client wraps it",
			fmt.Errorf("rpc: getEvents from %s: %w", "https://rpc.example", rpcErr(rangeErrorCode)),
			true,
		},
		{
			"a -32600 wrapped twice still resolves",
			fmt.Errorf("outer: %w", fmt.Errorf("inner: %w", rpcErr(rangeErrorCode))),
			true,
		},
		{"invalid params (-32602) is not a range error", rpcErr(-32602), false},
		{"internal error (-32603) is not a range error", rpcErr(-32603), false},
		{"a wrapped non-range code stays non-range", fmt.Errorf("rpc: %w", rpcErr(-32602)), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsRangeError(tt.err); got != tt.want {
				t.Errorf("IsRangeError(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestParseEventID(t *testing.T) {
	tests := []struct {
		name    string
		id      string
		want    int64
		wantErr bool
	}{
		{"a real fixture id has order 0", "0019251572429041664-0000000000", 0, false},
		{"the order is the second component", "12345-7", 7, false},
		{"a large order parses", "1-4294967296", 4294967296, false},
		{"no dash is malformed", "1234567890", 0, true},
		{"an empty order is malformed", "12345-", 0, true},
		{"a non-numeric order is malformed", "12345-abc", 0, true},
		{"an empty id is malformed", "", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseEventID(tt.id)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ParseEventID(%q) = %d, want an error", tt.id, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseEventID(%q) returned an unexpected error: %v", tt.id, err)
			}
			if got != tt.want {
				t.Errorf("ParseEventID(%q) = %d, want %d", tt.id, got, tt.want)
			}
		})
	}
}

func TestParseLedgerCloseTime(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    time.Time
		wantErr bool
	}{
		{"a UTC timestamp parses", "2026-08-27T12:00:00Z", time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC), false},
		{
			"an offset timestamp normalizes to UTC",
			"2026-08-27T12:00:00+01:00",
			time.Date(2026, 8, 27, 11, 0, 0, 0, time.UTC),
			false,
		},
		{"a non-timestamp is an error", "not-a-time", time.Time{}, true},
		{"an empty string is an error", "", time.Time{}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseLedgerCloseTime(tt.raw)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ParseLedgerCloseTime(%q) = %v, want an error", tt.raw, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseLedgerCloseTime(%q) returned an unexpected error: %v", tt.raw, err)
			}
			if !got.Equal(tt.want) {
				t.Errorf("ParseLedgerCloseTime(%q) = %v, want the same instant as %v", tt.raw, got, tt.want)
			}
			// The helper normalizes to UTC so stored timestamps never carry a
			// local zone (ADR-032). Equal alone would not catch a dropped
			// .UTC() because it compares instants; the location check does.
			if got.Location() != time.UTC {
				t.Errorf("ParseLedgerCloseTime(%q) location = %v, want UTC", tt.raw, got.Location())
			}
		})
	}
}

// TestIsRangeErrorLive confirms, against a real RPC node, that a startLedger
// below the retention floor surfaces as a range error through the SDK client
// and this package's %w wrapping. It is the empirical counterpart to the
// synthetic TestIsRangeError. It is opt-in: CI does not set the variable, so
// it skips rather than depending on the network.
func TestIsRangeErrorLive(t *testing.T) {
	if os.Getenv("PULSAR_TEST_LIVE_RPC") == "" {
		t.Skip("set PULSAR_TEST_LIVE_RPC=1 to run the live testnet range-error check")
	}

	url := os.Getenv("PULSAR_INDEXER_RPC_URL")
	if url == "" {
		url = "https://soroban-testnet.stellar.org"
	}

	client := NewClient(url, nil)
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Ledger 1000 is far below any plausible retention floor, so the node
	// rejects it with -32600. This is the exact error the poller keys on to
	// re-read the floor and retry (ADR-028 finding #4).
	_, err := client.GetEvents(ctx, protocol.GetEventsRequest{
		StartLedger: 1000,
		Filters: []protocol.EventFilter{{
			ContractIDs: []string{showcaseContractID},
		}},
		Pagination: &protocol.PaginationOptions{Limit: 1},
	})
	if err == nil {
		t.Fatal("expected a range error for startLedger 1000, got nil")
	}
	if !IsRangeError(err) {
		t.Fatalf("IsRangeError should be true for the live range error, got false: %v", err)
	}
}
