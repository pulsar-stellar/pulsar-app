# Contributing to Pulsar App

## How this project is built

This project's initial scaffolding and much of its ongoing implementation is written with Claude Code assistance under human review. Every commit is authored, reviewed, and merged by a human maintainer. Design decisions, architecture choices, and merge judgments are human. If you contribute a PR, we don't require you to disclose whether AI tools helped you write it; we do require that your code passes review, tests, and the discipline rules below.

The discipline rules are stated in this file, under Commit rules, Test discipline, and Code rules. Those sections are the standard a PR is measured against. The reasoning behind any rule that looks arbitrary is in the decision log at `.agent/decisions.md`.

## Setup

Three sub-stacks, two toolchains. The TypeScript side is one pnpm workspace. The Go indexer is a separate module with its own tooling, and pnpm does not manage it.

| Tool | Version | Install |
|---|---|---|
| Node.js | 22 LTS, pinned in `.nvmrc` | `nvm install` in the repo root, see ADR-012 |
| pnpm | 11 or newer | `corepack enable` |
| Go | 1.23 or newer | platform specific, indexer only |
| Git | 2.40 or newer | platform specific |

Verify before you start:

```sh
node --version     # expect v22.x
pnpm --version     # expect 11 or newer
go version         # expect go1.23 or newer
```

Install and check:

```sh
pnpm install
pnpm typecheck
pnpm lint
pnpm test

cd indexer && go vet ./... && go test ./...
```

Workspace members land in sequence as the build progresses, so a freshly cloned scaffold may contain fewer packages than the finished layout. The README records which artifacts exist at the current commit.

### pnpm-lock.yaml

`pnpm-lock.yaml` is committed and is not generated output you may freely overwrite. Do not commit an incidental lock file update. If your change genuinely requires new or updated dependencies, land the lock change in its own commit whose message says which dependency moved and why, for example `build(deps): bump @stellar/stellar-sdk for getEvents cursor fix`. A lock diff that appears alongside unrelated work will be sent back.

The same applies to `indexer/go.sum`.

## Working from issues

Substantive work is tracked with a GitHub issue opened before the work starts. The issue carries the scope, the acceptance criteria, and references to any ADR or specification section that governs it. The PR closes it with `Closes #NN` in the body.

This applies to maintainer work as much as to outside contributions. An issue written after the fact documents what was done; one written before it is a chance to disagree about scope while disagreeing is still cheap.

## Commit rules

These are enforced, not stylistic preferences.

**One commit per logical unit.** One SDK method, one HTTP handler, one component, one type file, one test block. A change that must compile together, such as a type field plus its callers, is one commit.

**Never `git add .`** Stage exact paths. This prevents build output, environment files, and key material from entering the history by accident.

**Push after every commit.** Do not batch local commits. If a push fails, stop and resolve it rather than accumulating work locally.

**Never rewrite pushed history.** Fix forward with a follow-up commit.

**Conventional commit format:** `type(scope): description`

- Types: `feat`, `fix`, `refactor`, `test`, `docs`, `chore`, `build`, `ci`
- Scopes: `sdk`, `indexer`, `web`, `docs`, `workspace`, `ci`, `deploy`. Use a scope only where it adds information the paths do not already carry
- Description: imperative mood, lowercase first letter, no trailing period, under 72 characters

Examples:

```
feat(sdk): implement events query with cursor pagination
test(indexer): assert events handler rejects unknown contract id
chore: seed decisions.md with pnpm workspace rationale
```

## Test discipline

Contributors must:

- Include tests for behavior-carrying code, meaning any function that encodes a decision the framework or SDK does not already make
- Pair every behavior-carrying change with its tests. In TypeScript the test may land in its own commit immediately after the implementation, since a test naming an absent export fails rather than breaking the build. In Go a test calling an undefined function does not compile, so implementation and test land together
- Give every SDK public method a happy path, one network failure path, and one validation failure path
- Give every indexer handler a success case, a malformed-input case, and a not-found case
- Assert exact decoded shapes, never partial matches
- Name tests for the single claim they make. A name that needs "and" usually means the test is hiding a coverage gap
- Not submit test theater: tests asserting only "did not throw", tests mocking the code under test, and trivially true assertions are rejected in review
- Keep line coverage above 80 percent on both sub-stacks

Some code is structural rather than behavior-carrying: type declarations, module scaffolding, and pass-through wrappers around a single library call with no decision in them. Structural work lands without paired tests and is covered through the public functions that call it.

Where a test's own correctness is not obvious from reading it, mutate the code it covers and confirm the test fails. A test that still passes against broken code is worse than no test, because it reports coverage it does not have.

## Code rules

### TypeScript, SDK and web

- `strict: true`, `noUncheckedIndexedAccess: true`, `exactOptionalPropertyTypes: true`
- No `any`. No `as` assertions except at a Zod parse boundary
- No `@ts-ignore` or `@ts-expect-error` without an ADR entry explaining why
- Every value crossing a system boundary is validated with a Zod schema before use. Data from RPC, from the indexer, and from the user is untrusted alike
- Errors are `PulsarError` or a subclass. Never `throw new Error(...)` in shipped code
- No floating promises. Every promise is awaited or explicitly handled
- No non-null assertions (`!`) in shipped code. They are the TypeScript equivalent of a panic on unexpected input
- Prefer `readonly` fields and `as const` literals
- Every exported item carries a doc comment stating what it does and what it throws. Write the doc comment before the implementation: if the contract is hard to state, the design is not ready

### Go, indexer

- Every error is wrapped with `%w` and carries context. Never `_ = err`
- No `panic` in request-handling code, and no `must`-style helpers outside `init`. A malformed request produces a typed error response, not a crash. Recovering middleware is a backstop, not a license
- Every function that can block takes `ctx context.Context` as its first parameter and passes it through
- Every SQL query is parameterized. String concatenation into SQL is never acceptable, not even for an identifier
- Contract event data is untrusted input. Validate before insert, always
- `gofmt` clean, `go vet` clean, warnings fail CI
- Every exported item carries a doc comment starting with its name. Every package carries a package comment

### React and Next.js, web

- App Router only. No Pages Router
- No inline styles in JSX. Tailwind and shadcn/ui only
- Server Components by default. `use client` only where interactivity requires it
- No secret ever reaches a `NEXT_PUBLIC_` variable

### Everywhere

- No stubs, no `TODO` comments, no unimplemented placeholders in shipped commits
- Every env var referenced in code appears in `.env.example` with a placeholder value
- Secrets never enter the repository

## Verify before writing

When the behavior of an external API is uncertain, check its current documentation or its types before writing code against it. Do not write to a remembered signature and let CI discover the difference. This applies to `@stellar/stellar-sdk`, `github.com/stellar/go`, and Soroban RPC in particular, where method names and response shapes have moved between versions.

## Writing rules for documentation and commit messages

No em dashes anywhere. Avoid "seamlessly", "robust", "powerful", "leverage", "unlock", "cutting-edge", "revolutionize", "delve into", "elevate", and "empower" used figuratively. Prefer concrete verbs and specific nouns.

Numbers in documentation come from measured data. A number that is a target is labeled as a target.

## Pull requests

Branch from `main`, one logical change per PR where practical, and open the PR with a description of what changed and how you verified it. CI must be green before review. A PR that changes behavior without changing tests will be sent back.

Changes to the indexer's HTTP surface, its database layer, or its migrations carry a higher review bar. That surface is publicly reachable and it writes data every downstream consumer reads.

## Security

Do not open a public issue for a security problem. Follow `SECURITY.md`.
