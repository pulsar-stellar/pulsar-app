package models

import (
	"encoding/json"
	"time"
)

// Event is one row of the events table.
//
// The identifier is an int64 serialized as a JSON string. This is the case
// ADR-021 was written for: the column is BIGSERIAL, whose range exceeds what a
// JSON number carries without losing precision past 2^53, so the wire value is
// a string of digits and the SDK's EventIdSchema validates it as such. Go has
// a natural int64 and does its own arithmetic on the number, so the conversion
// happens only at the wire boundary. Contrast Contract.ID, which is a TEXT
// strkey and carries no ,string tag because it is already a string.
//
// Ledger and EventIndex stay plain JSON numbers. A Stellar ledger sequence is
// protocol-bounded to uint32 and the event ordinal is bounded within a ledger,
// so neither approaches 2^53 and neither needs the string treatment.
//
// TopicsJSON and DataJSON hold decoded values in the ADR-023 taxonomy, kept as
// raw JSON so this layer neither re-encodes nor validates them. json.RawMessage
// rather than []byte is load-bearing: []byte marshals to base64, which would
// put a base64 blob on the wire where the SDK expects an object.
//
// RawTopics is the one field whose db tag does not describe a direct scan. The
// column is TEXT[] on Postgres and TEXT holding a JSON array on SQLite, per
// ADR-029, so the store converts per driver. The tag names the column this
// field corresponds to; it does not promise that database/sql can fill it
// unaided on both engines.
//
// Every column in the events table is NOT NULL, so no field is a pointer.
// Contract needed one for first_indexed_ledger; this table has no nullable
// column at all.
type Event struct {
	ID                       int64           `db:"id" json:"id,string"`
	ContractID               string          `db:"contract_id" json:"contract_id"`
	Ledger                   int64           `db:"ledger" json:"ledger"`
	TxHash                   string          `db:"tx_hash" json:"tx_hash"`
	EventIndex               int64           `db:"event_index" json:"event_index"`
	Name                     string          `db:"name" json:"name"`
	TopicsJSON               json.RawMessage `db:"topics_json" json:"topics_json"`
	DataJSON                 json.RawMessage `db:"data_json" json:"data_json"`
	RawTopics                []string        `db:"raw_topics" json:"raw_topics"`
	RawData                  string          `db:"raw_data" json:"raw_data"`
	EmittedAt                time.Time       `db:"emitted_at" json:"emitted_at"`
	InSuccessfulContractCall bool            `db:"in_successful_contract_call" json:"in_successful_contract_call"`
}

// Named reports whether the event's first topic was a Symbol the decoder could
// read as a name.
//
// An empty name is not an error. Per ADR-026 an emitter that does not follow
// Soroban's naming convention produces a nameless event with its topics
// intact, rather than failing the page it appears in.
func (e Event) Named() bool { return e.Name != "" }
