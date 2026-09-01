-- Indexes for the read paths section 7.2 exposes: events by contract in
-- ledger order, by event name, and by recency.
--
-- This file has a Postgres counterpart that must stay structurally identical to
-- it. The two may differ only in the tokens ADR-029 allows, and a test
-- enforces that. Edit both together.
--
-- The topics index is the one place the two engines differ in capability
-- rather than only in spelling. Postgres uses GIN here, which this index cannot match for containment
-- queries, so a topic_contains filter is an indexed lookup on Postgres and a
-- json_each scan on SQLite. The query layer branches per driver accordingly.

CREATE INDEX idx_events_contract_ledger ON events (contract_id, ledger DESC);
CREATE INDEX idx_events_name ON events (name);
CREATE INDEX idx_events_emitted_at ON events (emitted_at DESC);
CREATE INDEX idx_events_topics ON events (topics_json);
