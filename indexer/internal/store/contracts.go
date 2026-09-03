package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/pulsar-stellar/pulsar-app/indexer/internal/models"
)

// Contracts reads and writes the contracts table.
//
// Nothing here branches per driver. The one divergence this table has is how a
// timestamp comes back, which scanTime absorbs, so the SQL below is valid on
// both engines unchanged.
type Contracts struct {
	q Querier
}

// NewContracts builds a store over a database handle or a transaction.
func NewContracts(q Querier) *Contracts { return &Contracts{q: q} }

const contractColumns = `id, added_at, first_indexed_ledger, last_indexed_ledger, status`

// Register starts tracking a contract and returns its record.
//
// Registration is idempotent on identical input, per ADR-018: registering an
// already-tracked contract succeeds and returns the existing row with its
// indexing progress untouched, rather than failing or resetting it.
//
// The insert and the read are two statements because ON CONFLICT DO NOTHING
// combined with RETURNING yields no rows precisely when the conflict fires,
// on both engines, so the returning form cannot report the existing record.
func (c *Contracts) Register(ctx context.Context, id string) (models.Contract, error) {
	if id == "" {
		return models.Contract{}, errors.New("store: contract id is empty")
	}

	if _, err := c.q.ExecContext(ctx,
		`INSERT INTO contracts (id) VALUES ($1) ON CONFLICT (id) DO NOTHING`, id); err != nil {
		return models.Contract{}, fmt.Errorf("store: registering contract: %w", err)
	}

	contract, err := c.Get(ctx, id)
	if err != nil {
		return models.Contract{}, fmt.Errorf("store: reading back the registered contract: %w", err)
	}
	return contract, nil
}

// Get returns one contract, or ErrNotFound.
func (c *Contracts) Get(ctx context.Context, id string) (models.Contract, error) {
	row := c.q.QueryRowContext(ctx,
		`SELECT `+contractColumns+` FROM contracts WHERE id = $1`, id)

	contract, err := scanContract(row)
	if errors.Is(err, sql.ErrNoRows) {
		return models.Contract{}, fmt.Errorf("store: contract %s: %w", id, ErrNotFound)
	}
	if err != nil {
		return models.Contract{}, fmt.Errorf("store: reading contract %s: %w", id, err)
	}
	return contract, nil
}

// List returns every tracked contract, oldest registration first, so the order
// is stable across calls rather than whatever the engine happens to return.
func (c *Contracts) List(ctx context.Context) ([]models.Contract, error) {
	rows, err := c.q.QueryContext(ctx,
		`SELECT `+contractColumns+` FROM contracts ORDER BY added_at, id`)
	if err != nil {
		return nil, fmt.Errorf("store: listing contracts: %w", err)
	}
	defer func() { _ = rows.Close() }()

	contracts := []models.Contract{}
	for rows.Next() {
		contract, err := scanContract(rows)
		if err != nil {
			return nil, fmt.Errorf("store: listing contracts: %w", err)
		}
		contracts = append(contracts, contract)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: listing contracts: %w", err)
	}
	return contracts, nil
}

// Delete stops tracking a contract and removes its events, which the schema's
// ON DELETE CASCADE handles. It returns ErrNotFound if there was nothing to
// delete, so a caller can tell 204 from 404.
func (c *Contracts) Delete(ctx context.Context, id string) error {
	result, err := c.q.ExecContext(ctx, `DELETE FROM contracts WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("store: deleting contract %s: %w", id, err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: deleting contract %s: %w", id, err)
	}
	if affected == 0 {
		return fmt.Errorf("store: contract %s: %w", id, ErrNotFound)
	}
	return nil
}

// SetProgress records how far the indexer has read for a contract.
//
// first_indexed_ledger is written only when it is still null, so it keeps
// meaning "the ledger the first completed poll reached" rather than drifting
// forward with every update.
func (c *Contracts) SetProgress(ctx context.Context, id string, ledger int64) error {
	result, err := c.q.ExecContext(ctx,
		`UPDATE contracts
		    SET last_indexed_ledger = $1,
		        first_indexed_ledger = COALESCE(first_indexed_ledger, $2)
		  WHERE id = $3`, ledger, ledger, id)
	if err != nil {
		return fmt.Errorf("store: updating progress for %s: %w", id, err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: updating progress for %s: %w", id, err)
	}
	if affected == 0 {
		return fmt.Errorf("store: contract %s: %w", id, ErrNotFound)
	}
	return nil
}

// SetStatus moves a contract between tracking states.
func (c *Contracts) SetStatus(ctx context.Context, id string, status models.Status) error {
	result, err := c.q.ExecContext(ctx,
		`UPDATE contracts SET status = $1 WHERE id = $2`, string(status), id)
	if err != nil {
		return fmt.Errorf("store: updating status for %s: %w", id, err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: updating status for %s: %w", id, err)
	}
	if affected == 0 {
		return fmt.Errorf("store: contract %s: %w", id, ErrNotFound)
	}
	return nil
}

// scanner is what *sql.Row and *sql.Rows have in common.
type scanner interface {
	Scan(dest ...any) error
}

func scanContract(s scanner) (models.Contract, error) {
	var (
		contract models.Contract
		addedAt  any
		first    sql.NullInt64
		status   string
	)

	if err := s.Scan(&contract.ID, &addedAt, &first, &contract.LastIndexedLedger, &status); err != nil {
		return models.Contract{}, err
	}

	parsed, err := scanTime(addedAt)
	if err != nil {
		return models.Contract{}, err
	}
	contract.AddedAt = parsed

	if first.Valid {
		value := first.Int64
		contract.FirstIndexedLedger = &value
	}
	contract.Status = models.Status(status)

	return contract, nil
}
