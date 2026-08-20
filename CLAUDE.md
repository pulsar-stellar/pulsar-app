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

Read these in order and treat them as authoritative:

1. `docs/planning/requirements.md` — project-wide standards. Where any rule elsewhere is more lenient than this file, this file wins. Where elsewhere is stricter, the stricter rule wins.
2. `docs/planning/system-prompt.md` — the build guide for this repo. Section 12 holds the numbered build sequence.
3. `docs/planning/roadmap-product.md` — product context across both repos.

Then read `.agent/context.md` for the state of the work, and `.agent/decisions.md` before proposing anything that contradicts a recorded decision.

## Current phase

Sprint 2, Phase A of the Section 12 build sequence: monorepo scaffold and context files, steps 1 through 15. Do not skip forward. Phase B, the SDK skeleton, starts at step 16.

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

Full detail in `docs/planning/requirements.md` and `CONTRIBUTING.md`.

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
