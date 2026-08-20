# Context: pulsar-app

Onboarding for a fresh session, AI or human. Read this before touching code. Updated at phase transitions, not at every commit.

## 1. What Pulsar Stellar is

Pulsar Stellar is a developer toolkit for Soroban contract events. Every Stellar project that needs to consume its contract's events today writes the same plumbing from scratch: XDR decoders, indexer glue, custom APIs. Pulsar Stellar provides three shared building blocks so they don't have to. A Rust library that turns raw contract events into typed data, a Go daemon that stores historical events past the seven-day RPC retention window, and a web explorer where anyone can paste a contract ID and browse every event that contract has ever emitted, decoded and searchable. It serves Soroban dapp builders, backend engineers integrating with existing protocols, and auditors reviewing contract behavior post-deployment.

That paragraph is locked wording. It appears verbatim in the README and in the docs introduction. Do not paraphrase it.

## 2. Why this repo exists

`pulsar-core` proves the events can be decoded. `pulsar-app` is what makes that useful to somebody who is not writing Rust: a client library they install, a daemon they run or call, and a site they can send a colleague to. Everything a user of the toolkit actually touches lives here.

## 3. Relationship to pulsar-core

`pulsar-stellar/pulsar-core` is the Rust contract layer. Its `pulsar-showcase` contract is the fixture the whole toolkit is calibrated against: every function on it exists to emit a specific event shape that the decoders here are tested against.

The import point is the `v0.1.0-contracts` release, deployed to Stellar testnet:

| Field | Value |
|---|---|
| Contract ID | `CDNWTVUDKCCGW7GOC6SBLUFXXUCD2YDHWRDUSXZ6CYBQKQWLCUYYWI5L` |
| Network | testnet |
| Release | `v0.1.0-contracts` |

That ID is the default in `.env.example` for both `NEXT_PUBLIC_SHOWCASE_CONTRACT_ID` and `PULSAR_INDEXER_BOOTSTRAP_CONTRACTS`. If a decoder here disagrees with what the contract emits, the contract is right and the decoder is wrong.

`pulsar-decoder`, the Rust decoder crate, is a placeholder until `v0.2.0-contracts`. Nothing here depends on it yet.

## 4. The three sub-stacks

**`packages/sdk`**, TypeScript, published as `@pulsar-stellar/sdk`. Wraps the indexer's HTTP API in a typed client, and offers a direct-RPC path for callers who want live events without running an indexer. Validates every response with Zod before returning it. Throws typed `PulsarError` subclasses, never raw errors. `@stellar/stellar-sdk` is a peer dependency, not a bundled one.

**`indexer/`**, Go, its own module. Polls Soroban RPC on an interval, decodes the events it finds, and writes them to SQLite locally or Postgres in production. Serves the read API that the SDK and the explorer both consume. It treats every byte from a contract as hostile input and validates before insert. Separate module rather than a workspace member because the language boundary is a natural seam, recorded in ADR-002.

**`apps/web`**, Next.js 15 with the App Router. The public explorer. Server Components by default, TanStack Query for client-side paging against the indexer, Tailwind and shadcn/ui for everything visual.

## 5. Documentation

Documentation is not in this repo. It lives in `pulsar-stellar/pulsar-docs` and publishes through GitBook's GitHub Sync from that repo's `main`. See ADR-009. A change here that alters public API is not done until the corresponding change is open against `pulsar-docs`.

## 6. Current phase and definition of done

Sprint 2, Phase A of the build sequence in `docs/planning/system-prompt.md` section 12: steps 1 through 15, the monorepo scaffold and these context files.

Phase A is done when the workspace installs clean on a fresh clone, the three CI workflows run and pass on an empty workspace, `.env.example` carries every variable from section 10, and these three `.agent/` files exist and are accurate.

Phase B, steps 16 through 19, starts the SDK skeleton. Before any SDK code lands, verify the `@stellar/stellar-sdk` version line, the contract bindings generation command, and the `getEvents` request and response shape against current upstream documentation. The system prompt's section 6 was written ahead of that verification and at least one of its claims about the bindings CLI looks wrong. See ADR-011.

## 7. Drips Wave context

The toolkit is aimed at a Drips Wave submission. That shapes two things. Repo hygiene is a deliverable, not an afterthought: branch protection, CONTRIBUTING, SECURITY, and a README that matches the pattern of approved Stellar repos all get a dedicated sprint before submission. And contributor-facing surface matters, because outside contributors are expected to work mostly in this repo rather than in `pulsar-core`, which stays reviewer-only until v1.0. Write issues and code with a stranger in mind.

## 8. Local development

```sh
nvm use                       # reads .nvmrc, Node 22
corepack enable               # provides pnpm
pnpm install                  # all TypeScript workspaces
cp .env.example .env.local    # then fill in anything non-default

pnpm typecheck
pnpm lint
pnpm test
pnpm build

cd indexer
go vet ./...
go test ./...
```

Per sub-stack commands land with each sub-stack and this list grows as they do.

## 9. Where secrets live

Nowhere in this repository. `.env.example` holds placeholders and defaults that are safe to publish, and it is the only env file that is committed. Real local values go in `.env.local`, which is gitignored.

In production, web values are set in the Vercel project's environment UI and indexer values in the Render service dashboard. Neither is mirrored into the repo. Anything prefixed `NEXT_PUBLIC_` reaches the browser, so a secret must never carry that prefix. If a credential does reach a commit, rotate it. Removing the commit is not sufficient.
