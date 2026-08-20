# Pulsar App

Application layer of the Pulsar Stellar toolkit: a TypeScript SDK, a Go indexer daemon, and a Next.js explorer for Soroban contract events.

## Repo role

This repo is the app layer. The sibling repo `pulsar-stellar/pulsar-core` is the Rust contract layer, and `pulsar-stellar/pulsar-docs` is the GitBook documentation source. Documentation does not live here.

## Sub-stacks

| Path | Stack | Purpose |
|---|---|---|
| `packages/sdk` | TypeScript, published as `@pulsar-stellar/sdk` | Client for the indexer API and for live RPC reads |
| `indexer/` | Go, separate `go.mod` | Polls RPC, decodes events, stores them, serves HTTP |
| `apps/web` | Next.js 15, App Router | Public explorer |

`indexer/` is not a pnpm workspace member. See ADR-002.

## Read before doing anything

Everything authoritative is tracked. Read in this order:

1. `CONTRIBUTING.md` — setup, commit rules, test discipline, and the code rules per sub-stack. This is the standard your work is measured against.
2. `.agent/context.md` — the state of the work, what is done, and what the next phase is gated on.
3. `.agent/decisions.md` — the ADR log. Read it before proposing anything that contradicts a recorded decision. It is append-only.
4. `.agent/glossary.md` — the domain terms used throughout.

`docs/planning/` is not tracked. It holds maintainer-local drafts, including the system prompt whose section 12 carries the numbered build sequence, and it is absent from a fresh clone. See `docs/planning/README.md`, which is the only tracked file there. Where a draft and a tracked file disagree, the tracked file wins, and a decision worth keeping is moved into the ADR log rather than left in a draft.

## Current phase

Sprint 2. Phase A of the Section 12 build sequence, steps 1 through 15, is complete. Phase B, the SDK skeleton, starts at step 16 and is gated: verify the `@stellar/stellar-sdk` version line, the bindings generation command, and the `getEvents` shape against upstream documentation first, and record the findings in an ADR. See ADR-011. Do not skip forward past step 19.

Node 22 LTS and pnpm 11 are the pinned toolchain, superseding Section 3.1's Node 20 row. See ADR-012.

## Depends on pulsar-core

`pulsar-core` `v0.1.0-contracts` is released and deployed to testnet. Its showcase contract ID is `CDNWTVUDKCCGW7GOC6SBLUFXXUCD2YDHWRDUSXZ6CYBQKQWLCUYYWI5L`, recorded in `.env.example` as `NEXT_PUBLIC_SHOWCASE_CONTRACT_ID` and `PULSAR_INDEXER_BOOTSTRAP_CONTRACTS`, and in `.agent/context.md`.

## Skills to load and cite

Restate which of these are active before every task:

- `humanizer`
- `frontend-patterns`
- `coding-standards`
- `tdd-workflow`
- `blueprint`
- `security-review`

`security-review` is mandatory for every commit touching the indexer HTTP surface, database migrations, or auth. `frontend-patterns` and the built-in `frontend-design` skill are mandatory for every commit touching UI. `tdd-workflow` is mandatory for every SDK method and every indexer handler.

## Non-negotiables

Full detail in `CONTRIBUTING.md`.

- No em dashes anywhere in any output
- TypeScript strict, no `any`, no `@ts-ignore` without an ADR
- Go: every error wrapped with `%w`, every `ctx context.Context` passed through, no panic in request handling
- React: App Router only, no inline styles, Tailwind and shadcn/ui only
- SQL: every query parameterized, no string concatenation ever
- The indexer never trusts contract data. Validate every event before insert
- One commit per logical unit, push after every commit, never `git add .`
- Every behavior-carrying change paired with tests
- Every env var referenced in code appears in `.env.example`
- Secrets never in the repo
- Verify an external API's current signature before writing against it
- Halt and ask on ambiguity, never guess
