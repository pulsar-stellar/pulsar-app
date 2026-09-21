# Pulsar App

Pulsar Stellar is a toolkit for reading Soroban contract events. A project that
consumes its own contract's events on Stellar writes the same pieces each time:
an XDR decoder, an indexer to hold events past the seven-day RPC retention
window, and an API to query them. Pulsar Stellar provides those pieces once. A
Rust library decodes raw contract events into typed data, a Go daemon stores
them past the retention window, and a web explorer lets anyone paste a contract
ID and browse its decoded event history.

`pulsar-app` is the application layer of that toolkit. The Rust contract layer
lives in [`pulsar-stellar/pulsar-core`](https://github.com/pulsar-stellar/pulsar-core).

## What this repository holds

Three sub-stacks share this repository and ship on their own timelines:

- **`packages/sdk`**: the TypeScript client, published as `@pulsar-stellar/sdk`.
  It queries the indexer HTTP API, and reads live events straight from Soroban
  RPC when no indexer is available.
- **`indexer/`**: the Go daemon. It polls Soroban RPC, decodes each event, and
  stores it past the seven-day RPC retention window. It also carries the read
  API the SDK and the explorer are built against.
- **`apps/web`**: the Next.js explorer. Paste a contract ID, browse its decoded
  event history. Not landed yet, see Status.

Documentation is maintained in the separate `pulsar-stellar/pulsar-docs`
repository and publishes through GitBook.

## Monorepo map

```
pulsar-app/
├── package.json              workspace root
├── pnpm-workspace.yaml       members: packages/*, apps/*
├── tsconfig.base.json        strict config every TS workspace extends
├── .agent/                   long-term project memory, including the ADR log
├── .github/workflows/        one CI workflow per sub-stack
├── scripts/                  repo-wide checks CI runs
├── packages/
│   └── sdk/                  TypeScript SDK, Vitest, tsup dual build
├── indexer/                  Go daemon, separate go.mod
│   ├── cmd/pulsar-indexer/   binary entry point
│   ├── internal/             api, apierror, config, db, decoder, logger,
│   │                         models, rpc, store, validate, version
│   └── migrations/           per-engine SQL, one directory each
└── apps/                     not yet present, arrives with the explorer
```

Workspace members land in sequence as the build progresses, so a fresh clone
holds fewer directories than the finished layout. The status table below records
what exists at this commit.

`indexer/` is not a pnpm workspace member. The language boundary is a natural
seam and the two toolchains stay independent, recorded in ADR-002.

`migrations/` carries a directory per engine rather than one shared set. SQLite
accepts several Postgres declarations and then behaves differently, so a single
file would apply cleanly on both and silently corrupt one. See ADR-029.

## Status

Sprint 3. The SDK is released. The indexer runs as a daemon and stores events;
its HTTP API is written and tested but not yet served. The web explorer has not
landed.

| Artifact | State |
|---|---|
| Workspace scaffold | complete |
| `@pulsar-stellar/sdk` | released, `0.1.0` on npm, tag `v0.1.0-app` |
| Go indexer, ingestion | running: polls RPC, decodes, stores events |
| Go indexer, HTTP API | implemented and tested, not yet served |
| Web explorer | not in the tree, nothing deployed |

What the daemon does today: it loads its configuration, opens the database,
applies migrations, registers the bootstrap contracts, and runs one polling loop
per contract that decodes events and writes them to the store. Run it and the
events table fills. See [`indexer/cmd/pulsar-indexer/main.go`](indexer/cmd/pulsar-indexer/main.go).

What it does not do yet: serve HTTP. The read API in `internal/api` implements
`GET /health` and the `/contracts` collection (list, register, get, delete) with
tests, but the daemon does not mount it, so no route is reachable from a running
indexer. Event query routes are not built. Wiring the server into the daemon is
step 72; when it is mounted, the state-changing routes (`POST` and
`DELETE /contracts`) will be gated behind authentication and rate limiting. The
sequencing note is in `main.go` and `.agent/context.md`.

The web explorer has not landed. The workspace reserves `apps/*` for it, but
`apps/web` is not present at this commit and nothing is deployed.

This repository depends on `pulsar-core` `v0.1.0-contracts`, deployed to Stellar
testnet. Its showcase contract ID is the fixture every sub-stack here reads from,
recorded in `.env.example`.

## Related resources

- npm package: [`@pulsar-stellar/sdk`](https://www.npmjs.com/package/@pulsar-stellar/sdk)
- Release tag: [`v0.1.0-app`](https://github.com/pulsar-stellar/pulsar-app/releases/tag/v0.1.0-app)
- Rust contract layer: [`pulsar-stellar/pulsar-core`](https://github.com/pulsar-stellar/pulsar-core)
- Decision log (ADRs): [`.agent/decisions.md`](.agent/decisions.md)

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
| Node.js | 22 LTS, pinned in `.nvmrc`, see ADR-012 |
| pnpm | 11 or newer |
| Go | 1.23 or newer, for the indexer only |

`indexer/go.mod` pins the Go 1.26.7 toolchain. A distribution Go from 1.23 acts
as a bootstrap and downloads that toolchain on first build, so an older
distribution Go needs no manual upgrade. See ADR-030.

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

## Contributing

`CONTRIBUTING.md` carries the full standard: setup, commit rules, test
discipline, and the code rules per sub-stack. The workflow in effect:

1. **Open an issue first.** Substantive work is tracked by a GitHub issue that
   states the scope, the acceptance criteria, and the ADR or specification
   section that governs it. The PR closes it with `Closes #NN`.
2. **Branch from `main`, one logical unit per branch.** Name it for the unit,
   for example `step-68-events-handlers`. Do not push to `main` directly.
3. **Commit with discipline.** One commit per logical unit, a conventional
   `type(scope): description` subject, and a body that says what changed and
   why. Stage exact paths, never `git add .`.
4. **Push the branch and open a PR.** The description says what changed and how
   it was verified. It is detailed, not a one-line summary.
5. **CI must be green before review.** Three workflows gate a PR: `ci-ts.yml`,
   `ci-go.yml`, and `ci-web.yml`. A PR that changes behavior without changing
   tests is sent back.
6. **A second maintainer reviews and merges.** The author does not merge their
   own PR.

Small or urgent corrections may go to `main` directly at a maintainer's
discretion. Changes to the indexer's HTTP surface, its database layer, or its
migrations carry a higher review bar, because that surface is publicly reachable
and writes data every downstream consumer reads.

Report a security issue privately per `SECURITY.md`, not in a public issue.

## License

Apache-2.0. See [LICENSE](LICENSE).
