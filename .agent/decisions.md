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
