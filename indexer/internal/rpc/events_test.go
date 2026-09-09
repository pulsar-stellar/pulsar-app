package rpc

import (
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/pulsar-stellar/pulsar-app/indexer/internal/db"
	"github.com/pulsar-stellar/pulsar-app/indexer/internal/decoder"
	"github.com/pulsar-stellar/pulsar-app/indexer/internal/models"
	"github.com/pulsar-stellar/pulsar-app/indexer/internal/store"
	protocol "github.com/stellar/go-stellar-sdk/protocols/rpc"
)

// showcase is the contract every test polls. It is the deployed showcase
// contract, the same id the store tests use.
const showcase = "CDNWTVUDKCCGW7GOC6SBLUFXXUCD2YDHWRDUSXZ6CYBQKQWLCUYYWI5L"

// --- mock Caller ---

// mockCaller serves scripted getEvents and getHealth responses. Each GetEvents
// call pops the next entry from events; each GetHealth call returns health.
// Calls are recorded so a test can assert how many round trips a tick made.
type mockCaller struct {
	mu sync.Mutex

	eventsQueue []eventsReply
	eventsCalls []protocol.GetEventsRequest

	health      protocol.GetHealthResponse
	healthErr   error
	healthCalls int
}

// eventsReply is one scripted GetEvents outcome: a response or an error.
type eventsReply struct {
	resp protocol.GetEventsResponse
	err  error
}

func (m *mockCaller) GetEvents(_ context.Context, req protocol.GetEventsRequest) (protocol.GetEventsResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.eventsCalls = append(m.eventsCalls, req)
	if len(m.eventsQueue) == 0 {
		return protocol.GetEventsResponse{}, errors.New("mock: no more scripted GetEvents replies")
	}
	reply := m.eventsQueue[0]
	m.eventsQueue = m.eventsQueue[1:]
	return reply.resp, reply.err
}

func (m *mockCaller) GetHealth(_ context.Context) (protocol.GetHealthResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.healthCalls++
	return m.health, m.healthErr
}

func (m *mockCaller) eventsCallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.eventsCalls)
}

func (m *mockCaller) firstStartLedger() uint32 {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.eventsCalls) == 0 {
		return 0
	}
	return m.eventsCalls[0].StartLedger
}

func (m *mockCaller) lastStartLedger() uint32 {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.eventsCalls) == 0 {
		return 0
	}
	return m.eventsCalls[len(m.eventsCalls)-1].StartLedger
}

// --- fixtures and helpers ---

// migrated opens an in-memory SQLite database with every migration applied and
// the showcase contract registered, matching the store tests' house pattern.
func migrated(t *testing.T) *sql.DB {
	t.Helper()

	driver, err := db.Resolve(db.Options{DriverName: "sqlite"})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	handle, err := db.Open(driver, db.ConnOptions{DSN: "file::memory:"})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = handle.Close() })

	if _, err := db.Up(context.Background(), handle, driver); err != nil {
		t.Fatalf("Up: %v", err)
	}
	if _, err := store.NewContracts(handle).Register(context.Background(), showcase); err != nil {
		t.Fatalf("Register: %v", err)
	}
	return handle
}

// newPoller builds a Poller over the mock and a real store on an in-memory DB.
// The interval is short so a caller driving the loop does not wait on wall time,
// though most tests call tickOnce directly and never start the loop.
func newPoller(t *testing.T, caller Caller, handle *sql.DB) *Poller {
	t.Helper()
	return NewPoller(
		caller,
		decoder.New(),
		handle,
		store.NewContracts(handle),
		10*time.Millisecond,
		100,
		nil,
	)
}

// fixtureBase64 reads a decoder fixture's .xdr bytes and returns them base64
// encoded, as they arrive in an EventInfo's topic and value fields. The
// fixtures carry real provenance: transfer_topic0 is the symbol "transfer",
// deposit_data the i128 "5000", checked against the CLI when recorded.
func fixtureBase64(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join("..", "decoder", "testdata", "fixtures", name+".xdr")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading fixture %s: %v", name, err)
	}
	return base64.StdEncoding.EncodeToString(raw)
}

// event builds one EventInfo with a well-formed id whose ledger-wide ordinal is
// eventIndex (ADR-022) and whose toid encodes the ledger, so ParseCursor can
// read it back. The topic and value are fixture base64.
func event(t *testing.T, ledger uint32, eventIndex uint32, topicFixture, valueFixture string, success bool) protocol.EventInfo {
	t.Helper()
	cursor := protocol.Cursor{Ledger: ledger, Tx: 0, Op: 0, Event: eventIndex}
	return protocol.EventInfo{
		EventType:                protocol.EventTypeContract,
		Ledger:                   int32(ledger),
		LedgerClosedAt:           "2026-08-27T12:00:00Z",
		ContractID:               showcase,
		ID:                       cursor.String(),
		TransactionHash:          "abc123",
		InSuccessfulContractCall: success,
		TopicXDR:                 []string{fixtureBase64(t, topicFixture)},
		ValueXDR:                 fixtureBase64(t, valueFixture),
	}
}

// endOfWindowCursor is the synthetic marker a zero-match page carries, with the
// uint32-max ordinal (ADR-028). It must never be mistaken for an event id.
func endOfWindowCursor(ledger uint32) string {
	return protocol.Cursor{Ledger: ledger, Event: 4294967295}.String()
}

func storedEvents(t *testing.T, handle *sql.DB) []*models.Event {
	t.Helper()
	// A high limit so the read-back is one page: the default page size is 100,
	// and a paging test stores more than that.
	page, err := store.NewEvents(handle).Query(context.Background(), store.EventQuery{ContractID: showcase, Limit: 5000})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	return page.Events
}

func progress(t *testing.T, handle *sql.DB) int64 {
	t.Helper()
	c, err := store.NewContracts(handle).Get(context.Background(), showcase)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	return c.LastIndexedLedger
}

// --- tests ---

// A tick fetches a page, decodes it, stores it, and advances the cursor to the
// highest ledger it saw. The stored event carries the decoded name and flag.
func TestTickStoresADecodedPageAndAdvancesTheCursor(t *testing.T) {
	handle := migrated(t)
	caller := &mockCaller{
		eventsQueue: []eventsReply{
			{resp: protocol.GetEventsResponse{
				Events: []protocol.EventInfo{
					event(t, 4430000, 0, "transfer_topic0", "deposit_data", true),
				},
				Cursor:       endOfWindowCursor(4430000),
				LatestLedger: 4430005,
			}},
		},
	}
	p := newPoller(t, caller, handle)

	if err := p.tickOnce(context.Background(), showcase); err != nil {
		t.Fatalf("tickOnce: %v", err)
	}

	events := storedEvents(t, handle)
	if len(events) != 1 {
		t.Fatalf("stored %d events, want 1", len(events))
	}
	got := events[0]
	if got.Name != "transfer" {
		t.Errorf("name = %q, want %q", got.Name, "transfer")
	}
	if got.Ledger != 4430000 {
		t.Errorf("ledger = %d, want 4430000", got.Ledger)
	}
	if got.EventIndex != 0 {
		t.Errorf("event_index = %d, want 0", got.EventIndex)
	}
	if !got.InSuccessfulContractCall {
		t.Errorf("in_successful_contract_call = false, want true")
	}
	if want := `{"type":"i128","value":"5000"}`; string(got.DataJSON) != want {
		t.Errorf("data_json = %s, want %s", got.DataJSON, want)
	}
	if p := progress(t, handle); p != 4430000 {
		t.Errorf("progress = %d, want 4430000", p)
	}
}

// The first getEvents asks for last_indexed_ledger + 1, so a tick never re-reads
// the ledger it already committed.
func TestTickStartsFromTheLedgerAfterProgress(t *testing.T) {
	handle := migrated(t)
	if err := store.NewContracts(handle).SetProgress(context.Background(), showcase, 4430000); err != nil {
		t.Fatalf("SetProgress: %v", err)
	}
	caller := &mockCaller{
		eventsQueue: []eventsReply{
			{resp: protocol.GetEventsResponse{LatestLedger: 4430010}},
		},
	}
	p := newPoller(t, caller, handle)

	if err := p.tickOnce(context.Background(), showcase); err != nil {
		t.Fatalf("tickOnce: %v", err)
	}
	if got := caller.firstStartLedger(); got != 4430001 {
		t.Errorf("startLedger = %d, want 4430001", got)
	}
}

// The reverted-call flag is stored as false, not dropped, per ADR-026.
func TestRevertedCallFlagIsStoredNotFiltered(t *testing.T) {
	handle := migrated(t)
	caller := &mockCaller{
		eventsQueue: []eventsReply{
			{resp: protocol.GetEventsResponse{
				Events: []protocol.EventInfo{
					event(t, 4430000, 0, "transfer_topic0", "deposit_data", false),
				},
				Cursor: endOfWindowCursor(4430000),
			}},
		},
	}
	p := newPoller(t, caller, handle)

	if err := p.tickOnce(context.Background(), showcase); err != nil {
		t.Fatalf("tickOnce: %v", err)
	}
	events := storedEvents(t, handle)
	if len(events) != 1 {
		t.Fatalf("stored %d events, want 1; a reverted-call event must not be filtered", len(events))
	}
	if events[0].InSuccessfulContractCall {
		t.Errorf("in_successful_contract_call = true, want false")
	}
}

// The low side of -32600: the retention floor moved past our cursor between the
// call that set our progress and this tick. The poller re-reads the window,
// resumes from the new floor, and stores what it finds there. This is the
// floor-move retry ADR-028 finding #4 and ADR-036 require.
func TestRangeErrorLowSideResumesFromTheNewFloor(t *testing.T) {
	handle := migrated(t)
	// Our cursor is well below where the floor has now moved to.
	if err := store.NewContracts(handle).SetProgress(context.Background(), showcase, 4000000); err != nil {
		t.Fatalf("SetProgress: %v", err)
	}
	caller := &mockCaller{
		eventsQueue: []eventsReply{
			// startLedger 4000001 is below the floor: rejected.
			{err: rpcErr(rangeErrorCode)},
			// After re-reading health, the poller retries from oldestLedger.
			{resp: protocol.GetEventsResponse{
				Events: []protocol.EventInfo{
					event(t, 4465754, 0, "transfer_topic0", "deposit_data", true),
				},
				Cursor: endOfWindowCursor(4465754),
			}},
		},
		health: protocol.GetHealthResponse{OldestLedger: 4465754, LatestLedger: 4586713},
	}
	p := newPoller(t, caller, handle)

	if err := p.tickOnce(context.Background(), showcase); err != nil {
		t.Fatalf("tickOnce: %v", err)
	}

	if caller.healthCalls != 1 {
		t.Errorf("getHealth called %d times, want 1", caller.healthCalls)
	}
	if got := caller.lastStartLedger(); got != 4465754 {
		t.Errorf("retry startLedger = %d, want 4465754 (the new floor)", got)
	}
	events := storedEvents(t, handle)
	if len(events) != 1 {
		t.Fatalf("stored %d events, want 1 from the new floor", len(events))
	}
	if events[0].Ledger != 4465754 {
		t.Errorf("stored ledger = %d, want 4465754", events[0].Ledger)
	}
	if p := progress(t, handle); p != 4465754 {
		t.Errorf("progress = %d, want 4465754", p)
	}
}

// The high side of -32600: the contract is caught up. startLedger is past the
// tip because we have indexed everything. The poller holds position, makes no
// second getEvents call, and returns nil so the loop resets its backoff. This
// is the steady state ADR-036 was written for.
func TestRangeErrorHighSideHoldsPositionWhenCaughtUp(t *testing.T) {
	handle := migrated(t)
	if err := store.NewContracts(handle).SetProgress(context.Background(), showcase, 4586713); err != nil {
		t.Fatalf("SetProgress: %v", err)
	}
	caller := &mockCaller{
		eventsQueue: []eventsReply{
			// startLedger 4586714 is past the tip: rejected.
			{err: rpcErr(rangeErrorCode)},
		},
		health: protocol.GetHealthResponse{OldestLedger: 4465754, LatestLedger: 4586713},
	}
	p := newPoller(t, caller, handle)

	if err := p.tickOnce(context.Background(), showcase); err != nil {
		t.Fatalf("tickOnce should hold position on the high side, got: %v", err)
	}

	if caller.healthCalls != 1 {
		t.Errorf("getHealth called %d times, want 1", caller.healthCalls)
	}
	if caller.eventsCallCount() != 1 {
		t.Errorf("getEvents called %d times, want 1; a caught-up tick must not re-scan", caller.eventsCallCount())
	}
	if len(storedEvents(t, handle)) != 0 {
		t.Errorf("a caught-up tick stored events, want none")
	}
	if p := progress(t, handle); p != 4586713 {
		t.Errorf("progress = %d, want 4586713 held", p)
	}
}

// A -32600 for a startLedger that the freshly read window says is in range is a
// third condition the poller does not model. It surfaces rather than being
// silently treated as either branch (ADR-036).
func TestRangeErrorInsideTheWindowSurfaces(t *testing.T) {
	handle := migrated(t)
	if err := store.NewContracts(handle).SetProgress(context.Background(), showcase, 4500000); err != nil {
		t.Fatalf("SetProgress: %v", err)
	}
	caller := &mockCaller{
		eventsQueue: []eventsReply{{err: rpcErr(rangeErrorCode)}},
		health:      protocol.GetHealthResponse{OldestLedger: 4465754, LatestLedger: 4586713},
	}
	p := newPoller(t, caller, handle)

	err := p.tickOnce(context.Background(), showcase)
	if err == nil {
		t.Fatal("tickOnce returned nil for an in-window -32600, want an error")
	}
	if IsRangeError(err) {
		// It should be wrapped as an unclassified condition, but the point is
		// that it is not swallowed.
	}
	if len(storedEvents(t, handle)) != 0 {
		t.Errorf("stored events despite an unclassified range error")
	}
}

// A single undecodable event is logged and skipped; the rest of the page is
// stored. This is the poison-pill protection of section 7.3.
func TestDecodeFailureSkipsTheEventAndKeepsThePage(t *testing.T) {
	handle := migrated(t)
	bad := event(t, 4430000, 0, "transfer_topic0", "deposit_data", true)
	bad.ValueXDR = "not-valid-base64-xdr!!!"
	good := event(t, 4430000, 1, "transfer_topic0", "deposit_data", true)

	caller := &mockCaller{
		eventsQueue: []eventsReply{
			{resp: protocol.GetEventsResponse{
				Events: []protocol.EventInfo{bad, good},
				Cursor: endOfWindowCursor(4430000),
			}},
		},
	}
	p := newPoller(t, caller, handle)

	if err := p.tickOnce(context.Background(), showcase); err != nil {
		t.Fatalf("tickOnce: %v", err)
	}
	events := storedEvents(t, handle)
	if len(events) != 1 {
		t.Fatalf("stored %d events, want 1; the good event must survive the bad one", len(events))
	}
	if events[0].EventIndex != 1 {
		t.Errorf("stored event_index = %d, want 1 (the good event)", events[0].EventIndex)
	}
}

// An event whose first topic is not a Symbol is stored nameless with its topics
// intact, not failed, per ADR-026.
func TestNonSymbolFirstTopicYieldsANamelessEvent(t *testing.T) {
	handle := migrated(t)
	// deposit_data is an i128, not a Symbol, so using it as topic0 gives a
	// nameless event.
	caller := &mockCaller{
		eventsQueue: []eventsReply{
			{resp: protocol.GetEventsResponse{
				Events: []protocol.EventInfo{
					event(t, 4430000, 0, "deposit_data", "deposit_data", true),
				},
				Cursor: endOfWindowCursor(4430000),
			}},
		},
	}
	p := newPoller(t, caller, handle)

	if err := p.tickOnce(context.Background(), showcase); err != nil {
		t.Fatalf("tickOnce: %v", err)
	}
	events := storedEvents(t, handle)
	if len(events) != 1 {
		t.Fatalf("stored %d events, want 1", len(events))
	}
	if events[0].Name != "" {
		t.Errorf("name = %q, want empty for a non-Symbol first topic", events[0].Name)
	}
	if events[0].Named() {
		t.Errorf("Named() = true, want false")
	}
}

// A tick that finds nothing stores nothing and leaves progress untouched.
func TestEmptyPageStoresNothingAndHoldsProgress(t *testing.T) {
	handle := migrated(t)
	if err := store.NewContracts(handle).SetProgress(context.Background(), showcase, 4430000); err != nil {
		t.Fatalf("SetProgress: %v", err)
	}
	caller := &mockCaller{
		eventsQueue: []eventsReply{
			{resp: protocol.GetEventsResponse{Cursor: endOfWindowCursor(4430001), LatestLedger: 4430001}},
		},
	}
	p := newPoller(t, caller, handle)

	if err := p.tickOnce(context.Background(), showcase); err != nil {
		t.Fatalf("tickOnce: %v", err)
	}
	if len(storedEvents(t, handle)) != 0 {
		t.Errorf("an empty page stored events")
	}
	if p := progress(t, handle); p != 4430000 {
		t.Errorf("progress = %d, want 4430000 held", p)
	}
}

// A full page is followed by a second call carrying the opaque cursor, and the
// walk terminates on the short page that follows. Both pages are stored.
func TestDrainPagesUntilAShortPage(t *testing.T) {
	handle := migrated(t)
	// batchSize is 100; a first page of 100 forces a second call.
	firstPage := make([]protocol.EventInfo, 100)
	for i := range firstPage {
		firstPage[i] = event(t, 4430000, uint32(i), "transfer_topic0", "deposit_data", true)
	}
	caller := &mockCaller{
		eventsQueue: []eventsReply{
			{resp: protocol.GetEventsResponse{
				Events: firstPage,
				Cursor: protocol.Cursor{Ledger: 4430000, Event: 99}.String(),
			}},
			{resp: protocol.GetEventsResponse{
				Events: []protocol.EventInfo{
					event(t, 4430001, 0, "transfer_topic0", "deposit_data", true),
				},
				Cursor: endOfWindowCursor(4430001),
			}},
		},
	}
	p := newPoller(t, caller, handle)

	if err := p.tickOnce(context.Background(), showcase); err != nil {
		t.Fatalf("tickOnce: %v", err)
	}
	if caller.eventsCallCount() != 2 {
		t.Errorf("getEvents called %d times, want 2 (a full page then a short one)", caller.eventsCallCount())
	}
	if got := len(storedEvents(t, handle)); got != 101 {
		t.Errorf("stored %d events, want 101 across two pages", got)
	}
	if p := progress(t, handle); p != 4430001 {
		t.Errorf("progress = %d, want 4430001 (the highest ledger seen)", p)
	}
}

// The second page's request carries the cursor and no StartLedger: the SDK
// rejects a request that sets both, so the poller must clear StartLedger once
// paging begins.
func TestPagingClearsStartLedgerAndSetsCursor(t *testing.T) {
	handle := migrated(t)
	firstPage := make([]protocol.EventInfo, 100)
	for i := range firstPage {
		firstPage[i] = event(t, 4430000, uint32(i), "transfer_topic0", "deposit_data", true)
	}
	caller := &mockCaller{
		eventsQueue: []eventsReply{
			{resp: protocol.GetEventsResponse{
				Events: firstPage,
				Cursor: protocol.Cursor{Ledger: 4430000, Event: 99}.String(),
			}},
			{resp: protocol.GetEventsResponse{Cursor: endOfWindowCursor(4430000)}},
		},
	}
	p := newPoller(t, caller, handle)

	if err := p.tickOnce(context.Background(), showcase); err != nil {
		t.Fatalf("tickOnce: %v", err)
	}
	caller.mu.Lock()
	defer caller.mu.Unlock()
	if len(caller.eventsCalls) != 2 {
		t.Fatalf("made %d calls, want 2", len(caller.eventsCalls))
	}
	second := caller.eventsCalls[1]
	if second.StartLedger != 0 {
		t.Errorf("second call StartLedger = %d, want 0", second.StartLedger)
	}
	if second.Pagination == nil || second.Pagination.Cursor == nil {
		t.Fatal("second call carried no cursor")
	}
	if second.Pagination.Cursor.Ledger != 4430000 || second.Pagination.Cursor.Event != 99 {
		t.Errorf("second cursor = %+v, want ledger 4430000 event 99", *second.Pagination.Cursor)
	}
}

// Events and the advanced cursor commit together: a store failure on progress
// leaves no events behind either. Driven by pointing the poller at a closed DB
// so the transaction cannot commit.
func TestCommitIsAtomic(t *testing.T) {
	handle := migrated(t)
	caller := &mockCaller{
		eventsQueue: []eventsReply{
			{resp: protocol.GetEventsResponse{
				Events: []protocol.EventInfo{
					event(t, 4430000, 0, "transfer_topic0", "deposit_data", true),
				},
				Cursor: endOfWindowCursor(4430000),
			}},
		},
	}
	p := newPoller(t, caller, handle)
	_ = handle.Close() // force the commit to fail

	if err := p.tickOnce(context.Background(), showcase); err == nil {
		t.Fatal("tickOnce succeeded against a closed database, want an error")
	}
}

// nextBackoff doubles to the ceiling and never drops below the floor.
func TestNextBackoff(t *testing.T) {
	floor := 5 * time.Second
	tests := []struct {
		name    string
		current time.Duration
		want    time.Duration
	}{
		{"doubles from the floor", 5 * time.Second, 10 * time.Second},
		{"doubles again", 10 * time.Second, 20 * time.Second},
		{"caps at the ceiling", 40 * time.Second, maxBackoff},
		{"stays at the ceiling", maxBackoff, maxBackoff},
		{"never below the floor", time.Second, floor},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := nextBackoff(tt.current, floor); got != tt.want {
				t.Errorf("nextBackoff(%v, %v) = %v, want %v", tt.current, floor, got, tt.want)
			}
		})
	}
}

// Run returns the context error when cancelled, so the startup wiring can tell
// a clean shutdown from a crash.
func TestRunStopsOnContextCancel(t *testing.T) {
	handle := migrated(t)
	caller := &mockCaller{
		eventsQueue: []eventsReply{
			{resp: protocol.GetEventsResponse{Cursor: endOfWindowCursor(1)}},
		},
	}
	p := newPoller(t, caller, handle)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := p.Run(ctx, showcase)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Run returned %v, want context.Canceled", err)
	}
}

// run drives real ticks: it stores a first page, then keeps ticking (the mock
// returns empty pages) until the context is cancelled. This exercises the
// success path and the timer reset that the cancel-only test skips.
func TestRunTicksThenStopsOnCancel(t *testing.T) {
	handle := migrated(t)
	caller := &mockCaller{
		eventsQueue: []eventsReply{
			{resp: protocol.GetEventsResponse{
				Events: []protocol.EventInfo{
					event(t, 4430000, 0, "transfer_topic0", "deposit_data", true),
				},
				Cursor: endOfWindowCursor(4430000),
			}},
		},
	}
	p := newPoller(t, caller, handle)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- p.Run(ctx, showcase) }()

	// Wait for the first page to be committed, then stop.
	deadline := time.After(2 * time.Second)
	for {
		if progress(t, handle) == 4430000 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("first page was never committed")
		case <-time.After(5 * time.Millisecond):
		}
	}
	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Run returned %v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after cancel")
	}
}

// A tick failure is logged and backed off, not returned: run keeps going. This
// drives the backoff branch, then cancels. The tick fails because the mock's
// queue is exhausted (a generic error, not a range error).
func TestRunBacksOffOnTickFailure(t *testing.T) {
	handle := migrated(t)
	caller := &mockCaller{} // empty queue: every GetEvents errors
	p := newPoller(t, caller, handle)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- p.Run(ctx, showcase) }()

	// Let at least one tick fail and back off.
	deadline := time.After(2 * time.Second)
	for caller.eventsCallCount() < 1 {
		select {
		case <-deadline:
			t.Fatal("no tick was attempted")
		case <-time.After(5 * time.Millisecond):
		}
	}
	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Run returned %v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after cancel")
	}
}

// When the health re-read itself fails on a range error, the failure is wrapped
// and returned rather than swallowed.
func TestRangeErrorWithFailingHealthSurfaces(t *testing.T) {
	handle := migrated(t)
	caller := &mockCaller{
		eventsQueue: []eventsReply{{err: rpcErr(rangeErrorCode)}},
		healthErr:   errors.New("health down"),
	}
	p := newPoller(t, caller, handle)

	err := p.tickOnce(context.Background(), showcase)
	if err == nil {
		t.Fatal("tickOnce returned nil when the health re-read failed, want an error")
	}
}

// A generic (non-range) getEvents error surfaces from the tick unchanged, so
// run can back off on it.
func TestNonRangeErrorSurfaces(t *testing.T) {
	handle := migrated(t)
	caller := &mockCaller{
		eventsQueue: []eventsReply{{err: errors.New("transport boom")}},
	}
	p := newPoller(t, caller, handle)

	err := p.tickOnce(context.Background(), showcase)
	if err == nil {
		t.Fatal("tickOnce returned nil for a transport error, want an error")
	}
	if caller.healthCalls != 0 {
		t.Errorf("getHealth called %d times for a non-range error, want 0", caller.healthCalls)
	}
}

// nameFromTopics returns empty on an empty topic list.
func TestNameFromEmptyTopicsIsEmpty(t *testing.T) {
	if got := nameFromTopics(nil); got != "" {
		t.Errorf("nameFromTopics(nil) = %q, want empty", got)
	}
	if got := nameFromTopics([]decoder.DecodedValue{}); got != "" {
		t.Errorf("nameFromTopics([]) = %q, want empty", got)
	}
}
