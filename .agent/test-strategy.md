# Test strategy: pulsar-app

App-specific testing rules. The cross-repo methodology, including how to verify that a test covers what it claims, lives in `pulsar-core`'s `.agent/testing.md` and in `docs/requirements.md` section 2. This file holds what those do not: rules that came out of working in this repo, in this toolchain.

The enforced rules a contributor is measured against are in `CONTRIBUTING.md`. This file is methodology, not a second rulebook.

## Bounded execution for mutation checks

A mutation check breaks the code under test and confirms the test notices. When the mutation removes a termination guard, such as cycle detection, a recursion limit, or a timeout, the resulting run does not fail. It runs forever, and the runner has to be killed by hand.

Put a hard bound on that specific check:

```sh
pnpm --filter ./packages/sdk exec vitest run --testTimeout=5000 tests/errors.test.ts
```

or wrap the command with `timeout`. The mutation still proves the guard is load-bearing, and it reports that as a failure rather than as a hang.

This came out of step 18. Removing the cause-cycle guard from `findPulsarError` hung the runner until it was killed after two minutes, which proved the point at the cost of the session's attention.

### The test's own loops need bounds too

The same applies to loops inside a test, not just to guards in the code under test. A paging test that walks until a cursor runs out will run forever if the code stops advancing the cursor, and that is exactly the regression the test exists to catch.

Bound the loop with a maximum iteration count and assert it, so the failure reports itself:

```ts
const maxPages = 5;
let requests = 0;
do {
  requests += 1;
  expect(requests, 'paging did not terminate').toBeLessThanOrEqual(maxPages);
  // ...
} while (cursor !== undefined);
```

This came out of step 28. Mutating the events method to drop the cursor from its outgoing query hung the runner, because the test kept fetching page one and reading the same cursor back. With the bound, the same mutation fails in under a second and names the reason.

## Trust but verify a background command

An operation that outlives the window in which it was watched does not get to be assumed complete. Installs, builds, deploys, and coverage runs all qualify. If subsequent work depends on its output, run a fresh verification pass first rather than reading the last output you happened to see.

A command moved to the background can also be stopped without leaving any record that says so. Absence of a failure is not evidence of success.

This came out of step 18 as well. A coverage run was backgrounded, left no completion record, and the checks were rerun from scratch before the commit rather than trusting the interrupted output.

## Schema tests need plausible-wrong inputs

An input that is obviously wrong proves almost nothing. Rejecting `/relative/path` proves the schema rejects nonsense, and accepting `https://example.com` proves the happy case. Neither says anything about `localhost:8080`, `ftp://internal/`, or `javascript:void(0)`.

Plausible-wrong inputs are the shapes an inattentive caller actually produces: a URL with the scheme left off, a capitalized enum value, a number where a string was meant, an ID pasted one character short. They sit close enough to correct that a permissive schema waves them through, and they are what a validator exists to catch. Cover them explicitly.

This came out of step 20. `PulsarConfigSchema` accepted `localhost:8080`, which the URL parser reads as protocol `localhost:`, along with `ftp://` and `javascript:`. Step 17's tests had only checked that a relative path was rejected, so a schema that would have built a client failing at its first request passed a full test suite and a coverage gate.

The general form: when writing a rejection test, ask what a hurried caller would actually type, not what a fuzzer would generate.

## High coverage can hide a miscovered branch

Coverage says a line ran. It does not say which test ran it, or that the test whose name claims that behavior is the one that did. A test can exercise a superset of the branch it names, pass for the wrong reason, and leave the branch it was written for untouched.

Multi-step parses are where this happens most. When a response goes through several stages, transport, then JSON parse, then envelope, then payload, an arrangement meant to exercise a late stage can fail at an early one and still produce the error type the assertion expects.

Read the line-by-line coverage output, not just the summary. An uncovered line inside a branch you believe you tested is the signal, and it usually shows up before a mutation check would.

This came out of step 22. The non-2xx tests returned an empty body, so `response.json()` threw before the status check ran. They asserted `PulsarNetworkError` and passed, through the malformed-JSON branch, while the status branch they were named for stayed uncovered. The summary looked healthy; the per-line output named the missed line. The fix was to return a valid JSON body with the non-success status, and to keep the empty-body case as its own test rather than conflating the two.

The general form: when arranging a test around a multi-step parse, check that every earlier stage succeeds, so the failure under test is the one the name claims.

## Where the two kinds of test live

- `tests/*.test.ts` runs by default and must not touch the network.
- `tests/*.test-d.ts` holds type-level assertions and runs under `vitest --typecheck`. Use it for what a runtime test cannot reach, such as the difference between a schema's input and output types, which is invisible at runtime because both parse the same values.
- `tests/*.integration.test.ts` is excluded from the default run. It may reach the network and must skip, not fail, when the network is unavailable.
