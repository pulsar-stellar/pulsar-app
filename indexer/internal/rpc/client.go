// Package rpc wraps the Stellar Soroban RPC client for the indexer's polling
// loop and classifies the errors it returns.
//
// The upstream rpcclient.Client already enforces error-first access: its
// methods return (T, error), and a non-nil error means the result is the zero
// value. JSON-RPC errors arrive as HTTP 200 with no result field and an error
// object (ADR-028 finding #2); the underlying jrpc2 library decodes that into
// a Go error, so no caller can reach result data without having first checked
// the error return.
//
// This package adds three things the raw client does not provide:
//
//  1. An interface (Caller) so the polling loop can be tested with a mock.
//  2. Error classification: IsRangeError identifies the -32600 code that
//     signals a startLedger before the retention floor, which the poller
//     retries once with a fresh floor (ADR-028 finding #4).
//  3. Context wrapping on every error, per the project's %w rule.
package rpc

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/stellar/go-stellar-sdk/clients/rpcclient"
	protocol "github.com/stellar/go-stellar-sdk/protocols/rpc"
)

// Caller is the subset of the Stellar RPC surface the indexer uses. The
// polling loop depends on this interface rather than on the concrete client,
// so tests can supply a mock without a network.
type Caller interface {
	// GetEvents fetches contract events matching the request. The response
	// carries OldestLedger and LatestLedger alongside the events, so the
	// caller can observe the retention window without a second call.
	GetEvents(ctx context.Context, req protocol.GetEventsRequest) (protocol.GetEventsResponse, error)

	// GetHealth returns the RPC node's health, including OldestLedger and
	// LatestLedger. Used to re-read the retention floor after a range error.
	GetHealth(ctx context.Context) (protocol.GetHealthResponse, error)
}

// Client wraps rpcclient.Client and implements Caller. It adds context to
// errors and is safe for concurrent use (the underlying client is too).
type Client struct {
	inner *rpcclient.Client
	url   string
}

// NewClient builds a Client pointing at the given RPC URL. The HTTP client
// may be nil, in which case http.DefaultClient is used. The caller must call
// Close when the client is no longer needed.
func NewClient(rpcURL string, httpClient *http.Client) *Client {
	return &Client{
		inner: rpcclient.NewClient(rpcURL, httpClient),
		url:   rpcURL,
	}
}

// Close releases the underlying connection.
func (c *Client) Close() error {
	if err := c.inner.Close(); err != nil {
		return fmt.Errorf("rpc: closing client for %s: %w", c.url, err)
	}
	return nil
}

// GetEvents fetches events matching the request.
func (c *Client) GetEvents(ctx context.Context, req protocol.GetEventsRequest) (protocol.GetEventsResponse, error) {
	resp, err := c.inner.GetEvents(ctx, req)
	if err != nil {
		return protocol.GetEventsResponse{}, fmt.Errorf("rpc: getEvents from %s: %w", c.url, err)
	}
	return resp, nil
}

// GetHealth returns the node's health status.
func (c *Client) GetHealth(ctx context.Context) (protocol.GetHealthResponse, error) {
	resp, err := c.inner.GetHealth(ctx)
	if err != nil {
		return protocol.GetHealthResponse{}, fmt.Errorf("rpc: getHealth from %s: %w", c.url, err)
	}
	return resp, nil
}

// --- Error classification ---

// rangeErrorCode is the JSON-RPC error code Soroban RPC returns when
// startLedger falls outside the retention window. Verified against testnet
// in ADR-028.
const rangeErrorCode = -32600

// IsRangeError reports whether err is (or wraps) the JSON-RPC -32600 error
// that signals a startLedger before the retention floor. The poller retries
// once with a fresh floor when it sees this.
//
// The jrpc2 library surfaces JSON-RPC errors as *jrpc2.Error carrying a
// numeric code. Since this package does not import jrpc2 directly (it is a
// transitive dependency of rpcclient), the check inspects the error string
// for the code rather than type-asserting. This is deliberate: a type
// assertion would couple the indexer to jrpc2's error type, which is an
// internal of the SDK client rather than part of its public API.
func IsRangeError(err error) bool {
	if err == nil {
		return false
	}
	// The jrpc2 library formats errors as "code -32600: message". The
	// rpcclient wraps that, and this package wraps it again. Walk the chain.
	for e := err; e != nil; e = errors.Unwrap(e) {
		msg := e.Error()
		if strings.Contains(msg, strconv.Itoa(rangeErrorCode)) &&
			strings.Contains(msg, "ledger") {
			return true
		}
	}
	return false
}

// ParseEventID splits an RPC event ID of the form "{toid}-{eventOrder}" and
// returns the event order as the event index within its ledger, per ADR-022
// and ADR-024.
func ParseEventID(id string) (eventIndex int64, err error) {
	parts := strings.SplitN(id, "-", 2)
	if len(parts) != 2 {
		return 0, fmt.Errorf("rpc: event id %q does not have the expected {toid}-{order} form", id)
	}
	idx, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("rpc: event id %q has a non-numeric order component: %w", id, err)
	}
	return idx, nil
}

// ParseLedgerCloseTime parses the ledgerClosedAt string from an RPC event
// into a time.Time. The format is RFC3339, verified against testnet.
func ParseLedgerCloseTime(raw string) (time.Time, error) {
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}, fmt.Errorf("rpc: ledgerClosedAt %q is not RFC3339: %w", raw, err)
	}
	return t.UTC(), nil
}
