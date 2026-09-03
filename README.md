# Pulsar App

Pulsar Stellar is a developer toolkit for Soroban contract events. Every Stellar project that needs to consume its contract's events today writes the same plumbing from scratch: XDR decoders, indexer glue, custom APIs. Pulsar Stellar provides three shared building blocks so they don't have to. A Rust library that turns raw contract events into typed data, a Go daemon that stores historical events past the seven-day RPC retention window, and a web explorer where anyone can paste a contract ID and browse every event that contract has ever emitted, decoded and searchable. It serves Soroban dapp builders, backend engineers integrating with existing protocols, and auditors reviewing contract behavior post-deployment.

## What this repository holds

`pulsar-app` is the application layer. It carries three sub-stacks that ship independently but share one repository:

- **`packages/sdk`**: the TypeScript client, published as `@pulsar-stellar/sdk`. Queries the indexer, and reads live events straight from Soroban RPC when no indexer is running.
- **`indexer/`**: the Go daemon. Polls Soroban RPC, decodes what it finds, and stores it past the seven-day RPC retention window. Serves the HTTP API the SDK and the explorer both read.
- **`apps/web`**: the Next.js explorer. Paste a contract ID, browse its decoded event history.

The sibling repository `pulsar-stellar/pulsar-core` carries the Rust contract layer: the `pulsar-showcase` reference contract and the `pulsar-decoder` crate. Documentation lives in `pulsar-stellar/pulsar-docs` and publishes through GitBook.

## Monorepo map

```
pulsar-app/
├── package.json              workspace root
├── pnpm-workspace.yaml       members: packages/*, apps/*
├── tsconfig.base.json        strict config every TS workspace extends
├── .agent/                   long-term project memory for future sessions
├── .github/workflows/        one CI workflow per sub-stack
├── scripts/                  repo-wide checks CI runs
├── packages/
│   └── sdk/                  TypeScript SDK, Vitest, tsup dual build
├── indexer/                  Go daemon, separate go.mod
│   ├── cmd/pulsar-indexer/   binary entry point
│   ├── internal/             config, logger, db, models, store
│   └── migrations/           per-engine SQL, one directory each
└── apps/                     not yet present, arrives with the explorer
```

Workspace members land in sequence as the build progresses, so a fresh clone
holds fewer directories than the finished layout. The status table below records
what exists at this commit.

`indexer/` is not a pnpm workspace member. The language boundary is a natural seam and the two toolchains stay independent, which is recorded in ADR-002.

`migrations/` carries a directory per engine rather than one shared set. SQLite accepts several Postgres declarations and then behaves differently, so a single file would apply cleanly on both and silently corrupt one. See ADR-029.

## Status

Sprint 3. The SDK is released and the Go indexer is under construction.

| Artifact | State |
|---|---|
| Workspace scaffold | complete |
| `@pulsar-stellar/sdk` | released, `0.1.0` on npm |
| Go indexer | in progress, storage layer done |
| Web explorer | not started |

Sprint 2 closed at `v0.1.0-app` with the SDK published. Sprint 3 is building the
indexer from the bottom up: configuration, structured logging, both database
drivers, the migration runner and schema, the row models, and the contract and
event stores are in. The binary starts and shuts down cleanly but does not yet
poll RPC or serve HTTP, so there is nothing to run end to end.

This repository depends on `pulsar-core` `v0.1.0-contracts`, which is deployed to Stellar testnet. Its contract ID is the showcase fixture every sub-stack here reads from, and it is recorded in `.env.example`.

## Using the SDK

```sh
pnpm add @pulsar-stellar/sdk
```

It reads from a running indexer, and falls back to live Soroban RPC when there
is none. See [`packages/sdk/README.md`](packages/sdk/README.md) for the client
surface and worked examples.

## Prerequisites

| Tool | Version |
|---|---|
| Node.js | 22 LTS, pinned in `.nvmrc` |
| pnpm | 11 or newer |
| Go | 1.21 or newer, for the indexer only |

`indexer/go.mod` pins Go 1.26.7. Any Go from 1.21 acts as a bootstrap and
downloads that toolchain on first build, so a distribution Go older than the pin
needs no manual upgrade. See ADR-030.

## Local development

```sh
nvm use                        # picks up .nvmrc
corepack enable                # provides pnpm
pnpm install                   # installs every TS workspace
cp .env.example .env.local
```

TypeScript workspaces, from the repository root:

```sh
pnpm lint
pnpm typecheck
pnpm test
pnpm build
```

The Go indexer, from `indexer/`:

```sh
go vet ./...
go test -race ./...
go build ./...
```

Repo-wide checks, which CI also runs:

```sh
./scripts/verify-env-parity.sh        # every env var in code is in .env.example
./scripts/verify-env-parity.test.sh   # and that check itself still catches drift
```

`.env.example` copies to a working local configuration as it stands, defaulting
to SQLite so the indexer needs no database to set up. `.env.local` is never
committed. Production values are set in the Vercel and Render dashboards.

## License

Apache-2.0. See [LICENSE](LICENSE).
