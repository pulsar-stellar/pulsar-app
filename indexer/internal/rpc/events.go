// Package rpc also holds the polling loop that turns a registered contract into
// stored events. client.go wraps the RPC surface; this file drives it.
//
// The loop is one goroutine per contract, ticking at a fixed interval. Each
// tick drains every event at or after the contract's last indexed ledger,
// decodes it, and commits the batch and the advanced cursor in one transaction
// so a crash can never leave progress ahead of the events it claims. The
// design decisions it encodes are recorded in ADR-022 (event_index is a
// ledger-wide ordinal), ADR-026 (store the reverted-call flag, never filter),
// ADR-028 (the seven live-RPC facts, including the empty-page cursor), ADR-034
// (classify the range error by code), ADR-035 (populate the deprecated flag),
// and ADR-036 (the two -32600 cases, told apart by fresh window bounds).
package rpc

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/pulsar-stellar/pulsar-app/indexer/internal/decoder"
	"github.com/pulsar-stellar/pulsar-app/indexer/internal/models"
	"github.com/pulsar-stellar/pulsar-app/indexer/internal/store"
	protocol "github.com/stellar/go-stellar-sdk/protocols/rpc"
)

// Backoff bounds. The tick interval is the floor and 60s the ceiling, doubling
// on each consecutive RPC failure, per section 7.3. A caught-up high-side
// -32600 is not a failure and resets the backoff (ADR-036).
const (
	maxBackoff      = 60 * time.Second
	backoffexponent = 2
)

// txBeginner is the slice of *sql.DB the poller needs to open the transaction
// that commits a batch and its cursor together. Defined here, where it is used,
// so a test can supply its own without a real database. *sql.DB satisfies it.
type txBeginner interface {
	BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error)
}

// Poller drives one contract's polling loop. It is built once and Run per
// contract; the fields are shared read-only across those goroutines, and the
// Decoder's counter is atomic, so one Poller is safe to Run for many contracts
// at once.
type Poller struct {
	caller    Caller
	decoder   *decoder.Decoder
	db        txBeginner
	contracts *store.Contracts
	interval  time.Duration
	batchSize int
	logger    *slog.Logger
}

// NewPoller builds a Poller over a decoded RPC client, a decoder, and the
// stores. The batch size is bounded by the caller (config rejects a size above
// the RPC page cap, per ADR-028), and the interval is the configured poll
// interval. A nil logger is replaced with a discarding one so the loop never
// panics on a missing dependency.
func NewPoller(
	caller Caller,
	dec *decoder.Decoder,
	database txBeginner,
	contracts *store.Contracts,
	interval time.Duration,
	batchSize int,
	logger *slog.Logger,
) *Poller {
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	return &Poller{
		caller:    caller,
		decoder:   dec,
		db:        database,
		contracts: contracts,
		interval:  interval,
		batchSize: batchSize,
		logger:    logger,
	}
}

// Run polls one contract until ctx is cancelled. It returns ctx.Err() on
// cancellation and never returns nil: the loop is meant to run for the life of
// the process, so a nil return would signal a bug rather than a clean stop.
//
// A tick failure is logged and backed off, not returned. A single bad poll,
// whether a transport error or a range rejection, must not tear down the
// goroutine and stop indexing a contract for good.
func (p *Poller) run(ctx context.Context, contractID string) error {
	backoff := p.interval
	timer := time.NewTimer(p.interval)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
			wait := p.interval
			if err := p.tickOnce(ctx, contractID); err != nil {
				if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
					return err
				}
				p.logger.Error("poll_tick_failed",
					"component", "rpc",
					"contract_id", contractID,
					"err", err)
				wait = backoff
				backoff = nextBackoff(backoff, p.interval)
			} else {
				backoff = p.interval
			}
			timer.Reset(wait)
		}
	}
}

// Run is the exported entry point, one call per contract. It exists so the
// startup wiring in main.go has a stable name to launch in a goroutine, while
// run holds the loop the tests drive directly.
func (p *Poller) Run(ctx context.Context, contractID string) error {
	return p.run(ctx, contractID)
}

// nextBackoff doubles the current wait, capped at maxBackoff, and never drops
// below the floor.
func nextBackoff(current, floor time.Duration) time.Duration {
	next := current * backoffexponent
	if next > maxBackoff {
		return maxBackoff
	}
	if next < floor {
		return floor
	}
	return next
}

// tickOnce drains one contract from its stored cursor to the tip and commits
// what it read. A range rejection is classified and handled here rather than
// bubbling up as a generic failure, so the backoff in run only ever sees a
// genuine fault.
func (p *Poller) tickOnce(ctx context.Context, contractID string) error {
	contract, err := p.contracts.Get(ctx, contractID)
	if err != nil {
		return fmt.Errorf("rpc: reading progress for %s: %w", contractID, err)
	}

	startLedger := contract.LastIndexedLedger + 1
	if startLedger < 1 {
		startLedger = 1
	}

	events, highestLedger, err := p.drain(ctx, contractID, uint32(startLedger))
	if err != nil {
		if IsRangeError(err) {
			return p.handleRangeError(ctx, contractID, uint32(startLedger), err)
		}
		return err
	}

	if len(events) == 0 {
		return nil
	}

	return p.commit(ctx, contractID, events, highestLedger)
}

// drain pages getEvents from startLedger to the end of the window, decoding as
// it goes. It returns the assembled events and the highest ledger seen, which
// becomes the new cursor. Paging uses the opaque response cursor and terminates
// on an empty events array, never on the cursor, which is present even on a
// zero-match page (ADR-028).
func (p *Poller) drain(
	ctx context.Context,
	contractID string,
	startLedger uint32,
) ([]*models.Event, int64, error) {
	req := protocol.GetEventsRequest{
		StartLedger: startLedger,
		Filters:     []protocol.EventFilter{{ContractIDs: []string{contractID}}},
		Pagination:  &protocol.PaginationOptions{Limit: uint(p.batchSize)},
	}

	var (
		assembled     []*models.Event
		highestLedger int64
	)

	for {
		resp, err := p.caller.GetEvents(ctx, req)
		if err != nil {
			return nil, 0, err
		}

		for i := range resp.Events {
			info := &resp.Events[i]
			event, err := p.assemble(contractID, info)
			if err != nil {
				// Poison-pill protection (section 7.3): a value this decoder
				// cannot read must not stall the contract forever. Log the
				// coordinates and skip the one event, keeping the rest of the
				// page.
				p.logger.Error("event_decode_failed",
					"component", "rpc",
					"contract_id", contractID,
					"tx_hash", info.TransactionHash,
					"event_id", info.ID,
					"err", err)
				continue
			}
			if event.Ledger > highestLedger {
				highestLedger = event.Ledger
			}
			assembled = append(assembled, event)
		}

		// An empty page is the end of the window. The cursor is not a
		// has-more signal: it is populated even here (ADR-028), so the length
		// of events is what terminates the walk.
		if len(resp.Events) == 0 {
			break
		}

		next, err := protocol.ParseCursor(resp.Cursor)
		if err != nil {
			return nil, 0, fmt.Errorf("rpc: parsing page cursor %q for %s: %w", resp.Cursor, contractID, err)
		}

		// A cursor and a ledger range cannot both be set (the SDK rejects it),
		// so once paging begins the request carries only the cursor.
		req.StartLedger = 0
		cursor := next
		req.Pagination = &protocol.PaginationOptions{
			Cursor: &cursor,
			Limit:  uint(p.batchSize),
		}

		// A short page means the window is exhausted: fewer events came back
		// than the limit could have carried, so there is no further page.
		if len(resp.Events) < p.batchSize {
			break
		}
	}

	return assembled, highestLedger, nil
}

// handleRangeError resolves a -32600 by re-reading the current window and
// classifying the rejection against fresh bounds, never against the message
// text (ADR-036). The low side is a backfill gap that resumes from the new
// floor; the high side is a caught-up contract that holds position.
func (p *Poller) handleRangeError(
	ctx context.Context,
	contractID string,
	startLedger uint32,
	rangeErr error,
) error {
	health, err := p.caller.GetHealth(ctx)
	if err != nil {
		return fmt.Errorf("rpc: re-reading window after a range error for %s: %w", contractID, err)
	}

	switch {
	case startLedger > health.LatestLedger:
		// High side: caught up. No events past the tip yet. Hold position and
		// wait for the next tick; run resets the backoff on this nil return.
		p.logger.Debug("poll_caught_up",
			"component", "rpc",
			"contract_id", contractID,
			"start_ledger", startLedger,
			"latest_ledger", health.LatestLedger)
		return nil

	case startLedger < health.OldestLedger:
		// Low side: the retention floor moved past our cursor. The ledgers
		// between the old cursor and the new floor are gone from RPC; record
		// the gap and resume from the floor (ADR-028 finding #4).
		p.logger.Warn("poll_skipped_ledgers",
			"component", "rpc",
			"contract_id", contractID,
			"skipped_from", startLedger,
			"skipped_to", health.OldestLedger-1,
			"resumed_at", health.OldestLedger)

		events, highestLedger, err := p.drain(ctx, contractID, health.OldestLedger)
		if err != nil {
			return fmt.Errorf("rpc: draining %s from the new floor: %w", contractID, err)
		}
		if len(events) == 0 {
			return nil
		}
		return p.commit(ctx, contractID, events, highestLedger)

	default:
		// startLedger is inside [oldestLedger, latestLedger], yet the call was
		// rejected with -32600. That is a third condition this code does not
		// model, so it surfaces rather than being mistaken for either branch.
		return fmt.Errorf(
			"rpc: %s rejected startLedger %d as out of range, but it lies within the reported window %d-%d: %w",
			contractID, startLedger, health.OldestLedger, health.LatestLedger, rangeErr)
	}
}

// commit writes a batch and advances the cursor in one transaction, so the
// stored progress can never run ahead of the events it accounts for.
func (p *Poller) commit(
	ctx context.Context,
	contractID string,
	events []*models.Event,
	highestLedger int64,
) error {
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("rpc: beginning the commit for %s: %w", contractID, err)
	}
	defer func() { _ = tx.Rollback() }()

	inserted, err := store.NewEvents(tx).Insert(ctx, events)
	if err != nil {
		return fmt.Errorf("rpc: inserting %d events for %s: %w", len(events), contractID, err)
	}

	if err := store.NewContracts(tx).SetProgress(ctx, contractID, highestLedger); err != nil {
		return fmt.Errorf("rpc: advancing progress for %s to %d: %w", contractID, highestLedger, err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("rpc: committing %d events for %s: %w", len(events), contractID, err)
	}

	p.logger.Info("events_indexed",
		"component", "rpc",
		"contract_id", contractID,
		"fetched", len(events),
		"inserted", inserted,
		"ledger", highestLedger,
		"unknown_total", p.decoder.UnknownCount())
	return nil
}

// assemble turns one RPC EventInfo into a models.Event, decoding its topics and
// data. It is the one place the RPC wire shape meets the storage shape, and
// every field the store requires is filled here from the event or its decode.
//
// A decode error is returned rather than swallowed: drain logs it with the
// event's coordinates and skips the one event. The name is derived per ADR-026:
// the first topic iff it decodes to a Symbol, otherwise empty, which is a
// nameless event with its topics intact rather than a failure.
func (p *Poller) assemble(contractID string, info *protocol.EventInfo) (*models.Event, error) {
	eventIndex, err := ParseEventID(info.ID)
	if err != nil {
		return nil, err
	}

	emittedAt, err := ParseLedgerCloseTime(info.LedgerClosedAt)
	if err != nil {
		return nil, err
	}

	topics, err := p.decoder.DecodeTopics(info.TopicXDR)
	if err != nil {
		return nil, fmt.Errorf("decoding topics for event %s: %w", info.ID, err)
	}

	data, err := p.decoder.DecodeBase64(info.ValueXDR)
	if err != nil {
		return nil, fmt.Errorf("decoding data for event %s: %w", info.ID, err)
	}

	topicsJSON, err := marshalTopics(topics)
	if err != nil {
		return nil, fmt.Errorf("encoding decoded topics for event %s: %w", info.ID, err)
	}

	dataJSON, err := data.MarshalJSON()
	if err != nil {
		return nil, fmt.Errorf("encoding decoded data for event %s: %w", info.ID, err)
	}

	return &models.Event{
		ContractID:               contractID,
		Ledger:                   int64(info.Ledger),
		TxHash:                   info.TransactionHash,
		EventIndex:               eventIndex,
		Name:                     nameFromTopics(topics),
		TopicsJSON:               topicsJSON,
		DataJSON:                 dataJSON,
		RawTopics:                info.TopicXDR,
		RawData:                  info.ValueXDR,
		EmittedAt:                emittedAt,
		InSuccessfulContractCall: info.InSuccessfulContractCall,
	}, nil
}

// marshalTopics renders the decoded topic list as the JSON array the store
// holds in topics_json. An empty topic list marshals to [], never to null, so
// the column always carries a JSON array (jsonOrNull in the store guards the
// null case, but emitting [] here keeps the stored shape a plain array).
func marshalTopics(topics []decoder.DecodedValue) (json.RawMessage, error) {
	if topics == nil {
		topics = []decoder.DecodedValue{}
	}
	encoded, err := json.Marshal(topics)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(encoded), nil
}

// nameFromTopics derives the event name from its first topic, per ADR-026: the
// name is that topic's value iff it decoded to a Symbol, and otherwise empty.
// An off-convention event is nameless with its topics intact, not a failure.
func nameFromTopics(topics []decoder.DecodedValue) string {
	if len(topics) == 0 {
		return ""
	}
	first := topics[0]
	if first.Type != decoder.TypeSymbol {
		return ""
	}
	name, ok := first.Value.(string)
	if !ok {
		return ""
	}
	return name
}
