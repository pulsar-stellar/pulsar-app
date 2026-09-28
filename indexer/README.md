# Pulsar Indexer

The Go daemon of the [Pulsar Stellar](https://github.com/pulsar-stellar/pulsar-app)
toolkit. It polls Soroban RPC, decodes each contract event, stores it past the
seven-day RPC retention window, and serves that history over an HTTP read API.
The TypeScript SDK and the web explorer both read from this API.

It treats every byte a contract emits as hostile input and validates before
insert. It is a separate Go module rather than a workspace member, because the
language boundary is a natural seam (ADR-002).

## How it runs

Startup is a fixed pipeline, each stage gated on the last: load and validate the
environment, build the logger, open and migrate the database, register the
bootstrap contracts, then launch one polling goroutine per contract and serve the
HTTP surface alongside them. A failure in any stage before the loops start is
fatal and names the stage. SIGINT or SIGTERM cancels the root context, the HTTP
server drains in flight requests, every poll loop returns, and the process waits
for all of them before exiting.

```
config -> logger -> db.Open -> migrations -> bootstrap register
                                                    |
                        +---------------------------+---------------------------+
                        |                                                       |
              one poll loop per contract                             HTTP surface on LISTEN_ADDR
              (RPC -> decode -> store)                               (REST + GraphQL, reads open,
                                                                      writes gated)
```

## Quick start

```sh
cd indexer
cp ../.env.example ../.env.local     # then fill in PULSAR_INDEXER_ADMIN_TOKEN
go build ./...
PULSAR_INDEXER_ADMIN_TOKEN=$(openssl rand -hex 32) \
PULSAR_INDEXER_DB_URL=file:./pulsar.db \
PULSAR_INDEXER_RPC_URL=https://soroban-testnet.stellar.org \
PULSAR_INDEXER_NETWORK=testnet \
PULSAR_INDEXER_BOOTSTRAP_CONTRACTS=CDNWTVUDKCCGW7GOC6SBLUFXXUCD2YDHWRDUSXZ6CYBQKQWLCUYYWI5L \
go run ./cmd/pulsar-indexer
```

The daemon defaults to SQLite, so it needs no database to stand up. It begins
tracking the bootstrap contract, and within one poll interval the events table
starts filling. The read API is then reachable on `:8080`.

## Configuration

Every value comes from the environment and is validated once, at startup. A
missing required variable or an unusable value is a startup failure that names
the variable, never a default quietly substituted for what the operator meant.
Every variable also appears in [`.env.example`](../.env.example) with a
publishable default.

| Variable | Required | Default | Meaning |
|---|---|---|---|
| `PULSAR_INDEXER_DB_URL` | yes | none | Database DSN. `file:./pulsar.db` for SQLite, a `postgres://` URL otherwise. |
| `PULSAR_INDEXER_RPC_URL` | yes | none | Soroban RPC endpoint. Must be http or https. |
| `PULSAR_INDEXER_NETWORK` | yes | none | One of `testnet`, `futurenet`, `mainnet`, `local`. |
| `PULSAR_INDEXER_ADMIN_TOKEN` | yes | none | Bearer token guarding the write routes. At least 16 characters. |
| `PULSAR_INDEXER_LISTEN_ADDR` | no | `:8080` | Address the HTTP surface binds. |
| `PULSAR_INDEXER_DB_DRIVER` | no | `sqlite` | `sqlite` or `postgres`. |
| `PULSAR_INDEXER_DB_POOL_MAX` | no | driver default | Max open connections. SQLite permits only 1. |
| `PULSAR_INDEXER_DB_POOL_MIN` | no | driver default | Min idle connections. |
| `PULSAR_INDEXER_DB_ALLOW_INSECURE_TLS` | no | `false` | Permit a Postgres DSN that can fall back to plaintext. Local use only (ADR-031). |
| `PULSAR_INDEXER_POLL_INTERVAL_SEC` | no | `5` | Seconds between RPC polls per contract. |
| `PULSAR_INDEXER_BATCH_SIZE` | no | `100` | Events fetched per RPC page, capped at 10000 (ADR-028). |
| `PULSAR_INDEXER_BOOTSTRAP_CONTRACTS` | no | empty | Comma-separated contract IDs tracked on first run. |
| `PULSAR_INDEXER_WRITE_RATE_PER_SEC` | no | `5` | Sustained requests per second allowed on the write surface. |
| `PULSAR_INDEXER_WRITE_RATE_BURST` | no | `10` | Burst the write limiter tolerates above the sustained rate. |
| `PULSAR_INDEXER_LOG_LEVEL` | no | `info` | `debug`, `info`, `warn`, or `error`. |
| `PULSAR_INDEXER_LOG_FORMAT` | no | `json` | `json` or `text`. |
| `PULSAR_INDEXER_READ_TIMEOUT_SEC` | no | `15` | Wall-clock ceiling on the time any request may spend downstream. |

## Storage: SQLite versus Postgres

The same schema runs on both engines, but each carries its own migration set
rather than one shared file. SQLite accepts several Postgres declarations and
then behaves differently, so a single file would apply cleanly on both and
silently corrupt one (ADR-029). The store also carries its SQL dialect so the one
query the engines spell differently is written correctly for each (ADR-041).

- **SQLite** is the default. Point `PULSAR_INDEXER_DB_URL` at a file
  (`file:./pulsar.db`), keep the pool at 1 (SQLite permits one writer at a time),
  and the daemon needs nothing else to stand up. This is the local development
  and single-node path.
- **Postgres** is the production path. Set `PULSAR_INDEXER_DB_DRIVER=postgres` and
  a `postgres://` DSN. The DSN's `sslmode` must be `require`, `verify-ca`, or
  `verify-full`. Omitting it means `sslmode=prefer`, which silently falls back to
  an unencrypted connection carrying the password, so the indexer refuses to start
  on it unless `PULSAR_INDEXER_DB_ALLOW_INSECURE_TLS=true` is set for local use
  (ADR-031).

## HTTP surface

Every JSON response, success or failure, is the same envelope (ADR-017): a
success carries `data` with an optional `next_cursor` and `meta.took_ms`
alongside it; a failure carries `error.code` (the wire class) and
`error.details.code` (the specific catalog code). Every response also carries
`X-Content-Type-Options: nosniff`.

### REST routes

| Method | Path | Auth | Purpose |
|---|---|---|---|
| `GET` | `/health` | open | Liveness plus `latest_ledger` and `tracked_contracts`. |
| `GET` | `/contracts` | open | List every tracked contract. Unpaginated: the list grows only by operator action. |
| `POST` | `/contracts` | required | Register a contract for indexing. Idempotent on identical input (ADR-018). |
| `GET` | `/contracts/{id}` | open | One contract's indexing metadata. |
| `DELETE` | `/contracts/{id}` | required | Stop tracking a contract. Cascades to every event under it (`ON DELETE CASCADE`). |
| `GET` | `/contracts/{id}/events` | open | That contract's events, newest first, paginated. |
| `GET` | `/events/{id}` | open | A single event by its id. |
| `POST` | `/graphql` | open | Read-only GraphQL over the same data (ADR-043). |

The events list accepts `limit` (1 to 500, default 50), `cursor`, `order`
(`asc` or `desc`, default `desc`), `from_ledger`, `to_ledger`, `name`, and
`topic_contains`. Absence of a tracked contract is a 404 with a `not_found`
envelope, distinct from a tracked contract with no matching events, which is an
empty page (ADR-021).

### GraphQL

`POST /graphql` serves a read-only schema over the same stores, with
`Contract.events` nesting the REST surface cannot express. It carries built-in
depth and query-length limits and bounded parallelism as its DoS guards, and
introspection stays on because the schema is public (ADR-043). The root:

```graphql
type Query {
  health: Health!
  contracts: [Contract!]!
  contract(id: ID!): Contract
  event(id: ID!): Event
  events(contractId: ID!, name: String, fromLedger: Int, toLedger: Int,
         topicContains: String, limit: Int, cursor: String, order: String): EventConnection!
}
```

Errors follow the GraphQL `{ data, errors }` envelope; each error carries the
same catalog code in `errors[].extensions.code`. An event `id` travels as a
string, because a `BIGSERIAL` exceeds a JSON number's safe range (ADR-021).

## Authentication

The two write routes (`POST /contracts` and `DELETE /contracts/{id}`) require a
static bearer token; every read route is public (ADR-044).

```sh
curl -X POST http://localhost:8080/contracts \
  -H "Authorization: Bearer $PULSAR_INDEXER_ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"id":"CDNWTVUDKCCGW7GOC6SBLUFXXUCD2YDHWRDUSXZ6CYBQKQWLCUYYWI5L"}'
```

- The token comes from `PULSAR_INDEXER_ADMIN_TOKEN`, is required with no default,
  and must be at least 16 characters. The daemon refuses to start otherwise, so a
  deployment cannot expose open writes by omission. Generate one with
  `openssl rand -hex 32`.
- The presented and configured tokens are each hashed with SHA-256 and compared
  in constant time, so the comparison leaks neither length nor content. An empty
  or mismatched token fails closed.
- A missing or wrong token returns `401` with the `unauthorized` wire class. A
  caller over the write rate limit returns `429` with the `RATE_LIMITED` code;
  the limiter sits outside the auth check, so an unauthenticated flood is capped
  before the comparison runs.

`DELETE` is the sharp edge: it cascades to every indexed event under the
contract, so one authenticated delete removes the contract and its whole event
history. That is why the write gate is a precondition of exposing the surface,
not a later addition.

## Development

```sh
cd indexer
go vet ./...
go test -race ./...
go build ./...
```

Tests run against SQLite and use fakes for the store, so they need no database or
network. See [`../CONTRIBUTING.md`](../CONTRIBUTING.md) for the commit and review
discipline, and [`.agent/decisions.md`](../.agent/decisions.md) for the reasoning
behind the choices above.

## License

Apache-2.0. See [`../LICENSE`](../LICENSE).

