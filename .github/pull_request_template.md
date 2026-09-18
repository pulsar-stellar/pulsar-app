<!--
  Pulsar App pull request template. Fill every section that applies.

  This is a standard your PR is measured against, alongside CONTRIBUTING.md.
  A PR that leaves sections as unedited placeholder text, or that changes
  behavior without changing tests, is sent back before review.

  If a section does not apply, keep its heading and write "N/A" with a
  one-line reason. Do not delete headings.

  Writing rules (see CONTRIBUTING.md > Writing rules):
  - No em dashes anywhere.
  - Avoid "seamlessly", "robust", "powerful", "leverage", "unlock",
    "cutting-edge", "revolutionize", "delve into", "elevate", and "empower"
    used figuratively. Prefer concrete verbs and specific nouns.
  - Numbers come from measured data. A number that is a target is labeled
    a target.
-->

# <type(scope): one-line summary>

<!--
  Match your squash-merge commit subject. Conventional format:
  type(scope): imperative, lowercase first letter, no trailing period, under 72 chars.
  Types:  feat  fix  refactor  test  docs  chore  build  ci
  Scopes: sdk  indexer  web  docs  workspace  ci  deploy   (use one only where it adds information the paths do not)
-->

> **Status:** Draft | Ready for review
> **Closes:** #<issue>
> **ADRs:** <ADR-0NN, ADR-0NN | none>
> **Sub-stacks touched:** <sdk | indexer | web | docs | workspace | ci>

---

## 1. Summary and core invariant

<!--
  Two or three sentences: what this PR changes and why. Then state the one
  invariant it establishes or preserves. The invariant is the single property
  a reviewer should hold the whole diff against.
-->

**What changed:**

**Core invariant this PR upholds:**

---

## 2. Scope and workstream audit

<!--
  One logical change per PR where practical (CONTRIBUTING.md > Commit rules).
  Say what is in scope and what is deliberately left out, so a reviewer is not
  hunting for work that was never intended here.
-->

**In scope:**
-

**Deliberately out of scope, and where it is tracked:**
-

**Requirement to evidence map:**

| Requirement (issue / ADR / spec section) | Where it is satisfied in this diff |
|---|---|
|  |  |

---

## 3. Background and trust-boundary context

<!--
  What was the behavior before this change, and why did it need to change?
  Name the boundary this PR touches, if any:
  - The indexer HTTP surface, which is publicly reachable and writes data every
    downstream consumer reads.
  - The decode-then-store path, where contract event data is untrusted input
    and is validated before insert.
  - The SDK boundary, where every value crossing from RPC or the indexer is
    validated with a Zod schema before use.
  A PR that touches no trust boundary can say so in one line.
-->

**Before:**

**After:**

| Case | Before | After |
|---|---|---|
|  |  |  |

---

## 4. Design and approach

<!--
  How the change works, and the decisions behind it. If it changes public
  behavior, link the ADR that authorizes it (.agent/decisions.md), or note that
  a new ADR is added in this PR. Call out any effect on the ADR-017 response
  envelope or the apierror catalog (ADR-038).
-->

**Approach:**

**Key decisions:**

**Response-envelope / error-catalog impact:** <!-- new codes, class or status changes, or "none" -->

---

## 5. Detailed changes, file by file

<!--
  Group by sub-stack and package. One short paragraph or bullet per file that
  carries a decision. Pure scaffolding can be summarized in a line.
-->

### SDK (`packages/sdk`)
-

### Indexer (`indexer/`)
-

### Web (`apps/web`)
-

### Other (workspace, ci, docs)
-

---

## 6. Failure-mode analysis and observability

<!--
  For each new failure path: what triggers it, what the caller sees, and what
  the operator sees. The indexer never leaks a store error to the client; it
  logs the cause server-side against the request id and returns a generic
  message. Show that pairing here.
-->

| Failure scenario | Client sees (status + code) | Server logs (event + fields) | Recovery |
|---|---|---|---|
|  |  |  |  |

---

## 7. Verification and test evidence

<!--
  Paste the commands you ran and their results. CI must be green before review;
  this section is the local evidence that it will be. Every behavior-carrying
  change is paired with tests (CONTRIBUTING.md > Test discipline). Coverage
  stays above 80 percent on each sub-stack this PR touches.
-->

**TypeScript, if `packages/sdk` or `apps/web` is touched:**

```
pnpm typecheck
pnpm lint
pnpm test
```

<paste result>

**Go indexer, if `indexer/` is touched:**

```
cd indexer
go vet ./...
go test -race ./...
staticcheck ./...
```

<paste result>

**Coverage on the sub-stacks this PR touches:**

**New or changed tests, and the single claim each one makes:**
-

---

## 8. Configuration reference

<!--
  Every env var referenced in code appears in .env.example with a placeholder
  (CONTRIBUTING.md > Code rules > Everywhere). List anything added or changed
  here, or write "no configuration change".
-->

| Env var / flag | Default | Purpose | Present in `.env.example`? |
|---|---|---|---|
|  |  |  |  |

---

## 9. Security, invariants, and data-integrity checklist

<!--
  Tick what applies. An unticked box that should be ticked is a reason the PR is
  not ready. The indexer HTTP surface, its database layer, and its migrations
  carry a higher review bar (CONTRIBUTING.md > Pull requests).
-->

- [ ] No secret, key, or credential enters the repo or the diff. `.env.example` carries placeholders only.
- [ ] Every value crossing a system boundary is validated before use: Zod in TypeScript, `validate` at the indexer boundary. Contract event data is treated as hostile input and validated before insert.
- [ ] Every SQL query is parameterized. No string concatenation into SQL, not even for an identifier.
- [ ] No `panic` is reachable from an HTTP request. A malformed request produces a typed error response, not a crash.
- [ ] Errors returned to a client carry no store internals, host names, or stack traces. The cause is logged server-side against the request id.
- [ ] A new or changed state-changing endpoint (POST, DELETE, and the like) is authenticated and rate limited, or this PR states why the surface is not yet network-reachable and what gates it before it is.
- [ ] No `any`, no `as` outside a Zod parse boundary, and no `@ts-ignore` or non-null assertion in shipped TypeScript without an ADR.
- [ ] No secret reaches a `NEXT_PUBLIC_` variable.
- [ ] The change stays within the SECURITY.md scope, and anything security-sensitive is handled through the repository Security tab, not a public issue.

---

## 10. Files changed

<!--
  A table of the files this PR touches. Keep it short: path, area, a one-line
  change, and the sign of the diff. The totals line is a sanity check against
  the diff stat.
-->

| Path | Area | Change | +/- |
|---|---|---|---|
|  |  |  |  |

**Totals:** <N files, +A / -B>

---

## 11. Rollout, migration, and operations

<!--
  What has to happen for this to ship, and what an operator should watch.
  Database migrations live under indexer/migrations/ per driver; note any added
  here and whether they are forward-only and present for both drivers. Note
  compatibility with the published SDK contract (the ADR-017 wire classes are
  fixed) and with any running indexer.
-->

**Database migration:** <none | migration added; forward-only?; both drivers?>

**Backward compatibility:** <effect on published `@pulsar-stellar/sdk` consumers, or "none">

**Operator note:** <what to watch after deploy, or "none">

---

<!--
  Maintainer merge gate (the reviewer confirms these, not the author):
  - Branch from main, one logical change, CI green.
  - Commits follow the conventional format; pushed history is not rewritten.
  - A behavior change is paired with tests.
  - Indexer HTTP, database, and migration changes are reviewed at the higher bar.
-->
