// Package models holds the Go representations of the indexer's database rows
// and the shapes they take on the wire.
package models

import "time"

// Status is the indexer's tracking state for a contract.
//
// The values match the SDK's ContractStatusSchema exactly, which is what the
// explorer and any other consumer validate against. A value outside this set
// is rejected at the client, so the two lists have to stay in step.
type Status string

const (
	// StatusActive means the indexer is following the contract.
	StatusActive Status = "active"
	// StatusPaused means tracking is suspended and no polling happens.
	StatusPaused Status = "paused"
	// StatusError means the last poll failed and the contract needs attention.
	StatusError Status = "error"
)

// Contract is one row of the contracts table.
//
// The identifier is the Stellar contract strkey, a 56 character string, not a
// surrogate key. ADR-021's rule that identifiers are serialized as strings
// because they can outgrow a JSON number applies to events.id, which is a
// BIGSERIAL; this column is TEXT and is already a string on both sides, so it
// carries no ,string tag. Adding one would double-encode it.
//
// FirstIndexedLedger is a pointer because the column is nullable and null
// means something: the contract is registered but its first poll has not
// completed. A zero there would read as "indexed up to ledger 0", which is a
// different and wrong claim. LastIndexedLedger is NOT NULL DEFAULT 0 in the
// schema, so it is a plain int64; a pointer would model a state the database
// cannot produce.
type Contract struct {
	ID                 string    `db:"id" json:"id"`
	AddedAt            time.Time `db:"added_at" json:"added_at"`
	FirstIndexedLedger *int64    `db:"first_indexed_ledger" json:"first_indexed_ledger"`
	LastIndexedLedger  int64     `db:"last_indexed_ledger" json:"last_indexed_ledger"`
	Status             Status    `db:"status" json:"status"`
}

// Indexed reports whether the contract's first poll has completed.
func (c Contract) Indexed() bool { return c.FirstIndexedLedger != nil }
