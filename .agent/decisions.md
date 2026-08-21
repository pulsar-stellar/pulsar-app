# Architecture Decision Records: pulsar-app

Append-only. Never rewrite an entry. If a decision is reversed, append a new ADR that supersedes the old one and set the old entry's status to `superseded by ADR-NNN`.

Append-only applies to ADRs from the moment they are pushed. Before that first push, corrections to unpublished entries are edits, not violations.

Format:

```
## ADR-NNN: <title>
Date: YYYY-MM-DD
Status: accepted | superseded by ADR-MMM

### Context
### Decision
### Alternatives considered
### Consequences
```

---

## ADR-001: pnpm workspaces over npm or Yarn
Date: 2026-08-20
Status: accepted

### Context

The TypeScript side of this repo is at least two packages, an SDK and a Next.js app, with the SDK consumed by the app before it is ever published. That needs a workspace manager. The SDK is also published to npm, so a phantom dependency, one that resolves locally through hoisting but is not declared, would ship as a broken install for consumers rather than as a local error.

### Decision

pnpm, declared in `pnpm-workspace.yaml` with members `packages/*` and `apps/*`. `.npmrc` sets `strict-peer-dependencies=true`, `auto-install-peers=false`, and `shamefully-hoist=false`.

### Alternatives considered

**npm workspaces.** Rejected. Hoists by default, so an undeclared import resolves silently and the failure surfaces only after publish.

**Yarn.** Rejected. Plug'n'Play interoperates poorly with parts of the Next.js and Go tooling story, and the non-PnP mode gives up the strictness that motivated the choice.

### Consequences

- A workspace can import only what it declares. Undeclared imports fail at install or resolve time here, not for a consumer after publish.
- Contributors need corepack or a pnpm install. This is stated in CONTRIBUTING.md.
- `pnpm-lock.yaml` is committed and is treated as load-bearing, not as generated noise.

---

## ADR-002: Go indexer as a separate module, not a workspace member
Date: 2026-08-20
Status: accepted

### Context

The indexer is a Go daemon living in the same repository as the TypeScript packages. It could be listed in `pnpm-workspace.yaml` with a shim `package.json` so that one command builds everything.

### Decision

`indexer/` has its own `go.mod` and is absent from `pnpm-workspace.yaml`. Its build, test, and lint commands are Go's, run from `indexer/`. CI runs it in a separate workflow.

### Alternatives considered

**Shim `package.json` wrapping Go commands in npm scripts.** Rejected. It buys one root command and costs a layer of indirection on every Go error message, plus a fake package that pnpm tries to resolve dependencies for.

**Separate repository for the indexer.** Rejected. The indexer's HTTP contract and the SDK's client are one interface described from two sides. Splitting repos means every change to that interface is a cross-repo dance.

### Consequences

- No single command builds the whole repo. CONTRIBUTING.md lists both sequences.
- Go tooling stays idiomatic. `go test ./...` works with no wrapper.
- The two toolchains upgrade independently.

---

## ADR-003: SQLite locally, Postgres in production
Date: 2026-08-20
Status: accepted

### Context

The indexer is meant to be runnable by a developer who wants event history for their own contract, and also runnable as a hosted service backing the public explorer. Those two want different databases. A developer wants zero setup. A hosted service wants concurrent writers and managed backups.

### Decision

Support both behind one storage interface, selected by `PULSAR_INDEXER_DB_DRIVER`. SQLite through `modernc.org/sqlite` is the default and what local development uses. Postgres through `pgx` is what the hosted indexer runs. Migrations are written to run on both.

### Alternatives considered

**Postgres only.** Rejected. Requiring a Postgres instance to try the toolkit locally is exactly the friction the project exists to remove.

**SQLite only.** Rejected. The hosted indexer needs concurrent access and operational tooling that SQLite does not offer.

### Consequences

- Every query must be valid on both engines. No engine-specific SQL without a driver-level branch.
- The migration set is constrained to the intersection of both dialects.
- Tests run against SQLite by default. Postgres-specific behavior needs its own integration test.

---

## ADR-004: Next.js App Router only
Date: 2026-08-20
Status: accepted

### Context

Next.js 15 supports both the App Router and the older Pages Router. Mixing them in one application is possible and is a common source of confusion about which data-fetching rules apply where.

### Decision

App Router only. No `pages/` directory. Server Components are the default and `use client` is added only where interactivity requires it.

### Alternatives considered

**Pages Router.** Rejected. It is in maintenance. New Next.js capability lands in the App Router.

**Both, migrating gradually.** Rejected. There is nothing to migrate from. This is a new application.

### Consequences

- Data fetching for initial render happens on the server. TanStack Query covers client-side paging and refetching only.
- Contributors familiar only with the Pages Router need to read the App Router data-fetching rules first.

---

## ADR-005: shadcn/ui components copied in, not installed
Date: 2026-08-20
Status: accepted

### Context

shadcn/ui is distributed as source you copy into your project rather than as a versioned dependency. It can also be wrapped into an internal package shared across apps.

### Decision

Copy components into `apps/web` under the project's own component directory. Do not wrap them in a workspace package. Edit them in place when the design needs something different.

### Alternatives considered

**An internal `packages/ui` wrapping shadcn.** Rejected. There is one consuming app. A shared package for a single consumer adds a build step and an indirection for no reuse.

**A component library dependency such as MUI.** Rejected. Restyling an opinionated library to match the project's visual direction costs more than owning the source.

### Consequences

- Component source is ours. Upstream fixes do not arrive automatically and are pulled in deliberately.
- Component code is reviewed like any other code in the repo, including its accessibility behavior.

---

## ADR-006: TanStack Query for client-side data fetching
Date: 2026-08-20
Status: accepted

### Context

The explorer's central view is a paginated event list served by the indexer, with cursor pagination, filters that change the query, and a refresh that should not blank the screen.

### Decision

TanStack Query v5 for all client-side fetching against the indexer. Server Components handle initial render, and Query takes over for pagination and refetching, hydrating from the server-rendered payload.

### Alternatives considered

**`useEffect` plus `fetch`.** Rejected. Cache invalidation, request deduplication, and keeping previous data visible during a refetch are the actual problem, and all three would be reimplemented by hand.

**SWR.** Rejected. Capable, but its cursor pagination story is thinner than Query's `infiniteQuery`, which matches the indexer's cursor model directly.

### Consequences

- One more client dependency and a provider in the tree.
- Query keys become part of the app's design and need a documented convention.

---

## ADR-007: chi for the indexer HTTP router
Date: 2026-08-20
Status: accepted

### Context

The indexer serves a small read API, under ten routes, with middleware for logging, recovery, request IDs, and rate limiting.

### Decision

`go-chi/chi` v5.

### Alternatives considered

**net/http alone.** Rejected. Go 1.22's routing patterns cover the paths but not the middleware chaining and route grouping, which would be hand-rolled.

**gin or echo.** Rejected. Both bring their own context type and their own handler signature, so every handler stops being a standard `http.Handler` and testing goes through framework machinery.

### Consequences

- Handlers stay `http.HandlerFunc` and are testable with `httptest` alone.
- Middleware from the wider Go ecosystem drops in without adapters.

---

## ADR-008: modernc.org/sqlite over mattn/go-sqlite3
Date: 2026-08-20
Status: accepted

### Context

The default local driver needs to work on a contributor's machine without setup, and inside a minimal container image for anyone self-hosting. `mattn/go-sqlite3` is a cgo binding and needs a C toolchain to build.

### Decision

`modernc.org/sqlite`, a pure-Go implementation. `CGO_ENABLED=0` stays the build default.

### Alternatives considered

**mattn/go-sqlite3.** Rejected. Faster, and it drags a C toolchain into every build environment, cross-compilation, and container image. The indexer's write volume does not need the speed.

### Consequences

- Static binaries. The container image can be built from scratch.
- Slightly slower than the cgo binding. Acceptable at this write volume, and worth revisiting with a measurement rather than a guess if it ever is not.

---

## ADR-009: Documentation lives in a separate pulsar-docs repo
Date: 2026-08-20
Status: accepted

### Context

The toolkit publishes a documentation site through GitBook, which syncs from a GitHub repository folder. The source could live in this monorepo as `apps/docs/`, which is what the system prompt's section 9 assumes, or in its own repository.

### Decision

Documentation source lives in `pulsar-stellar/pulsar-docs`. GitBook's GitHub Sync points at that repository's `main`. This repo has no `apps/docs` member, and section 9 of the system prompt does not apply here.

### Alternatives considered

**`apps/docs/` in this monorepo, per the system prompt.** Rejected. It documents both repos, not just this one, so hanging it off the app layer misrepresents its scope. It also means every prose edit triggers this repo's CI and appears in the same commit stream as SDK and indexer work.

### Consequences

- This deviates from the system prompt's sections 3.4, 4, and 9. Those sections are superseded by this ADR for `pulsar-app`.
- A public API change here is not complete until the matching PR is open against `pulsar-docs`. That coupling is now a review checklist item rather than something the build enforces.
- Documentation CI, link checking, and GitBook configuration are that repo's concern.

---

## ADR-010: Vercel for web, Render for indexer and Postgres
Date: 2026-08-20
Status: accepted

### Context

Two workloads with different shapes. The explorer is a Next.js app that wants edge delivery, preview deployments, and no server management. The indexer is a long-running daemon with a polling loop and a database attached, which is not what a serverless platform is for.

### Decision

Vercel hosts `apps/web`. Render hosts the indexer as a service plus its managed Postgres. Environment values are set in each platform's dashboard and never committed.

### Alternatives considered

**One platform for both.** Rejected. Running a persistent polling loop on a serverless platform means restructuring it into scheduled invocations, which changes the indexer's design to suit a hosting choice.

**Self-hosted VPS for both.** Rejected. Cheaper in money, more expensive in maintenance for a solo maintainer, and it gives up preview deployments.

### Consequences

- Two dashboards, two sets of environment variables, and two places to check when something is down.
- The web app reaches the indexer over the public internet, so the indexer's API needs CORS and rate limiting from the start.
- A deploy of one does not imply a deploy of the other. Version skew between explorer and indexer API is possible and the API must tolerate it.

---

## ADR-011: SDK mechanics in system-prompt section 6 are unverified and must be checked before Phase B
Date: 2026-08-20
Status: accepted

### Context

`docs/planning/system-prompt.md` sections 3.1 and 6 specify the TypeScript SDK against `@stellar/stellar-sdk` in concrete detail: a pinned version, a bindings generation command, an RPC server API, and a retention window. Those were written from the spec author's recollection rather than from upstream documentation open at the time. Four claims do not survive a first reading:

1. `@stellar/stellar-sdk` is pinned at `16.2.0`, with a `>=16.2.0` peer dependency range. That version line has not been confirmed to exist.
2. Section 6.6 instructs consumers to run `npx @stellar/stellar-sdk generate --contract-id C... --output-dir ./client`. The npm package is not known to ship a `generate` CLI. TypeScript bindings are generated by the Stellar CLI, as `stellar contract bindings typescript`.
3. Section 6.5 uses `new rpc.Server(url)` and `server.getEvents({ startLedger, filters: [{ type: 'contract', contractIds: [...] }], limit })`. Plausible for recent versions, and both the namespace and the response shape have moved between major versions.
4. The seven-day RPC event retention figure is stated as a fixed property. It is a configurable retention setting that varies by RPC provider.

The same discipline was applied in `pulsar-core` Sprint 1 Phase B: check the current upstream signature before writing against it, rather than writing from memory and letting CI find the difference.

### Decision

Phase A lands as specified, because no Phase A artifact names a Stellar dependency. Before the first Phase B commit, step 16, verify all four claims against current upstream documentation and the package's published types. Append an ADR recording what each check found and correcting the pin, the command, and the API shape where they are wrong. No SDK code is written against an unverified signature.

### Alternatives considered

**Write to the spec and let CI catch the difference.** Rejected. A wrong version pin fails at install and a wrong API shape fails at typecheck, but a wrong documented command for consumers fails silently in our docs and loudly for a user.

**Halt Phase A until the verification is done.** Rejected. Nothing in steps 1 through 15 references a Stellar dependency, so the scaffold and the verification are independent and can proceed in either order.

### Consequences

- Step 16 gains a prerequisite that is not in the build sequence: the verification pass and its ADR.
- Sections 3.1 and 6 of the system prompt are provisional until that ADR lands. Do not treat their code samples as authoritative.
- If the pinned version turns out not to exist, the pinned-version tables in section 3.1 need a correction pass, not a silent substitution.

---

## ADR-012: Node 22 LTS with pnpm 11, superseding the section 3.1 Node 20 pin
Date: 2026-08-20
Status: accepted

### Context

`docs/planning/system-prompt.md` section 3.1 pins Node.js at 20 LTS and pnpm at "9.x or newer". Both pins were written before the scaffold existed. They cannot both hold.

pnpm 11 declares `engines.node >= 22.13` and calls `node:sqlite`, a built-in module Node 20 does not have. The first CI run on Node 20 with pnpm 11.1.3 failed at `pnpm/action-setup` with `ERR_UNKNOWN_BUILTIN_MODULE: No such built-in module: node:sqlite`, before reaching a single project step. Checked against the registry: pnpm 9.15.9 and pnpm 10.34.5 declare `engines.node >= 18.12`, pnpm 11.21.0 and later declare `>= 22.13`.

Separately, Node 20 reached end of maintenance in April 2026. Pinning it in August 2026 means building on a runtime that no longer receives security patches, and scheduling a forced migration into a later sprint.

Section 3.1 says to halt on a pinned version that produces an install error and find the compatibility matrix rather than iterating version by version. This ADR is that halt's outcome.

### Decision

Node 22.x LTS with pnpm 11.x.

- `.nvmrc` carries `22`
- `package.json` sets `packageManager` to `pnpm@11.1.3` and `engines.node` to `>=22.13.0`
- `pnpm-lock.yaml` is generated under pnpm 11
- CI resolves Node from `.nvmrc`, so no workflow version literal needs maintaining

This supersedes the Node and pnpm rows of section 3.1 for `pulsar-app`. Every other pin in that table stands.

### Alternatives considered

**Node 20 with pnpm 10.34.5.** Rejected. It satisfies section 3.1 exactly as written and is the only option that needs no deviation, but it adopts a runtime that stopped receiving security patches four months ago and defers the migration rather than avoiding it. A repo whose SECURITY.md is a submission deliverable should not build on an unsupported runtime to preserve a pin.

**Node 24 with pnpm 11.** Rejected. Node 24 enters LTS in October 2026 and is not LTS today. This scaffold is the foundation for months of work, and pinning a non-LTS line for that adds risk with no benefit over Node 22.

### Consequences

- Contributors run `nvm install 22`. The machine this was scaffolded on carries Node 24, so the maintainer needs the install too.
- Node 22 is in active LTS until October 2027, so the next forced runtime decision is more than a year out.
- Section 3.1's Node and pnpm rows are superseded here and stay unchanged in `docs/planning/system-prompt.md`, which is shared planning material rather than a file this repo owns. Anyone reading that table for `pulsar-app` should read this ADR first.
- The same conflict will reach `pulsar-core` only if it adopts a Node toolchain. It does not have one today.

---

## ADR-013: SDK verification findings, discharging the ADR-011 gate
Date: 2026-08-20
Status: accepted

### Context

ADR-011 held that four claims in the planning draft's SDK spec were unverified and had to be checked against upstream before Phase B step 16. That check has now run, against `@stellar/stellar-sdk@16.2.0` installed and read directly, its shipped `.d.ts` files, its CLI executed, and current Stellar RPC documentation. Two of the four claims were correct as written, and ADR-011's own skepticism about them was wrong.

**Claim 1, the version pin. Confirmed.** `16.2.0` is the current stable release, published 2026-07-29. The line runs 11.x through 16.x, and a `17.0.0-rc.2` prerelease exists, published 2026-08-17.

**Claim 2, the bindings command. Confirmed, with one omission.** The package does ship a CLI. Its `bin` is named `stellar-js`, not `stellar-sdk`, but `npx @stellar/stellar-sdk generate` resolves to it and runs, so the documented invocation works verbatim. `--contract-id` and `--output-dir` both exist. The omission: fetching by contract ID also requires `--network` or `--rpc-url`, and without one the command exits with `--rpc-url is required when fetching from network`. Verified end to end by generating bindings for the showcase contract `CDNWTVUDKCCGW7GOC6SBLUFXXUCD2YDHWRDUSXZ6CYBQKQWLCUYYWI5L` with `--network testnet`, which produced a client exposing the contract's real surface, including the `AmountOutOfRange` error variant.

**Claim 3, the RPC API shape. Confirmed, with two corrections.** `rpc.Server` is correct: the class is declared `RpcServer` and re-exported as `Server` from `rpc/index.js`. The constructor is `(serverURL: string, opts?: RpcServer.Options)`. `getEvents(request: Api.GetEventsRequest): Promise<Api.GetEventsResponse>` is correct, and the filter shape `{ type: 'contract', contractIds: [...] }` is correct.

The first correction is that `GetEventsRequest` is a discriminated union, not a flat object. Ledger-range mode takes `startLedger` with optional `endLedger` and declares `cursor?: never`. Cursor mode takes `cursor` and declares `startLedger?: never` and `endLedger?: never`. The two cannot be mixed, so a paginating loop must switch modes after its first page rather than carry `startLedger` forward.

The second is that `EventType` is `"contract" | "system"`. There is no `"diagnostic"` member, despite a JSDoc example in `server.d.ts` that shows `type: "diagnostic"`. Diagnostic events cannot be filtered for through this type.

Field names to write against, from `EventResponse` and its base: `id`, `type`, `ledger`, `ledgerClosedAt`, `transactionIndex`, `operationIndex`, `inSuccessfulContractCall`, `txHash`, `topic` (singular, `xdr.ScVal[]`), `value` (`xdr.ScVal`), and `contractId` (optional, a `Contract` instance rather than a string). `GetEventsResponse` carries `events`, `cursor`, `latestLedger`, `oldestLedger`, `latestLedgerCloseTime`, and `oldestLedgerCloseTime`.

**Claim 4, the seven-day retention window. Corrected.** Retention is not a fixed protocol property. Stellar RPC retains a ledger-denominated history governed by a single `history-retention-window` setting whose stock default is 120960 ledgers, which is roughly seven days at current ledger cadence. It is per-node configuration, so a provider may run a shorter or longer window, and the documentation states no per-network difference between testnet, mainnet, and futurenet. The effective window is readable at runtime: `getHealth()` returns `ledgerRetentionWindow`, `oldestLedger`, and `latestLedger`, and every `getEvents` response carries `oldestLedger` and `oldestLedgerCloseTime`.

### Decision

Phase B may begin. Write SDK and indexer code against the shapes recorded above, with these four rules.

1. Pin `@stellar/stellar-sdk` at exactly `16.2.0` as a dev dependency. Do not adopt the 17.0.0 release candidate line, on the same reasoning as `pulsar-core` ADR-001: a prerelease changes before it is final, and event decoding is the correctness boundary for every downstream consumer.
2. Declare the peer range as `>=16.2.0 <17.0.0` rather than the draft's open-ended `>=16.2.0`. An open range admits a major version whose decoding behavior has not been checked against our fixtures.
3. Never hardcode seven days or 120960 ledgers as a retention constant. Read `oldestLedger` from the response the indexer already has, and treat the gap between it and the last indexed ledger as the coverage question. The seven-day figure stays in prose, where it is described as a default rather than a guarantee.
4. Document the bindings command with `--network testnet` included, since the draft's example does not run as written.

### Alternatives considered

**Adopt 17.0.0-rc.2.** Rejected. Same reasoning as `pulsar-core` ADR-001.

**Keep the open-ended peer range from the draft.** Rejected. It moves a compatibility decision from us to whichever version a consumer's resolver happens to pick.

**Treat seven days as a constant and skip the runtime check.** Rejected. It is a default, not a guarantee, and the value needed to decide whether history has a hole is already present in every response.

### Consequences

- ADR-011's gate is discharged. Its text stands unedited, including the two predictions this ADR shows were wrong, because the log is append-only. Read this entry alongside it.
- The planning draft's section 6.5 sample does not paginate correctly as written and its section 6.6 command does not run as written. Both are superseded here.
- The indexer's event table can be modeled directly on the field list above.
- A `17.0.0` upgrade needs its own ADR and a fixture comparison proving decoded shapes are unaffected.

---

## ADR-014: Verify external SDK behavior before building on it
Date: 2026-08-20
Status: accepted

### Context

ADR-011 flagged four claims in the planning draft's SDK spec as unverified. ADR-013 recorded what checking them found: two were correct exactly as written, and two were wrong in ways that would have shipped as defects. The draft's `getEvents` sample cannot paginate, because the request type is a discriminated union that forbids mixing a cursor with a ledger range. Its bindings command does not run, because fetching by contract ID also needs `--network` or `--rpc-url`. Both would have compiled or appeared to work in review.

Note which way the errors ran. ADR-011 predicted the version pin and the bindings command were wrong; both were right. It accepted the RPC sample and the retention figure as plausible; both were wrong. Skepticism pointed at the wrong claims, so grading claims by how suspicious they look does not work. Only checking does.

The cause is ordinary. The spec was drafted before `@stellar/stellar-sdk` 16.2.0 was published on 2026-07-29. A specification written against a moving dependency is accurate as of its drafting date and drifts from there, silently, with no signal that it has.

`pulsar-core` hit the same thing in Sprint 1 Phase B, where the `#[contractevent]` behavior in its own draft predated soroban-sdk 26.1.0's macro and described helper functions that no longer existed. It was caught by checking against the pinned SDK before writing, and the draft section carries a stale marker pointing at the tracked correction.

### Decision

Before writing code against an external dependency's API, verify the shape you are about to write against, using the dependency itself rather than a specification, a memory, or a search result summarizing an older version. In order of preference: read the installed package's type declarations, execute its CLI, or read the current upstream documentation.

This applies whenever a planning document, an ADR, or a prior session states an external API's shape. It applies with full force to `@stellar/stellar-sdk`, `github.com/stellar/go`, and Stellar RPC, where names and response shapes have moved between majors.

When verification contradicts a document, record the correction in this log, add a stale marker to the superseded section of the draft, and build against the correction. Halt rather than guess when the check produces a surprise.

### Alternatives considered

**Trust the specification and let CI catch the difference.** Rejected. A wrong version pin fails at install and a wrong type fails at typecheck, but a wrong documented command for consumers passes CI and fails for a user, and a pagination loop that silently returns one page passes every test that does not specifically probe the second page.

**Verify once at the start of a phase and treat the result as settled.** Rejected as insufficient on its own. The phase-entry pass catches the claims someone thought to list. Claims surface mid-implementation too, and each one gets the same treatment when it does.

### Consequences

- Phase entry carries a verification pass whose findings land as an ADR before the first commit of the phase. Phase B's is ADR-013.
- Verification continues inside a phase: each SDK claim is checked at the point it is used, and a surprise halts the work rather than being worked around.
- Where behavior can be isolated, it is mutation-checked: break the thing under test and confirm the test notices. A check that cannot fail is not evidence, which is why `scripts/verify-env-parity.sh` was checked against both a missing TypeScript variable and a missing Go one before it was committed.
- This costs time at the start of each phase and is cheaper than the alternative, which is finding the same defect after it reaches a consumer.

---

## ADR-015: SDK toolchain versions, TypeScript 5.9 with Zod 4
Date: 2026-08-20
Status: accepted

### Context

Section 3.1 of the planning draft pins TypeScript at "5.6 or newer", Zod at 3.x, Vitest at 2.x, and tsup at 8.x. Checked before writing `packages/sdk`, per ADR-014. Three of the four had drifted: TypeScript is at 7.0.2, published 2026-07-08, with 6.0.2 also available and 5.9.2 ending the 5 line; Zod is at 4.4.3 with 3.25.76 ending the 3 line; Vitest is at 4.1.11 with 2.1.9 ending the 2 line. The tsup pin holds at 8.5.1.

Each of these reaches consumers through a different surface, which is what makes them separate decisions rather than one "how current do we want to be" decision.

### Decision

| Package | Version | Against section 3.1 |
|---|---|---|
| TypeScript | 5.9.2 | satisfies "5.6 or newer" |
| Zod | 4.4.3 | deviates from 3.x |
| Vitest | 4.1.11 | deviates from 2.x |
| tsup | 8.5.1 | holds |

**TypeScript 5.9.2.** The SDK emits `.d.ts` files that consumers compile with their own toolchain. A newer major risks emitting declarations a consumer's setup cannot consume, and the failure lands on them rather than on us. 5.9.2 satisfies the section as written, needs no deviation, and defers 6.x and 7.x until their ecosystem maturity is demonstrated rather than assumed.

**Zod 4.4.3.** Zod is a runtime dependency and lands in the consumer's dependency tree. Pinning 3.x while consumers increasingly run 4.x puts two copies of Zod in their tree. This SDK has no schemas yet, so the migration cost that normally argues for staying on 3.x is zero right now and grows with every schema written.

**Vitest 4.1.11.** Dev-only, invisible to consumers, and it accepts the Node 22 pin from ADR-012.

**tsup 8.5.1.** No decision needed.

### Alternatives considered

**TypeScript 6.0.2.** Rejected. Middle ground that buys little: it carries adoption risk without the 5 line's compatibility record.

**TypeScript 7.0.2.** Rejected. Too fresh for a correctness boundary. Published five weeks before this decision, and it is the major rewrite, so it is the version most likely to emit declarations that trip an older consumer toolchain.

**Zod 3.25.76.** Rejected. Matching the section exactly would defer the migration to a point where schemas exist and it costs real work, while creating duplicate-copy overhead for every consumer already on 4.x in the meantime.

**Vitest 2.1.9.** Rejected. Two majors behind with no compensating benefit, since nothing about the test runner reaches a consumer.

### Consequences

- Declarations are emitted against TypeScript 5.9. Consumers on 5.6 and later compile against them.
- Moving to TypeScript 6 or 7, or to a future Zod major, needs its own ADR and a check that decoded shapes and emitted declarations are unaffected.
- Zod 4 idioms are used from the first schema. No 3.x compatibility layer is written.
- The meta-principle, which generalizes past this decision: for a library other projects consume, "newest" is the wrong optimization and "compatible with the most consumers" is the right one. What that yields differs by how the dependency reaches the consumer. TypeScript reaches them through emitted `.d.ts`, where a newer major raises incompatibility risk, so compatibility means older. Zod reaches them through their dependency tree, where an older major raises duplicate-copy risk, so compatibility means newer. Same principle, opposite conclusions, because the surfaces differ. Ask which surface a dependency touches before asking how current to be.
- This is the same discipline `pulsar-core` applies to wire shapes: once pinned, they are a consumer contract, not an implementation detail.

---

## ADR-016: DecodedEvent is wire-oriented, generated bindings are call-oriented
Date: 2026-08-20
Status: accepted

### Context

Generating TypeScript bindings for the showcase contract and comparing them against `types.ts` showed the two shapes do not compose. The binding emits:

```ts
export interface DepositEvent { name: "Deposit"; data: { from: string; amount: bigint } }
```

The contract's `events.rs` sets the wire shape explicitly:

```rust
#[contractevent(topics = ["deposit"], data_format = "single-value")]
pub struct Deposit { #[topic] pub from: Address, pub amount: i128 }
```

Two divergences follow. The leading wire topic Symbol is `deposit`, lowercase, pinned in the annotation precisely so renaming the Rust struct cannot change the wire contract; the binding's `name` is `"Deposit"`, derived from the struct name. And `from` is marked `#[topic]`, so on the wire it is the second topic, while the binding places it inside `data`.

Neither is a defect. They are two views of the same event. The binding is call-oriented: it describes the contract the way a caller invoking it thinks about it, with named fields and no topic-versus-data distinction. `DecodedEvent` is wire-oriented: it describes what the ledger actually holds.

The hazard is a consumer holding both and assuming they interchange. Matching a `DecodedEvent.name` of `deposit` against the binding's `"Deposit"` literal silently never matches, and reading `from` out of `data` finds nothing because it is a topic.

### Decision

`DecodedEvent` stays wire-faithful. `name` is the decoded leading topic Symbol exactly as emitted, topics stay separate from data, and the raw XDR travels alongside as provenance.

Converting between the two views is an explicit helper landing with `contract.ts` at step 34, opt-in for consumers who want both. It is never an implicit conversion, and no shape here is bent to make the two look interchangeable.

### Alternatives considered

**Align `DecodedEvent` to the binding shape.** Rejected. It would make the indexer report an event name that does not appear on the ledger. The decoder's whole value is that what it reports is what was emitted, checkable against the raw XDR it ships beside it. A capitalized name that matches no topic would break that.

**Carry both names on every event, such as `name` and `typeName`.** Rejected. Two names on every event pushes the disambiguation onto every consumer on every event, including the majority who use one view. The confusion is worth solving once in a helper rather than in every consumer's matching logic.

### Consequences

- A consumer using both views writes two lines to bridge them. Explicit and visible beats implicit and surprising.
- The step 34 helper carries the mapping's tests, including the case difference and the topic-versus-data split, since those are exactly what a hand-rolled bridge gets wrong.
- The bindings composition check belongs to step 34, because it cannot be written before the helper it checks exists. Its design, settled here so it is not relitigated: generate bindings at test time into `tmp/test-fixtures/<hash>/`, gitignored, where the hash covers contract ID, network, and SDK version, so different inputs get different directories and identical inputs reuse one. Generated code is a build artifact, and a committed fixture would go stale against the deployment it claims to describe. Runtime generation also exercises the same command a consumer runs. The assertions are type-level, through `vitest --typecheck`, because the binding's event exports are interfaces and are erased at runtime. The test skips rather than fails when RPC is unreachable, so an outage does not turn CI red over something we do not control.
- If a future contract change makes the two views diverge further, that surfaces in the step 34 test as a type error rather than in a consumer's silently empty match.

---

## ADR-017: The indexer HTTP response envelope, fixed from the SDK side
Date: 2026-08-21
Status: accepted

### Context

The planning draft's section 7.2 lists `GET /health` as returning `{ ok, version, latest_ledger, tracked_contracts }`, and immediately after the endpoint list shows a response envelope of `{ data, next_cursor, meta: { took_ms } }`. It does not say whether the envelope wraps every endpoint or only the paginated ones, and the two readings produce different wire shapes for the same route.

The SDK is written before the indexer, so whichever shape lands here becomes the contract the Go side is built against. Guessing silently would leave the disagreement to be discovered at Phase D integration, when both sides already have tests asserting incompatible shapes.

### Decision

The envelope wraps every JSON response from the indexer, including `/health`. Success is:

```json
{ "data": { "ok": true, "version": "0.1.0", "latest_ledger": 12345, "tracked_contracts": 3 },
  "meta": { "took_ms": 12 } }
```

`next_cursor` appears only on paginated responses and is absent elsewhere rather than null. `meta` is optional from the SDK's perspective: the client validates it when present and does not require it.

Field names on the wire stay snake_case, matching the draft and Go's conventions. The SDK maps them to camelCase at its boundary, so a TypeScript caller never sees snake_case and the Go side never has to emit camelCase.

Errors use the error envelope the same section defines, and the SDK treats any response carrying `error` as a failure regardless of status code.

### Alternatives considered

**Bare objects on non-paginated endpoints.** Rejected. A client would need to know per-route which shape to expect, and every new endpoint becomes a fresh decision. Uniformity costs a few bytes on a health check and removes a category of integration bug.

**camelCase on the wire.** Rejected. It would put the conversion on the Go side, where it fights the language's conventions, and the SDK has to validate and reshape the response anyway.

**Leave it undecided until the indexer is built.** Rejected. That is the same decision made later, with two implementations already committed to opposite readings.

### Consequences

- Phase D builds the indexer to this contract. Section 7.2's ambiguity is resolved here and the draft carries a stale marker pointing at this entry.
- The SDK's response parsing is uniform: unwrap `data`, check for `error`, map snake_case to camelCase, then validate against the shape for that route.
- `meta.took_ms` is the indexer's own measurement and is not the same as the round-trip latency the SDK measures. Where both exist, they are reported separately rather than one standing in for the other.
- If the indexer later needs a non-enveloped route, such as a plaintext readiness probe for a load balancer, that is a new ADR rather than a quiet exception.

---

## ADR-018: Contract registration is idempotent on identical input
Date: 2026-08-21
Status: accepted

### Context

Section 7.2 specifies `POST /contracts` with a body of `{ contract_id }` returning `ContractInfo`, and says nothing about what happens when the same contract is registered twice. The two readings are a conflict response, or a success returning the existing record.

As with ADR-017, the SDK is written before the indexer, so whichever reading lands here becomes the contract Phase D is built against.

One piece of evidence sits in the spec already. The error envelope's code enum is `not_found`, `validation`, `internal`, and `rate_limited`. There is no `conflict` code, so the 409 reading would require inventing one.

### Decision

Registering a contract that is already tracked succeeds and returns the existing `ContractInfo`, with the same status code as a first registration. The operation is idempotent on identical input.

The response is the record as it stands, including its real `added_at`, `first_indexed_ledger`, and `last_indexed_ledger`. A repeat registration does not reset indexing progress, does not change `status`, and does not re-add the contract.

### Alternatives considered

**Return 409 with a `conflict` code.** Rejected. It makes the common retry path an error path. A client that registers a contract, loses the response to a network failure, and retries is doing the right thing, and it should not have to distinguish "already registered by my own lost request" from "already registered by somebody else" to carry on. It also requires adding an error code the spec does not define.

**Return 200 for an existing contract and 201 for a new one.** Rejected as a middle ground that helps nobody here. The SDK returns `ContractInfo` either way and does not surface the status code, so the distinction would exist only for callers reading raw HTTP, and it would still leave a retry after a lost response reporting a different status than the call it is retrying.

### Consequences

- Phase D implements `POST /contracts` as an upsert that leaves existing rows untouched and returns them. Section 7.2's silence is resolved here and the draft carries a stale marker pointing at this entry.
- A caller cannot learn from the response whether the contract was newly added. If that is ever needed it is a field on the response, decided in its own ADR, not a status code.
- Registration is safe to retry, which means the SDK may retry it on transport failure without special handling. No retry logic exists yet; this records that it would be sound.
- `DELETE /contracts/:id` returning 204 stays as specified. Deleting an untracked contract is a separate question this ADR does not answer.
