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

---

## ADR-019: Absence requires both a 404 status and a not_found envelope
Date: 2026-08-21
Status: accepted

### Context

`getContract` returns `ContractInfo | null`, where null means the indexer is not tracking that contract. Section 6.7 is explicit that absence is null and never undefined, but it does not say what on the wire constitutes absence.

Three signals could carry it: the HTTP status, the error envelope's `not_found` code, or both. They can also disagree, and a client has to decide what to do when they do.

### Decision

`null` is returned only when the response carries **both** HTTP 404 **and** an error envelope whose `code` is `not_found`.

Every other shape is an error:

| Response | Result |
|---|---|
| 404 with `not_found` envelope | `null` |
| 404 with no envelope, or an envelope with a different code | `PulsarNetworkError` |
| 2xx carrying any error envelope | `PulsarValidationError` |
| any other non-success status | `PulsarNetworkError` |

The 2xx-with-error-envelope case is deliberately a validation error rather than a network one. The transport worked and the server contradicted itself, which is a response-shape problem, not a connectivity problem.

This rule applies only where a method is documented to return null. On every other route a 404 is an error regardless of what envelope accompanies it.

### Alternatives considered

**Status alone.** Rejected. It couples the SDK's notion of absence to HTTP, so any proxy, CDN, or misrouted request that produces a 404 becomes indistinguishable from the indexer saying the contract is untracked. A caller would read a routing bug as a legitimate absence.

**Envelope alone, with a 200 status.** Rejected. It breaks HTTP semantics for everything between the SDK and the indexer: proxies, monitoring, and load balancers all read status codes, and reporting absence as success makes a 404 rate invisible in operational dashboards.

### Consequences

- `null` means the resource does not exist according to a structured signal the indexer deliberately sent. A caller can branch on it without a second check.
- A bare 404 surfaces as an error with the status, URL, and operation attached, which is what is needed to diagnose a routing or proxy problem.
- Contradictory signals surface as validation errors, keeping "the server is confused" distinct from "the server is unreachable".
- Phase D must send both signals together on absence. A handler returning a bare 404 is a bug against this ADR.

---

## ADR-020: Add GET /contracts/:id
Date: 2026-08-21
Status: accepted

### Context

Section 6.2 declares `getContract(contractId): Promise<ContractInfo | null>`, but section 7.2's endpoint list has no route for a single contract. It carries `GET /contracts`, `POST /contracts`, `DELETE /contracts/:id`, and `GET /contracts/:id/events`. The single-item GET is a gap rather than a deliberate omission: the DELETE on that exact path already exists.

### Decision

Add `GET /contracts/:id` to the indexer's HTTP surface. It returns the contract's record on 200 in the standard envelope, and on absence returns 404 together with a `not_found` error envelope, per ADR-019.

The SDK's `getContract` routes to this endpoint.

### Alternatives considered

**Derive `getContract` from `listContracts`.** Rejected. It transfers the whole tracked-contract list on every single-contract lookup, and it degrades as the list grows, which means it works in development and gets slower in production without any code changing. It also destroys the structured absence signal: a filter returning nothing cannot distinguish an untracked contract from a request that failed to reach the right indexer.

### Consequences

- Phase D implements this handler symmetrically with the existing `DELETE /contracts/:id`.
- Section 7.2's endpoint list is incomplete as written. The draft carries a stale marker pointing here.
- ADR-019 applies to this route and, so far, only to this route.

---

## ADR-021: The events query wire contract
Date: 2026-08-21
Status: accepted

### Context

Section 7.2 gives `GET /contracts/:id/events` its query parameters and a general status code table: 200 for success, 400 for validation, 404 for missing, 429 for rate limit, 500 for internal, and never 200 with an error payload. It does not say what the route returns for a contract the indexer is not tracking, what an exhausted cursor looks like, or how an event's identifier is serialized.

Each of those is a decision the SDK has to make now, and Phase D then has to match.

### Decision

**An untracked contract is a 404 with a `not_found` envelope, and the SDK throws.** It is not an empty page. "This contract is not being indexed" and "this contract has emitted nothing matching your filters" are different facts, and collapsing them means a caller who registered the wrong ID, or forgot to register at all, sees a plausible empty result instead of an error. The `events` method is not documented to return null, so ADR-019's absence path does not apply to it and the 404 surfaces as a `PulsarNetworkError` carrying the contract ID.

**A tracked contract with no matching events is 200 with `{ items: [] }`.** The query succeeded and matched nothing.

**Cursor exhaustion is `next_cursor` being null or absent.** Both mean the same thing, and the SDK normalizes them to null. An empty string is not a valid cursor and is rejected. A caller pages until `nextCursor === null`.

**Event identifiers are strings on the wire.** The `events` table uses `BIGSERIAL`, whose range exceeds what a JSON number can carry without losing precision, so serializing an id as a number would silently corrupt it past 2^53. The SDK treats the id as opaque and never parses it.

**Payload shape matches the contract list**: the envelope's `data` holds `{ items: [...] }`, with `next_cursor` alongside `data` in the envelope rather than inside it, per ADR-017.

### Alternatives considered

**Return an empty page for an untracked contract.** Rejected. It makes the most common integration mistake, querying a contract nobody registered, indistinguishable from a correct query with no results. That failure is silent and looks like data.

**Signal exhaustion by an empty-string cursor.** Rejected. It makes the sentinel a valid-looking value that a caller might pass back, and it forces every consumer to remember which falsy value means done.

**Serialize event ids as JSON numbers.** Rejected. It works until the table passes 2^53 rows and then corrupts ids with no error anywhere.

### Consequences

- Phase D returns 404 with `not_found` for an untracked contract on this route, and 200 with an empty `items` array for a tracked contract with no matches.
- Phase D serializes `events.id` as a string. This is a correctness requirement, not a style preference.
- The SDK's paging loop terminates on `nextCursor === null`, and the same rule applies to every paginated route added later.
- A caller who wants to distinguish "not tracked" from "no events" catches `PulsarNetworkError` and reads its status, or calls `getContract` first, which returns null rather than throwing.

---

## ADR-022: eventIndex is an ordinal within the ledger, not within the transaction
Date: 2026-08-24
Status: accepted

### Context

Section 6.4 documents `DecodedEvent.eventIndex` as the event's "position within tx", and section 7.1 enforces that reading with `UNIQUE (tx_hash, event_index)` on the events table. The direct-RPC path in step 32 has to populate the same field, so the two sources have to agree on what it means.

Soroban RPC cannot produce a per-transaction index. A live `getEvents` call against testnet returned three consecutive events with three different `txHash` values but `transactionIndex: 0` and `operationIndex: 0` on all three. Only the second component of the event's `id` incremented, `0000000000` through `0000000002`, and it did so across transactions within one ledger. The RPC event id has the form `{toid}-{eventOrder}`, and that ordinal is ledger-wide.

Two further facts from the same call. The event `id` doubles as the paging cursor, and the SDK's own type declaration documents it as "the JSON-RPC request ID", which is wrong; it is the event's identifier.

### Decision

`eventIndex` is the event's ordinal position within its ledger, on both paths.

The RPC path takes it from the second component of the event id. The indexer adopts the same definition, and section 7.1's constraint becomes `UNIQUE (ledger, event_index)`.

### Alternatives considered

**Keep "position within tx" and derive it on the RPC path** by counting events sharing a `txHash` within the fetched page. Rejected. It is wrong whenever a transaction's events straddle a page boundary, and wrong silently: a consumer storing events keyed on `(txHash, eventIndex)` then gets duplicate keys, which either trips a uniqueness constraint or overwrites a real event with another.

**Drop `eventIndex` from `DecodedEvent`** and identify an event by its opaque id alone. Rejected. It forces any consumer wanting ordering to parse the id, which leaks the id's internal structure into consumer code and makes the format hard to change later.

### Consequences

- Ordering by `(ledger, eventIndex)` gives correct global order on both paths.
- Ordering within a single transaction still works, because a transaction's events stay contiguous in the ledger-wide ordinal.
- Section 7.1's schema changes before it is written. This is cheap now and would be expensive after Phase D ships, which is the reason to settle it here rather than at integration.
- The RPC path's synthetic id and this ordinal come from the same field, so a malformed RPC id fails both at once rather than producing an event with a plausible-looking wrong index.

---

## ADR-023: The DecodedValue taxonomy, extended to what Soroban actually emits
Date: 2026-08-24
Status: accepted

### Context

Section 6.4's `DecodedValue` union covers `address`, `symbol`, `i128`, `u128`, `bytes`, `string`, `bool`, `vec`, `map`, `tuple`, and `void`. Soroban's `ScVal` also carries `u32`, `i32`, `u64`, `i64`, `timepoint`, `duration`, `u256`, `i256`, `error`, and `contractInstance`. A `u32` is among the most common things a contract emits, for counters, indices, and version tags, and the union has nowhere to put one.

The gap would not have surfaced in testing. The showcase contract emits only `Address`, `i128`, `Symbol`, and `Bytes`, all of which the original union covers, so every fixture calibrated against it passes. `pulsar-core` defines no value taxonomy of its own, so there was no cross-repo answer to adopt.

`tuple` has the opposite problem: it is unreachable from XDR, because Soroban encodes a tuple as a vector. Only a decoder holding the contract's spec can tell them apart.

The `map` variant carries a second question. Soroban map keys are themselves `ScVal`s, so a key can be a Symbol, an Address, or an integer, and section 6.4's `Record<string, DecodedValue>` has nowhere to put that. `contractInstance` carries a third: its shape can be inferred from `@stellar/stellar-sdk`, but this project has never observed one in an event.

### Decision

Extend the union with the variants Soroban commonly emits, and add a fallback that cannot fail.

| Variant | Carried as | Why |
|---|---|---|
| `u32`, `i32` | `number` | 32 bits always fit |
| `u64`, `i64`, `u128`, `i128`, `u256`, `i256` | `string` | wider than 2^53, so a JSON number rounds silently, per ADR-021 |
| `timepoint`, `duration` | `string` | unsigned 64-bit second counts, same reasoning |
| `unknown` | `{ xdr: string }` | anything this SDK version cannot name, base64 intact |

`error` and `contractInstance` are deliberately left to the fallback for v0.1: both are unusual in event payloads, and an opaque value with its XDR is more useful than a half-modelled one.

The `map` variant is an ordered `Array<{ key: DecodedValue; value: DecodedValue }>`. That preserves wire ordering, supports non-string keys, keeps duplicate keys, and serializes through `JSON.stringify` unchanged. Consumers who need frequent lookup build their own indexed structure from the array, explicitly and with their own key policy.

`tuple` stays in the union for the indexer, which can consult the contract spec. The RPC path emits `vec`.

### Alternatives considered

**Fallback only, leaving the union as section 6.4 has it.** Rejected. It abandons the decoder's whole purpose for something as ordinary as a `u32`, handing consumers base64 to decode themselves.

**Extend the union and throw on anything unrecognized.** Rejected. A future protocol version adding an `ScVal` variant would then break an SDK that had been working, and it would break it at read time on events already indexed. Degrading keeps old clients reading new data.

**A JavaScript `Map` for the `map` variant.** Rejected. `JSON.stringify` renders one as `{}`, so any consumer serializing an event loses its payload. Lookup is by reference equality, so a consumer calling `has()` with a freshly built `DecodedValue` key never hits. And its insertion ordering is a runtime property rather than something the type states.

**An object with stringified keys, as section 6.4 has it.** Rejected. It erases the key's type, so a Symbol key `admin` and a String key `admin` become the same entry. It collapses duplicate keys silently, keeping only the last. And it loses the wire ordering. Measured, not assumed: `scValToNative` on a two-entry map with the same key twice returns a single entry, and that is the behaviour this variant exists to avoid inheriting.

**A typed `contractInstance` variant, shaped as `{ type: 'contractInstance', executable, storage }` after the SDK's `ScContractInstance`.** Rejected for v0.1. It is speculative typing with no validation surface: the shape is read off a type declaration rather than observed in an emission, and `contractInstance` has to compose with Soroban's runtime executable structure, which makes a wrong guess more costly than usual. Sprint 2's recurring pattern is that the draft says one thing and live testnet says another, and there is no emission here to check against. `scValToNative` itself declines to convert one, handing back the raw XDR struct. A typed variant waits for v0.2, when emission evidence and consumer demand exist; the migration is a compile-time type change.

### Consequences

- New `ScVal` variants arrive as `unknown` rather than as an exception, so a protocol upgrade does not require an SDK upgrade to keep reading.
- The indexer stores the extended set. Its JSON columns need no schema change, since the taxonomy lives in the value rather than the column type.
- This union is the wire contract for `pulsar-decoder` v0.2. The Rust decoder must produce these variants, including the fallback, and any future extension is a coordinated change across both repos.
- The decoder never throws. A value that fails mid-decode degrades to `unknown` with its XDR, so one corrupt value cannot discard the rest of a page.
- A consumer reading a map iterates entries rather than indexing by key. That is the deliberate cost of keeping key type, ordering, and duplicates, and it is paid at the one place where the alternative loses data silently.
- Verified against live testnet output: eight real contract events decoded to `symbol`, `address`, and `i128` with zero unknowns, and the `address` variant correctly carries `G`-prefixed account addresses as well as contract ids.

---

## ADR-024: RPC-fetched events carry a prefixed synthetic id

Date: 2026-08-28
Status: accepted

### Context

`DecodedEvent.id` is the indexer's own primary key, a `BIGSERIAL` rendered as a string of digits. The direct-RPC path in step 32 produces the same type from a different source, and that source has an identifier of its own: every event in a `getEvents` response carries an `id` of the form `{toid}-{eventOrder}`, which also serves as the paging cursor.

Two identifiers therefore exist for what a consumer sees as one type, and they come from namespaces that have nothing to do with each other. Nothing structural stops an indexer id and an RPC event ordinal from colliding as strings.

An earlier draft composed the id as `rpc:{txHash}:{eventIndex}`. A live `getEvents` call killed it: five consecutive events returned three different `txHash` values with `transactionIndex` and `operationIndex` identical at zero on all of them, so neither index identified an event within its transaction. Only the id's second component incremented, and it did so across transactions. That finding is ADR-022.

### Decision

An event fetched over RPC gets `id: "rpc:{rpcId}"`, where `rpcId` is the event's own identifier exactly as RPC returned it.

The prefix is load-bearing. It makes the two namespaces non-overlapping, so an id from one path can never be mistaken for an id from the other, and it makes the source visible in any log, database row, or bug report that carries an id.

`eventIndex` comes from splitting the same identifier on `-` and reading the second component, per ADR-022. The id and the ordinal come from one field, so a malformed id fails both at once rather than producing an event with a plausible-looking wrong index.

A prefixed id is not accepted by `client.event()`. `EventIdSchema` requires digits only, and an RPC id is not resolvable against the indexer, so passing one is rejected where the mistake is made.

### Alternatives considered

**Compose the id from `txHash` and a per-transaction index.** Rejected on evidence. RPC exposes no per-transaction event index, and the fields that look like one repeat across events.

**Use RPC's id unprefixed.** Rejected. It makes the two namespaces overlap for no benefit, and a consumer storing events from both paths cannot tell which is which.

**Give `DecodedEvent` a `source` field instead of a prefix.** Rejected for v0.1. It adds a field every consumer must check to a type where the id already answers the question, and an id passed around alone loses the tag while a prefix travels with it.

### Consequences

- Any consumer keying storage on `DecodedEvent.id` gets non-colliding keys across both paths without doing anything.
- An event fetched over RPC and later fetched from the indexer has two different ids. They describe the same on-chain event, and reconciling them means comparing `(ledger, eventIndex)`, not ids.
- The prefix is part of the wire contract. Changing it later invalidates stored keys, so it is fixed now.

---

## ADR-025: LiveEventQuery is a discriminated union, enforced at both layers

Date: 2026-08-28
Status: accepted

### Context

Stellar RPC's `getEvents` takes either a ledger range or a cursor, never both. ADR-013 verified this in the SDK's own declarations: `GetEventsRequest` is a discriminated union in which ledger-range mode declares `cursor?: never` and cursor mode declares `startLedger?: never`. Sending both is a protocol error.

`fetchLiveEvents` is the SDK's wrapper over that call, and it can either mirror the constraint or flatten it into an optional-everything object and hope callers get it right.

### Decision

`LiveEventQuery` is a discriminated union with the same shape as the protocol's, and the constraint is enforced twice.

```ts
type LiveEventQuery =
  | { startLedger: number; cursor?: never; limit?: number; filter?: EventFilter }
  | { cursor: string; startLedger?: never; limit?: number; filter?: EventFilter };
```

At compile time the `never` members reject both fields together and reject neither. At runtime a Zod union rejects the same two cases with `PulsarValidationError`, because a caller in plain JavaScript, or one passing a value parsed from JSON, reaches the same function with no type checking behind them.

Neither layer is redundant. The type catches the mistake where it is made and costs nothing at runtime; the schema catches what the type never saw.

Verified that the union does not fight ordinary consumer code. Reassigning `query = { cursor: page.cursor }` after a first page type-checks with no cast and no widening.

### Alternatives considered

**A flat object with both fields optional, validated only at runtime.** Rejected. It moves a mistake TypeScript can catch at the keyboard into an exception at request time, and it invites the shape the protocol rejects.

**Compile-time only, trusting the type.** Rejected. The SDK is published to JavaScript consumers too, and a query built from user input or a stored session is not type-checked by anything.

**Two separate methods, `fetchLiveEventsFrom` and `fetchLiveEventsAfter`.** Rejected. It doubles the surface and makes the ordinary paging loop switch functions between the first page and the rest, which is worse than switching a field.

### Consequences

- A caller who spreads a previous query forward, `{ ...query, cursor }`, gets a compile error, because `startLedger` survives the spread. The fix is to build the continuation fresh. This is the one ergonomic cost, and it fails loudly rather than silently sending an invalid request.
- Live RPC provides no exhaustion signal. `LiveEventsPage.cursor` is always a string, never null: a page carrying events returns the last event's id as its cursor, and an empty page returns a positional marker so paging can continue. A consumer paging manually with `fetchLiveEvents` owns the decision of when to stop, and `liveEventStream` polls indefinitely until the consumer breaks. Manufacturing an exhaustion signal, whether as a null cursor or a derived boolean, would have the SDK claim to know something about protocol state that only the protocol knows, and a consumer stopping on it would miss events arriving immediately after.

---

## ADR-026: DecodedEvent records whether its contract call succeeded

Date: 2026-08-28
Status: accepted

### Context

Every event in a `getEvents` response carries `inSuccessfulContractCall`. A reverted contract call still emits events, and they still land in the ledger. `DecodedEvent` had nowhere to put that flag, so an event from a call that failed was indistinguishable from one that committed.

This is not hypothetical. A five-event sample from live testnet contained one event with the flag false.

### Decision

`DecodedEvent` gains `inSuccessfulContractCall: boolean`, required rather than optional, populated from the wire on both paths. The name matches the RPC field so the two line up without a lookup.

Consumers choose their own filtering. The SDK reports what the ledger holds.

### Alternatives considered

**Filter the events out on the RPC path.** Rejected. It is silent data loss, and it makes the two paths disagree unless the indexer filters bug-for-bug identically. A consumer investigating a failed call would find the event missing rather than marked.

**Leave it out for v0.1.** Rejected. A consumer summing transfer amounts would over-count reverted transfers with nothing in the data to reveal it, and a decoder that omits this is a decoder that misreports what happened on chain. CLAUDE.md's rule that the indexer never trusts contract data points the same way.

### Consequences

- A wire contract change. Section 7.1's events table gains an `in_successful_contract_call` boolean column, `DecodedEventPayloadSchema` gains the snake_case field, and `pulsar-decoder` v0.2 must produce it.
- Consumer migration is additive: existing code keeps working and reads the new field only if it cares.
- `DecodedEvent.name` is relaxed from a non-empty string to any string in the same change. `eventNameFromTopics` already returns an empty string when an event's first topic is not a Symbol, so the schema and the decoder contradicted each other. An off-convention event now degrades to a nameless event with its topics intact, per ADR-023's rule that one odd value must not discard a page. Consumers wanting only convention-following events filter on `event.name !== ''`.

---

## ADR-027: buildContractCall takes an Account, copies it, and returns an unprepared transaction

Date: 2026-08-31
Status: accepted

### Context

Section 6 specifies `buildContractCall(contractId, method, args, invoker)` returning an unsigned `Transaction`. Verifying that against `@stellar/stellar-sdk` 16.2.0 before writing it surfaced three behaviours the spec does not address.

`TransactionBuilder` needs a sequence number, so an address alone cannot produce a transaction. `build()` then increments the sequence of whatever `Account` it is handed: building twice from one account yields sequence 101 and 102, and the caller's object is left at 102. And the transaction that comes out is unsimulated, carrying an empty Soroban footprint and only the 100 stroop base fee, so submitting it fails.

The SDK also accepts a non-`ScVal` argument without complaint. `contract.call('deposit', 'raw-string')` returns an operation carrying one argument and no error, so the malformed call surfaces at submission with nothing pointing back at the argument.

### Decision

`buildContractCall` takes an already-fetched `Account`, copies it internally, and returns an unsigned and unprepared `Transaction`. It stays synchronous and touches no network. Arguments are checked to be `ScVal` before assembly.

The copy is the load-bearing part. Without it, two calls built from one account claim two different sequence numbers, and a caller who submits only one has silently burned the other. That is the class of silent corruption this project rejects elsewhere, and the repo's immutability rule already forbids mutating a caller's object.

The required preparation step is documented on the function itself, with the full flow shown, rather than mentioned in passing.

### Alternatives considered

**Take an address and fetch the account.** Rejected. It turns a pure builder into an async network call, so a caller building three invocations pays three round trips they did not ask for, and an unreachable network becomes a failure mode of assembly.

**Accept either an address or an account.** Rejected. The return type becomes a promise even when nothing is fetched, and one function acquires two unrelated failure modes.

**Prepare the transaction before returning it.** Rejected. Simulation failures, which include contract reverts and insufficient balances, would then surface from a function whose name promises only assembly, and a caller could not tell those from an argument mistake.

**Ship `buildContractCall` and `prepareContractCall` separately.** Rejected. Two functions where the spec asked for one, and picking the wrong one fails at submission rather than at compile time.

### Consequences

- A caller fetches an account once and builds as many calls from it as they like, all claiming the same sequence number, which is what someone building a batch actually wants.
- Preparation is the caller's step and is visible in the function's own documentation.
- The argument check belongs to this SDK because the underlying one does not make it.
