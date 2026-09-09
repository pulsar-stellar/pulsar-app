# Pulsar Stellar — Hardening and Submission Roadmap

**Repository home**: `pulsar-stellar/pulsar-app/docs/roadmap-hardening.md`
**Owner**: Emedit
**Related documents**: [`docs/roadmap-product.md`](./roadmap-product.md) is the master execution roadmap and stays authoritative for sprint numbering and the product path. This document is a focused epic that sits under it, covering the hardening, deployment, and submission work that turns the built toolkit into a submission-grade open-source project. [`docs/requirements.md`](./requirements.md) is the standard each workstream is measured against, and [`.agent/decisions.md`](../.agent/decisions.md) records the reasoning behind what ships. The decision to adopt this roadmap is ADR-037.
**Last updated**: 2026-09-09

---

## How to read this

This roadmap is organized as one epic of ordered workstreams, in the shape the reviewer-facing GrantFox OSS epics use: every workstream states an objective, the problem it closes, its scope, an implementation outline, acceptance criteria, and how it is tested. It exists so the work beyond the core build sequence is legible to a stranger and verifiable against explicit criteria, rather than living as a loose to-do list.

Two rules govern everything below.

**Pulsar stays a read-only toolkit.** The indexer polls, decodes, stores, and serves contract events. It never submits a transaction and never holds funds. This is a recorded non-negotiable (see `CLAUDE.md` and ADR-026's trust boundary), and every workstream here respects it. Where "payment systems" appear, they appear as event shapes to decode and serve, never as a money path to operate. This is the deliberate contrast with the IndigoPay epic that inspired the structure: that project hardens a money path it owns; Pulsar hardens the read path over money movements other contracts own.

**This roadmap extends the tracked docs, it does not replace them.** Sprint numbering, estimates, and the v0.1 → v0.2 → v1.0 milestone track live in `roadmap-product.md`. Directional decisions land as ADRs. Cross-repo items land in `docs/planning/cross-repo-updates.md` and then in the far repo's own docs. Nothing here forks the project's identity; it sharpens the finish.

---

## Where this sits in the build sequence

The numbered build sequence (system-prompt §12, Phase 7) runs steps 39 through 73 for the indexer and continues into the web explorer, docs, and release. As of 2026-09-09 the indexer is mid-Phase-F: the RPC client, the poller, and poller startup are done (steps 59-61 and the poller slice of step 72). The HTTP surface (steps 62-71), the rest of step 72, and the README (step 73) remain.

This roadmap does not renumber or replace that sequence. Workstream H1 below **is** the remaining indexer build sequence, restated in epic form so its acceptance criteria are explicit. Workstreams H2 through H8 are the hardening and submission layer that `roadmap-product.md` already sketches across Sprints 8 through 10; this document gives them acceptance criteria and testing plans they did not have.

---

## Epic: Toolkit hardening and Drips Wave submission readiness

The unifying property: *a first-time visitor can reach the live explorer, paste the showcase contract ID, and see correctly decoded events; a first-time contributor can clone either repo, run it green locally, and pick up a scoped issue; and a reviewer can verify every claim the submission makes from a public link.* Each workstream closes one gap between the current state and that property.

The workstreams are ordered so each builds on the last. H1 finishes the serving path the whole product depends on. H2 makes its failures legible. H3 through H5 make it deployable, observable, and reproducible. H6 makes the payment-event coverage a headline capability. H7 makes both repos contributor-ready. H8 assembles the submission.

---

### Workstream H1 — Finish the indexer serving path: HTTP surface, atomic startup, README

**Objective.** Complete the indexer so a running `pulsar-indexer` binary serves decoded events over HTTP in exactly the shape the SDK's wire contracts require, alongside the poller that is already draining events into the store.

**Problem.** The poller and its startup wiring are done, but the indexer cannot yet be consumed: there is no HTTP surface. The SDK (`@pulsar-stellar/sdk@0.1.0`) is published against wire contracts (ADR-017 envelope, ADR-019 not-found, ADR-021 events query, ADR-022 event ordinal) that nothing currently serves. Until the handlers exist, the explorer cannot render and the end-to-end claim cannot be made.

**Scope.**
- `indexer/internal/api/` — HTTP handlers for the contract and event endpoints the SDK calls.
- `indexer/cmd/pulsar-indexer/main.go` — wire the API server alongside the poller under one shared context and graceful-shutdown path (the rest of step 72).
- `indexer/README.md` — run instructions, env var table, endpoint reference (step 73).

**Implementation outline.**
1. Handlers returning the ADR-017 response envelope on every path, success and error alike, with the ADR-019 `not_found` envelope and a 404 for an absent contract.
2. The events query contract (ADR-021): pagination by cursor, filter by name/ledger-range/topic-value, the empty page reaching the wire as `[]` not `null`.
3. Every event carries `in_successful_contract_call` (ADR-026) and `event_index` as the ledger-wide ordinal (ADR-022).
4. The server shares the poller's root context: one SIGINT/SIGTERM cancels both, `http.Server.Shutdown` drains in-flight requests, and `main` waits on both before exiting.
5. No endpoint is exposed without stating its auth posture in the README; the read API is intentionally unauthenticated and that choice is documented, not silent.

**Acceptance criteria.**
- Every SDK method (`ping`, `registerContract`, `getContract`, `listContracts`, `events`, `event`) has a handler that returns the exact envelope the SDK's Zod schema validates.
- An absent contract returns HTTP 404 with a `not_found` envelope; an empty list returns `[]`.
- Malformed query parameters return a 400 with a structured error code (see H2), never a 500.
- `go test ./...` green in `indexer/`, race detector clean, coverage at or above the 80% floor.
- A local run indexes the showcase contract and serves its events over HTTP within 60 seconds of a poll.

**Testing.**
- Unit: each handler against a store-backed in-memory SQLite DB, asserting status code and envelope shape.
- Integration: table-driven request/response cases covering pagination, every filter, not-found, and bad-input paths.
- The `tdd-workflow` skill is mandatory for every handler (a failing test first, per `CLAUDE.md`).

---

### Workstream H2 — Structured error-code catalog across the toolkit

**Objective.** Give every failure the toolkit can return a stable, documented, machine-readable code, so consumers branch on codes rather than parsing prose, and so the error surface is a documented contract rather than an accident of implementation.

**Problem.** Errors today are wrapped Go errors and thrown SDK exceptions with human-readable messages. There is no stable identifier a consumer can switch on, no catalog a reviewer can read, and no guarantee a message string will not change. ADR-034 already established for one case that classification must be by code, not message text; this workstream generalizes that principle into a catalog. The "250 error codes" ask is read as *comprehensive, aligned coverage* in this template's shape, not a literal count to pad to.

**Scope.**
- `indexer/internal/apierror/` — the Go error-code registry and the mapping to HTTP status.
- `packages/sdk` — a typed `PulsarError` carrying the code, surfaced from every method.
- `docs/error-codes.md` — the catalog: code, meaning, HTTP status, when it fires, what a consumer should do.
- Codes are also relevant to `pulsar-core`'s `contracterror` enum; that alignment is a cross-repo item (see below), not built here.

**Implementation outline.**
1. Define codes as stable UPPER_SNAKE strings grouped by domain: `VALIDATION_*` (bad input), `NOT_FOUND_*` (absence), `RANGE_*` (RPC window, per ADR-036), `RPC_*` (upstream transport), `STORE_*` (persistence), `DECODE_*` (event decode), `CONFIG_*` (startup). The count follows the real surface; the goal is that every failure path maps to exactly one code.
2. Each code carries a fixed HTTP status and a one-line consumer-facing meaning. The envelope from H1 carries `{ code, message }` where `message` may change but `code` may not.
3. The SDK maps each wire code to a `PulsarError` subtype so TypeScript consumers get compile-time-known members.
4. `docs/error-codes.md` is generated or checked against the registry in CI, so the catalog cannot drift from the code (the same discipline the SDK README extraction uses as a build gate).

**Acceptance criteria.**
- Every error path in the indexer HTTP surface returns a registered code; a CI test fails if a handler returns an unregistered one.
- The SDK exposes the code on every error it throws, typed.
- `docs/error-codes.md` lists every code with meaning, status, and remedy, and a CI check proves it matches the registry.
- No code's string identifier changes without an ADR (they are a wire contract, like ADR-023's union).

**Testing.**
- Unit: registry completeness (every defined code has status + doc entry), and the handler-to-code mapping.
- Integration: assert the wire `code` for each failure scenario in H1's test matrix.
- SDK: assert each thrown `PulsarError` carries the expected code member.

---

### Workstream H3 — Deployment: live explorer and indexer, secret hygiene, teardown

**Objective.** Put the toolkit on the public internet: the explorer on Vercel, the indexer reachable, both pointing at testnet, with a fresh deployment whose secrets are managed correctly and whose old/stale deployment is removed.

**Problem.** Nothing is deployed on the app side yet (`roadmap-product.md` §Now). The submission needs live URLs. An earlier/old Vercel deployment, if present, must not linger with a stale build or a leaked token; the ask explicitly includes rotating to a new Vercel secret, updating the project and README with the new link, and deleting the old deployment.

**Scope.**
- `apps/web` — production build config, environment variables, Vercel project.
- `indexer/` — deployment target (Render or equivalent per `roadmap-product.md` Sprint 8) with its own env config.
- Secret management: the new Vercel token lives in the deploy environment and in the local `.env.local`, never in the repo. `.env.example` gains any new variable name (value blank), per the non-negotiable that every referenced env var appears there.
- README and `.agent/context.md`: the new live URLs.

**Implementation outline.**
1. Create the Vercel project against `apps/web`, set env vars (including `NEXT_PUBLIC_SHOWCASE_CONTRACT_ID` and the indexer base URL) in the Vercel dashboard, not in the repo.
2. Rotate the Vercel access token: generate a new one, store it in the deploy secret store, and revoke the old one. Treat any token that ever touched the repo or a log as compromised and rotate it (global security rule).
3. Deploy the indexer to its backend host with testnet RPC config and a managed database.
4. Delete the old Vercel deployment/project once the new one serves the current build at the new URL.
5. Update `README.md`, `roadmap-product.md`, and `.agent/context.md` with the new URLs.

**Acceptance criteria.**
- The explorer is reachable at a public URL serving the current build.
- The indexer is reachable, polling testnet, with the showcase contract indexed.
- Pasting the showcase contract ID on the live explorer shows decoded events, zero manual setup.
- No secret appears in the repo or in any commit; `.env.example` names every variable the code reads.
- The old Vercel deployment no longer resolves; the old token is revoked.

**Testing.**
- A post-deploy smoke check (scripted): hit the indexer health endpoint and the explorer's contract route, assert 200 and a decoded event in the payload.
- A secret-scan pass (the existing env-parity script plus a secrets check) is green before the deploy commit.
- Verify the old URL returns 404/NXDOMAIN after teardown.

> Security note: this workstream touches production and secrets. It is high-risk under the safety rules. Each irreversible step (token revocation, old-deployment deletion) is confirmed with the maintainer before execution, and the `security-review` skill is loaded for the deploy commit.

---

### Workstream H4 — CI/CD: deployment as a pipeline stage

**Objective.** Make deployment a reproducible pipeline step rather than a manual act, so every merge to `main` that passes checks can ship, and so the deploy is auditable.

**Problem.** Deployment done by hand is unrepeatable and easy to get wrong under pressure (wrong env, stale build, forgotten migration). The ask is to add Vercel deployment to CI/CD. The indexer's backend deploy belongs in the same pipeline.

**Scope.**
- `.github/workflows/` — a deploy workflow gated on the existing test/lint/build jobs.
- Deploy secrets stored as GitHub Actions secrets (the rotated Vercel token from H3), never inline.

**Implementation outline.**
1. A `deploy-web` job that runs only on `main` after `ci-web` passes, using the Vercel token from Actions secrets.
2. A `deploy-indexer` job gated on `ci-go`, deploying the backend and running migrations against the managed database as a release step.
3. Both jobs are idempotent and log the deployed URL as a job summary.
4. Branch protection (H7) requires the CI jobs, so nothing ships without green checks.

**Acceptance criteria.**
- A merge to `main` triggers a deploy only after all test/build jobs pass.
- The Vercel token is read from Actions secrets; it appears in no workflow file or log.
- A failed deploy fails the workflow loudly and does not leave a half-updated environment.
- The workflow file names match the branch-protection required checks exactly.

**Testing.**
- A dry-run of the workflow on a branch (deploy step guarded to no-op off `main`) proving the gating order.
- Verify the job summary shows the correct URL after a real `main` deploy.

---

### Workstream H5 — Reproducible local run and observability

**Objective.** Let anyone bring the whole toolkit up locally with one command, and let an operator see what the indexer is doing in production.

**Problem.** `roadmap-product.md` Sprint 8 calls for `docker-compose up` running indexer + Postgres, and the Drips issue-area list calls for a Prometheus metrics endpoint. The Postgres path has never actually been executed (`context.md` open gap), which is a standing risk this workstream is the place to close where Docker is available.

**Scope.**
- `docker-compose.yml`, `scripts/setup.sh`, `scripts/dev.sh`.
- `indexer/internal/api` — a `/metrics` endpoint and a `/health` endpoint.
- The Postgres migration path, finally executed against a real server.

**Implementation outline.**
1. `docker-compose` brings up Postgres and the indexer wired to it, applying migrations on start.
2. Run `0001_init` and `0002_events_index` against real Postgres and record the result, closing the open gap honestly (either it applies, recorded as verified, or it does not, recorded as a bug).
3. A Prometheus `/metrics` endpoint exposing poll counts, decode failures, events indexed, and RPC error counts by code (reusing H2's codes as label values).
4. A `/health` endpoint reporting DB reachability and last-poll recency.

**Acceptance criteria.**
- `docker-compose up` yields a working indexer against Postgres with the schema applied.
- The Postgres migration path is executed and its result recorded in `context.md`, replacing "unproven".
- `/metrics` serves Prometheus-format counters; `/health` reports honest status.
- Both SQLite and Postgres pass the same store test suite (existing exit criterion, now actually run for Postgres).

**Testing.**
- The store test suite run against a Postgres container in CI (testcontainers-go where the runner has Docker).
- A metrics assertion test: drive a poll, scrape `/metrics`, assert the counters moved.

---

### Workstream H6 — Payment-event coverage as a first-class capability

**Objective.** Make "decodes the payment flows people actually care about" a headline capability by covering the common Stellar payment and asset event shapes as richly typed, well-documented feeds. Read-only throughout: these are shapes to decode, never transactions to submit.

**Problem.** The showcase contract exercises primitive event shapes. The payments people track on Stellar are SEP-41 token transfers/mints/burns, path payments, claimable balances, and anchor flows (SEP-24/31). Covering them, with fixtures and docs, is what turns "an event indexer" into "the tool I reach for to watch token and payment activity." The user's "multiple payment systems" ask maps here, within the read-only boundary.

**Scope.**
- `indexer/internal/decoder` and `packages/sdk` decoder: confirm every SEP-41 event variant decodes with no special-casing (proving conformance), add fixtures for path-payment and claimable-balance event shapes.
- `docs/` — a "decoding payments on Stellar" guide keyed to real testnet events.
- Cross-repo: the SEP-41 token showcase is a `pulsar-core` deliverable (its roadmap's v0.3.0-contracts). This workstream consumes it as fixtures; it does not build the contract here.

**Implementation outline.**
1. Enumerate the target event shapes: SEP-41 (`transfer`, `mint`, `burn`, `approve`, `clawback`, `set_admin`, `set_authorized`), path payment, claimable balance create/claim.
2. For each, capture a real testnet event as a fixture (real provenance, never synthesized, per the authenticity rule) and assert the decoded shape.
3. Prove SEP-41 decodes through the same generic path as the showcase events, with no shape-specific branches, which is the conformance claim.
4. Document each shape in the payments guide with the decoded JSON a consumer receives.

**Acceptance criteria.**
- Every enumerated payment event shape has a fixture and a decode assertion in both the Go and TS decoders.
- No payment-shape-specific branch exists in the decoder; coverage is proven through the generic path.
- The payments guide shows, for each shape, the exact decoded output.
- Client-side, user-signed payment *tooling* (build-and-submit helpers with no custody) is recorded as a future-horizon item with a trigger, not built here.

**Testing.**
- Fixture-based decode tests for every shape, in both decoders, asserting exact output.
- A conformance test asserting SEP-41 events need no special-case code.

---

### Workstream H7 — Contributor readiness and repo hygiene (both repos)

**Objective.** Make both repos look and behave like a real open-source project a stranger could contribute to today: hygiene files, branch protection, a scoped issue backlog, and code quality held to a documented standard.

**Problem.** `roadmap-product.md` Sprint 9 specifies the hygiene checklist and a 40-65 issue backlog; this workstream carries it with acceptance criteria. The "boost code quality to open-source standard" and "capacity to accommodate many open-source issues / a tooling system" asks land here: the codebase must be modular enough that scoped issues do not collide, and the quality bar must be explicit.

**Scope.**
- Per repo: `CONTRIBUTING.md`, `SECURITY.md`, README rewrite to the approved-repo pattern (badges, maintainer table, architecture, quick-start, contributing, contributors), GitHub topics, issue/PR templates, branch protection on `main`.
- `apps/…`: `scripts/create-wave-issues.sh` batch-creating the labeled issue backlog.
- Code-quality pass: run `code-reviewer`, `security-reviewer`, and the language linters (`go vet`, `staticcheck`, `gosec`; the TS/React equivalents) across the tree; fix criticals/highs.

**Implementation outline.**
1. Author the hygiene files against the pattern of currently-approved Drips Wave Stellar repos (verify the current pattern by fetching a few top-approved repos first, per the roadmap's own instruction, rather than assuming).
2. Enable branch protection whose required checks match the actual workflow job names.
3. Generate the issue backlog grouped by area (SDK, indexer, web, docs), each issue with a commit-style title, a complexity label, a type label, and a body with Summary / Acceptance Criteria checkboxes / Tech Stack.
4. Run the review agents; address findings; record any deliberate non-fix as an ADR.

**Acceptance criteria.**
- Both repos pass the hygiene checklist in `roadmap-product.md` Sprint 9.
- Branch protection is on, required checks match workflow names, no direct pushes to `main`.
- 40-65 well-scoped, labeled issues exist across the two repos.
- The review-agent pass leaves no unaddressed critical or high finding; each deliberate exception has an ADR.

**Testing.**
- A checklist verification (scriptable via `gh api`) that protection, templates, and topics are set.
- CI green across both repos on the hygiene commits.

---

### Workstream H8 — Submission assembly

**Objective.** Assemble a Drips Wave / GrantFox submission that maximizes approval odds by giving reviewers every verifying link and a clear, honest project narrative.

**Problem.** `roadmap-product.md` Sprint 10 lists the submission ceremony; the user's ask emphasizes that the submission's link section should carry every credible link (GitHub repos, live Vercel app, docs, contract verification) because a reviewer approves what they can verify. The IndigoPay epic's `GrantFox OSS` labeling and evidence-first PR body are the model for how thoroughly claims are backed.

**Scope.**
- The submission form content and the assets it references: live app URL, docs URL, both repo URLs, Stellar Expert contract link, demo video, repo-relationship paragraph, planned-issues description, project description.

**Implementation outline.**
1. Collect every link H3 and H7 produced: explorer, docs, both repos, the deployed contract on Stellar Expert.
2. Record a 60-120 second real-narration demo of the full flow (paste ID → events → expand → filter → export).
3. Write the project description: problem, mechanism, who benefits, real scale only if a credible figure exists (no fabricated numbers, per the authenticity rule).
4. Populate the submission's link section with every verifying URL, since approval tracks verifiability.

**Acceptance criteria.**
- Every claim in the submission is backed by a public link a reviewer can open.
- The link section carries: both repos, live app, docs, contract verification, demo video.
- The description states problem + mechanism + beneficiary honestly, with no fabricated metrics.
- The submission is assembled and ready before the form is opened (no scrambling mid-form).

**Testing.**
- A link-check pass: every URL in the submission resolves and shows what it claims.
- A dry read-through against the approved-repo submission pattern.

---

## Cross-repo deliverables (land in pulsar-core, not here)

These items were surfaced in the requirements but belong to the `pulsar-core` repo, which is not in this workspace. They are recorded here and queued in [`docs/planning/cross-repo-updates.md`](./planning/cross-repo-updates.md) so they land in `pulsar-core`'s own roadmap and ADR log on next touch, rather than being built in the app repo.

- **Smart-contract gas optimization and benchmarking.** The `pulsar-showcase` contract is a decoder fixture, not production money code, so this is a documentation-and-credibility exercise: optimize the contract for low gas where it does not hurt clarity, benchmark gas per public function, and publish a gas-per-call table in the `pulsar-core` README. This belongs in `pulsar-core`'s release roadmap (alongside its v0.3.0 SEP-41 token showcase, which is the more realistic gas subject). Recorded as a cross-repo item; not an app-repo deliverable.
- **SEP-41 token showcase contract.** The fixtures Workstream H6 consumes come from `pulsar-core`'s planned `v0.3.0-contracts`. H6 depends on it but does not build it.
- **Error-code alignment with `contracterror`.** The catalog in H2 should align its `DECODE_*` and contract-facing codes with `pulsar-core`'s `contracterror` enum, a coordinated change like ADR-023's union.

---

## Sequencing and dependencies

```
H1 (HTTP surface) ─┬─> H2 (error codes) ─┬─> H3 (deploy) ─> H4 (CI/CD deploy)
                   │                      └─> H5 (compose + metrics + Postgres)
                   └─> H6 (payment events, parallel; needs pulsar-core SEP-41 fixtures)

H3 + H7 (hygiene, both repos) ──> H8 (submission)
```

H1 gates everything downstream: nothing deploys or is documented until the serving path exists. H2 is next because deploy, metrics, and the SDK all consume its codes. H3 and H5 can proceed together once H2 lands; H4 follows H3 (it needs the rotated token and a working manual deploy first). H6 runs in parallel but is gated on `pulsar-core` shipping the SEP-41 fixtures. H7 can start any time and must finish before H8. H8 is last and assembles what the others produced.

---

## Definition of done for this epic

- A first-time visitor reaches the live explorer, pastes the showcase contract ID, and sees decoded events with zero setup.
- A first-time contributor clones either repo, brings it up green locally (SQLite or Postgres via compose), and finds a scoped issue to pick up.
- Every failure the toolkit returns carries a documented, stable code.
- Every submission claim is backed by a public, resolving link.
- No secret is in the repo; the old deployment is gone; the rotated token is the only one live.
- Both repos pass the hygiene checklist and CI is green on `main`.

---

## Changelog

- **2026-09-09**: Roadmap drafted. Adopts the GrantFox OSS epic structure (objective / problem / scope / implementation / acceptance / testing per workstream) modeled on Stellar-IndigoPay issue #1098, within Pulsar's read-only boundary. Extends `roadmap-product.md` rather than replacing it. Adoption recorded as ADR-037. Payment scope resolved to read-only event decoding (H6); "250 error codes" resolved to a structured aligned catalog (H2); gas optimization recorded as a `pulsar-core` cross-repo deliverable.

---

**End of hardening roadmap.**
