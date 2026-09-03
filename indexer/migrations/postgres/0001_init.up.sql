-- Initial schema: the contracts the indexer tracks and the events it has
-- decoded from them.
--
-- This file has a SQLite counterpart that must stay structurally identical to
-- it. The two may differ only in the type and default tokens ADR-029 allows,
-- and a test enforces that. Edit both together.

CREATE TABLE contracts (
    id                   TEXT NOT NULL PRIMARY KEY,
    added_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    first_indexed_ledger INTEGER,
    last_indexed_ledger  INTEGER NOT NULL DEFAULT 0,
    status               TEXT NOT NULL DEFAULT 'active'
);

-- event_index is the event's ordinal within its ledger, not within its
-- transaction. Soroban RPC cannot produce a per-transaction index: a single
-- page returned six events with six different tx_hash values, all carrying
-- transactionIndex 0, with only the ledger-wide ordinal incrementing. So the
-- uniqueness constraint is (ledger, event_index). See ADR-022 and ADR-028.
--
-- topics_json and data_json are JSONB here and TEXT on SQLite. Any query using
-- a Postgres JSON operator such as data_json->>'from' has no SQLite equivalent
-- and must branch per driver, the same way this file does.
--
-- raw_topics is TEXT holding a JSON array on both engines, not TEXT[]. Through
-- database/sql, pgx returns a text[] column as the Postgres array literal
-- string rather than a []string, because stdlib's Rows.Next has no case for
-- that OID and falls through to scanning a string. Nothing queries this column
-- as an array, so the native type bought no capability and cost a divergence
-- the driver could not bridge.
CREATE TABLE events (
    id          BIGSERIAL NOT NULL PRIMARY KEY,
    contract_id TEXT NOT NULL REFERENCES contracts(id) ON DELETE CASCADE,
    ledger      INTEGER NOT NULL,
    tx_hash     TEXT NOT NULL,
    event_index INTEGER NOT NULL,
    name        TEXT NOT NULL,
    topics_json JSONB NOT NULL,
    data_json   JSONB NOT NULL,
    raw_topics  TEXT NOT NULL,
    raw_data    TEXT NOT NULL,
    emitted_at  TIMESTAMPTZ NOT NULL,
    UNIQUE (ledger, event_index)
);
