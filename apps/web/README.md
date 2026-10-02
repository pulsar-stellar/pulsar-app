# @pulsar-stellar/web

The explorer of the [Pulsar Stellar](https://github.com/pulsar-stellar/pulsar-app)
toolkit: a public web app for browsing decoded Soroban contract events. Paste a
contract ID and read every event it has emitted, decoded and searchable, served
by a Pulsar indexer.

This is the third piece of the toolkit, alongside the TypeScript SDK
(`packages/sdk`) and the Go indexer (`indexer/`). The SDK is the typed client,
the indexer stores and serves events, and this app is the explorer a person
opens in a browser. It holds no data of its own: every screen reads the
indexer's GraphQL surface through one validated boundary.

## Stack

- Next.js 15, App Router, Server Components by default
- React 19, TypeScript strict (extending the workspace `tsconfig.base.json`)
- Tailwind CSS v4 with shadcn/ui design tokens
- Vitest and Testing Library for component and unit tests

Node 22 and pnpm 11 are the pinned toolchain (`.nvmrc`, `packageManager`).

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
`http://localhost:8080`, the indexer's local address. See the indexer's own
[README](../../indexer/README.md) for standing one up; within a poll interval of
starting it against the showcase contract, this app has events to show.

Then:

```sh
pnpm --filter ./apps/web dev        # dev server on http://localhost:3000
pnpm --filter ./apps/web lint
pnpm --filter ./apps/web typecheck
pnpm --filter ./apps/web test
pnpm --filter ./apps/web build
```

## Configuration

The explorer needs exactly one value, and it is public by design.

| Variable | Required | Default | Meaning |
|---|---|---|---|
| `NEXT_PUBLIC_PULSAR_INDEXER_URL` | yes | `http://localhost:8080` | Base URL of the Pulsar indexer. Must be http or https; a trailing slash is trimmed so `/graphql` joins cleanly. |

It carries the `NEXT_PUBLIC_` prefix because it is inlined into the browser
bundle, so it must never hold a secret. It is still validated at the boundary
(`src/lib/env.ts`): a missing or malformed URL fails loudly as a deployment
mistake rather than surfacing as an opaque fetch error deep in a request. Every
variable the repo references appears in [`.env.example`](../../.env.example);
`scripts/verify-env-parity.sh` enforces that.

## Architecture: one validated boundary (ADR-046)

The explorer reads the indexer over GraphQL, and it reuses the SDK's Zod schemas
(`DecodedEventSchema`, `ContractIdSchema`, `ContractStatusSchema`) to validate
every response, so the two cannot drift. It reuses the schemas but never the
SDK's XDR decoder: `@stellar/stellar-sdk` must never reach the browser, which
would multiply the client bundle. The schema modules are `server-only`, imported
into client code by erased `type` imports alone, and the boundary is verified on
every build (shared First Load JS holds at about 102 kB, with no `stellar` in
the client chunks).

The data layer is **Hybrid**:

- **Server-side loads by default.** A page is a Server Component that calls a
  typed query wrapper (`src/lib/graphql/queries.ts`), which validates the
  response against its boundary schema before the page uses it. A contract and
  its first page of events arrive in a single nested query the REST surface
  cannot express (ADR-043).
- **A browser dispatch route for interactive refetches.** "Load more" and the
  health poll go through `POST /api/graphql` (`src/app/api/graphql/route.ts`),
  which is **not** a GraphQL proxy: the browser names one of a fixed allowlist of
  operations (`health`, `contract`, `event`, `events`, `contractWithEvents`) and
  supplies variables, and the route maps that to a server-side query constant. An
  input the allowlist does not name never reaches the indexer. The route bounds
  the body, rate-limits per client, and returns the catalog's generic messages
  for non-safe error categories (ADR-046).

### Escaping untrusted contract data (ADR-043)

Decoded event values are arbitrary contract content, so the explorer treats them
as untrusted: the recursive `DecodedValue` renderer places every decoded string
only as an escaped React text child, never through `dangerouslySetInnerHTML` and
never into an attribute sink. "Correct escaping on render is the explorer's
responsibility," and that is where it is met, with regression tests that assert a
script payload renders as literal text.

## Screens

| Route | Rendering | Purpose |
|---|---|---|
| `/` | static | The front door: a contract-ID lookup that validates the shape before routing. |
| `/c/[contractId]` | dynamic | Contract detail: indexing status and ledger window, plus the event list with name/topic/ledger filters, ordering, and cursor pagination, all carried in the URL. |
| `/e/[eventId]` | dynamic | Event detail: identity, the decoded topics and data as a collapsible tree over the full ADR-023 taxonomy, the raw XDR provenance, and a JSON export. |
| `/api/graphql` | dynamic (Node) | The browser dispatch route described above. |

A live indexer health indicator sits in the header, refreshing on an interval
and pausing while the tab is hidden. Each route has its own not-found, error, and
loading state; a malformed or absent id renders the segment's not-found UI rather
than an error.

## Accessibility

The screens target WCAG 2.1 AA: a skip link to the main landmark, a descriptive
heading per page, form labels and hints wired with `aria-describedby`, `status`
and `alert` live regions for async results and errors, `aria-busy` on loading
skeletons, and `prefers-reduced-motion` on every animation. Color is never the
sole signal (every status dot has a text label), and the interactive tokens
(input border, focus ring) meet the 3:1 non-text contrast minimum.

## Testing

```sh
pnpm --filter ./apps/web test            # Vitest, jsdom, Testing Library
pnpm --filter ./apps/web test:coverage   # enforces the 80% thresholds
```

Coverage thresholds are 80% for lines, functions, branches, and statements
(`vitest.config.ts`); the App Router route files are excluded, since their logic
lives in tested components. Tests use fakes and stubbed fetches, so they need no
indexer or network.

## Deployment

Vercel hosts `apps/web` (ADR). Connect the repository, set the project root to
`apps/web`, and set `NEXT_PUBLIC_PULSAR_INDEXER_URL` (and any other values) in
the Vercel project's environment UI, never in the repository. Anything prefixed
`NEXT_PUBLIC_` reaches the browser, so a secret must never carry that prefix.

## Documentation

This README and the sibling `indexer/README.md` are the in-repo operator docs.
The user-facing guide lives in the separate `pulsar-stellar/pulsar-docs` repo,
not here (ADR-009); an explorer walkthrough is owed there alongside the indexer's
GraphQL reference. See [`../../CONTRIBUTING.md`](../../CONTRIBUTING.md) for the
commit and review discipline, and [`../../.agent/decisions.md`](../../.agent/decisions.md)
for the reasoning behind the choices above.

## Layout

```
src/
  app/         App Router routes, layouts, API route, and global styles
  components/  Presentational and interactive UI
  lib/         Framework-agnostic helpers and the GraphQL data layer
tests/         Component and unit tests
```

## License

Apache-2.0. See [`../../LICENSE`](../../LICENSE).
