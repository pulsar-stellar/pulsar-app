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
├── packages/
│   └── sdk/                  TypeScript SDK, Vitest, tsup dual build
├── apps/
│   └── web/                  Next.js 15 explorer, App Router
└── indexer/                  Go daemon, separate go.mod
```

`indexer/` is not a pnpm workspace member. The language boundary is a natural seam and the two toolchains stay independent, which is recorded in ADR-002.

## Status

Sprint 2, Phase A. The monorepo scaffold is landing. No sub-stack is implemented yet.

| Artifact | State |
|---|---|
| Workspace scaffold | in progress |
| `@pulsar-stellar/sdk` | not started, Phase B |
| Go indexer | not started, Phase D |
| Web explorer | not started, Phase F |

This repository depends on `pulsar-core` `v0.1.0-contracts`, which is deployed to Stellar testnet. Its contract ID is the showcase fixture every sub-stack here reads from, and it is recorded in `.env.example`.

## Prerequisites

| Tool | Version |
|---|---|
| Node.js | 20 LTS, pinned in `.nvmrc` |
| pnpm | 9 or newer |
| Go | 1.23 or newer, for the indexer only |

## Local development

```sh
nvm use            # picks up .nvmrc
pnpm install       # installs every TS workspace
cp .env.example .env.local
```

Per sub-stack commands land with each sub-stack. `.env.local` is never committed. Production values are set in the Vercel and Render dashboards.

## License

Apache-2.0. See [LICENSE](LICENSE).
