# @pulsar-stellar/web

The Pulsar explorer: a public web app for browsing decoded Soroban contract
events. Paste a contract ID and read every event it has emitted, decoded and
searchable, served by a Pulsar indexer.

This is the third piece of the Pulsar Stellar toolkit, alongside the TypeScript
SDK (`packages/sdk`) and the Go indexer (`indexer/`). The SDK is the typed
client, the indexer stores and serves events, and this app is the explorer a
person opens in a browser.

## Stack

- Next.js 15, App Router, Server Components by default
- TypeScript strict, extending the workspace `tsconfig.base.json`
- Tailwind CSS v4 with shadcn/ui design tokens
- Vitest and Testing Library for component and unit tests

## Run it locally

From the repository root:

```sh
nvm use            # Node 22, per .nvmrc
corepack enable    # provides pnpm
pnpm install       # installs every workspace, this app included
```

Copy the environment template and point the app at an indexer:

```sh
cp .env.example .env.local
```

The explorer reads `NEXT_PUBLIC_PULSAR_INDEXER_URL`, which defaults to
`http://localhost:8080`, the indexer's local address. See the root
`.env.example` for the full list of variables and the indexer's own README for
running one locally.

Then, from this directory:

```sh
pnpm --filter ./apps/web dev        # start the dev server on http://localhost:3000
pnpm --filter ./apps/web lint
pnpm --filter ./apps/web typecheck
pnpm --filter ./apps/web test
pnpm --filter ./apps/web build
```

## Layout

```
src/
  app/         App Router routes, layouts, and global styles
  components/  Presentational and interactive UI
  lib/         Framework-agnostic helpers
tests/         Component and unit tests
```
