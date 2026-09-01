# Context: pulsar-app

Onboarding for a fresh session, AI or human. Read this before touching code. Updated at phase transitions, not at every commit.

## 1. What Pulsar Stellar is

Pulsar Stellar is a developer toolkit for Soroban contract events. Every Stellar project that needs to consume its contract's events today writes the same plumbing from scratch: XDR decoders, indexer glue, custom APIs. Pulsar Stellar provides three shared building blocks so they don't have to. A Rust library that turns raw contract events into typed data, a Go daemon that stores historical events past the seven-day RPC retention window, and a web explorer where anyone can paste a contract ID and browse every event that contract has ever emitted, decoded and searchable. It serves Soroban dapp builders, backend engineers integrating with existing protocols, and auditors reviewing contract behavior post-deployment.

That paragraph is locked wording. It appears verbatim in the README and in the docs introduction. Do not paraphrase it.

## 2. Why this repo exists

`pulsar-core` proves the events can be decoded. `pulsar-app` is what makes that useful to somebody who is not writing Rust: a client library they install, a daemon they run or call, and a site they can send a colleague to. Everything a user of the toolkit actually touches lives here.

## 3. Relationship to pulsar-core

`pulsar-stellar/pulsar-core` is the Rust contract layer. Its `pulsar-showcase` contract is the fixture the whole toolkit is calibrated against: every function on it exists to emit a specific event shape that the decoders here are tested against.

The import point is the `v0.1.0-contracts` release, deployed to Stellar testnet:

| Field | Value |
|---|---|
| Contract ID | `CDNWTVUDKCCGW7GOC6SBLUFXXUCD2YDHWRDUSXZ6CYBQKQWLCUYYWI5L` |
| Network | testnet |
| Release | `v0.1.0-contracts` |

That ID is the default in `.env.example` for both `NEXT_PUBLIC_SHOWCASE_CONTRACT_ID` and `PULSAR_INDEXER_BOOTSTRAP_CONTRACTS`. If a decoder here disagrees with what the contract emits, the contract is right and the decoder is wrong.

`pulsar-decoder`, the Rust decoder crate, is a placeholder until `v0.2.0-contracts`. Nothing here depends on it yet.

## 4. The three sub-stacks

**`packages/sdk`**, TypeScript, published as `@pulsar-stellar/sdk`. Wraps the indexer's HTTP API in a typed client, and offers a direct-RPC path for callers who want live events without running an indexer. Validates every response with Zod before returning it. Throws typed `PulsarError` subclasses, never raw errors. `@stellar/stellar-sdk` is a peer dependency, not a bundled one.

**`indexer/`**, Go, its own module. Polls Soroban RPC on an interval, decodes the events it finds, and writes them to SQLite locally or Postgres in production. Serves the read API that the SDK and the explorer both consume. It treats every byte from a contract as hostile input and validates before insert. Separate module rather than a workspace member because the language boundary is a natural seam, recorded in ADR-002.

### Why events paginate and contracts do not

`GET /contracts/:id/events` takes `limit` and `cursor`. `GET /contracts` takes nothing and returns the whole list. That asymmetry is deliberate, and it follows from how the two sets grow.

Tracked contracts are bounded by operator attention: each one exists because somebody deliberately registered it, so the list grows slowly, in human-sized steps, and has a natural ceiling. Events are bounded only by protocol activity: a busy contract emits them continuously, nobody decides one at a time, and there is no ceiling short of the ledger's own history.

So the API shapes follow the growth patterns rather than being made uniform for tidiness. Paginating the contract list would add a cursor round trip to every caller for a list that fits in one response. Not paginating events would work in testing and fail in production against a contract with real traffic.

The general rule for any endpoint added later: paginate what grows without anyone deciding to grow it.

**`apps/web`**, Next.js 15 with the App Router. The public explorer. Server Components by default, TanStack Query for client-side paging against the indexer, Tailwind and shadcn/ui for everything visual.

## 5. Documentation

Documentation is not in this repo. It lives in `pulsar-stellar/pulsar-docs` and publishes through GitBook's GitHub Sync from that repo's `main`. See ADR-009. A change here that alters public API is not done until the corresponding change is open against `pulsar-docs`.

## 6. Current phase and definition of done

Sprint 2 is closed. The SDK is published as `@pulsar-stellar/sdk@0.1.0` on npm. Tag `v0.1.0-app` is on GitHub with a release body covering what was delivered, verified, and not yet built. 428 tests at 100 percent coverage. 27 ADRs in `.agent/decisions.md`.

Sprint 2 ran three phases. Phase A (steps 1 through 15) scaffolded the workspace, CI, and toolchain. Phase B (steps 16 through 19) built the SDK skeleton: `PulsarClient` for the indexer path, `fetchLiveEvents`/`liveEventStream` for the RPC path, a full ScVal decoder, `buildContractCall`, and dual ESM/CJS output. Phase C (steps 20 through 37) completed the SDK with bindings, integration tests, README extraction as a build gate, and the publish pipeline. Every phase landed its verification gate before the next started.

Phase A landed four things the numbered sequence did not call for, each in its own commit: `scripts/verify-env-parity.sh`, which the spec requires but no step creates; `pnpm-lock.yaml`, without which a frozen-lockfile CI install cannot run; a correction moving pnpm strictness settings out of `.npmrc`, which pnpm 11 ignores, into `pnpm-workspace.yaml`; and a CI path-filter fix after `ci-go` and `ci-web` fired against sub-stacks that do not exist yet.

The toolchain is Node 22 LTS with pnpm 11, which supersedes the draft's Node 20 pin. pnpm 11 requires Node 22.13 or newer, so the two pins could not both hold. See ADR-012.

Several decisions recorded during Sprint 2 contradicted the planning draft. The event id format was not constructible as designed (ADR-022, ADR-024). The value taxonomy could not represent a u32 (ADR-023). Live RPC never returns a null cursor, so pages carry a plain string (ADR-025). Real events carry a flag marking reverted calls, which had nowhere to go (ADR-026). TransactionBuilder mutates the account it is handed, which would have burned sequence numbers silently (ADR-027). These are the load-bearing ADRs; the others fill in details around them.

### What Sprint 2 delivered

- `PulsarClient`: `ping`, `registerContract`, `getContract`, `listContracts`, `events`, `eventStream`, `event`.
- `fetchLiveEvents` and `liveEventStream`: discriminated union query enforced at compile time by `never` members and at runtime by Zod.
- Decoder covering every ScVal variant Soroban commonly emits, with an `unknown` fallback carrying base64.
- `buildContractCall` plus six helpers bridging wire events to generated bindings.
- Dual ESM and CommonJS output with declarations.
- README code blocks extracted and compiled as a build gate.
- TypeScript 5.6.3 consumer-floor compilation with node types absent.
- Live-tail testnet example returning real events through the built package.

### Next

Sprint 3 opens the Go indexer (Phase D). The SDK's wire contracts define what the indexer must produce: the response envelope (ADR-017), the events query contract (ADR-021), eventIndex as a ledger ordinal (ADR-022), the value taxonomy (ADR-023), and the success flag on every event (ADR-026). Absence requires both a 404 and a `not_found` envelope (ADR-019), and registration is idempotent on identical input (ADR-018).

### Open gaps carried forward

**The Postgres migration path has never been executed.** Steps 44 through 49 built the storage layer and validated it against SQLite only. Everything Postgres-specific is verified structurally rather than by running it: the DSN's TLS refusal is checked against `pgx.ParseConfig`'s output (ADR-031), the migration files are checked against their SQLite counterparts by the ADR-029 diff test, and the statement splitter is unit tested because pgx's extended protocol rejects the multi-statement `Exec` SQLite accepts. No Postgres server has applied `0001_init` or `0002_events_index`.

The cause is environmental, not a decision: the machine this was built on has no Docker, so testcontainers-go could not run, and writing tests that only CI could execute would mean pushing work never exercised locally. The gap is real and worth closing with a dedicated step wherever Docker is available. Until then, treat "the Postgres schema applies" as unproven.

## 7. Drips Wave context

The toolkit is aimed at a Drips Wave submission. That shapes two things. Repo hygiene is a deliverable, not an afterthought: branch protection, CONTRIBUTING, SECURITY, and a README that matches the pattern of approved Stellar repos all get a dedicated sprint before submission. And contributor-facing surface matters, because outside contributors are expected to work mostly in this repo rather than in `pulsar-core`, which stays reviewer-only until v1.0. Write issues and code with a stranger in mind.

## 8. Local development

```sh
nvm use                       # reads .nvmrc, Node 22
corepack enable               # provides pnpm
pnpm install                  # all TypeScript workspaces
cp .env.example .env.local    # then fill in anything non-default

pnpm typecheck
pnpm lint
pnpm test
pnpm build

cd indexer
go vet ./...
go test ./...
```

Per sub-stack commands land with each sub-stack and this list grows as they do.

## 9. Where secrets live

Nowhere in this repository. `.env.example` holds placeholders and defaults that are safe to publish, and it is the only env file that is committed. Real local values go in `.env.local`, which is gitignored.

In production, web values are set in the Vercel project's environment UI and indexer values in the Render service dashboard. Neither is mirrored into the repo. Anything prefixed `NEXT_PUBLIC_` reaches the browser, so a secret must never carry that prefix. If a credential does reach a commit, rotate it. Removing the commit is not sufficient.
