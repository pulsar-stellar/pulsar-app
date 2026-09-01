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

Bounded execution applies to loops inside a test, not only to the suite as a whole. Pick a cap far above any legitimate run, 100 pages rather than 5 where the real ceiling is unknown, so the bound catches non-termination without ever firing on correct behavior.

Count on the dimension a mutation moves, not the dimension the happy path produces. A passing run produces items, so items are the tempting thing to count, but a mutation can leave that number flat forever: a loop refetching an empty page yields nothing while requests grow without bound. The bound has to sit where every attempt is visible whether or not it produced anything, which for HTTP means the mock handler:

```ts
http.get(route, () => {
  requests += 1;
  if (requests > MAX_REQUESTS) {
    throw new Error(`paging did not terminate: over ${MAX_REQUESTS} requests`);
  }
  // ...
});
```

Step 29 hit exactly this, and the two mutations separate the dimensions cleanly. Dropping the cursor follow still yielded items, so an item-based cap caught it. Removing the end-of-pages check looped on an empty page, yielding nothing, so the same cap never fired and the runner hung. Only the request count moved under both.

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

## Identifiers that can outgrow a JSON number must be strings

A JSON number cannot carry an integer above `Number.MAX_SAFE_INTEGER`, which is 2^53 minus one. Past that, `JSON.parse` rounds silently. There is no error, no warning, and no way to recover the original value: by the time a schema sees the field, the damage is done, so validating the parsed number cannot help.

Anything whose growth is unbounded belongs in a string on the wire and in the schema: database primary keys declared `BIGSERIAL` or `BIGINT`, transaction sequence numbers, ledger heights, and any counter that only ever goes up.

Reject numbers at schema time rather than accepting them and checking the range. A schema that takes `z.union([z.string(), z.number()])` passes every test written with small fixtures and corrupts real identifiers years later, once the table has grown, with nothing in the logs to say so. Test that a number is rejected outright, using a small one, precisely because a small one is what a permissive schema would let through.

This came out of step 28 and is recorded as ADR-021. The indexer's `events` table uses `BIGSERIAL`, so event ids are strings on the wire and the schema rejects numbers of any size. The paired test asserts that round-tripping the id through `Number` changes it, which is the failure being prevented, stated as an assertion.

A related trap when writing the test itself: a numeric literal in TypeScript source is subject to the same rounding, so comparing a parsed value against a literal above 2^53 compares two already-rounded numbers and proves nothing. Assert on the string form instead.

## An external system's visible metric is often not its constraining one

External services enforce several limits at once, and they are usually orthogonal. GitHub has a primary quota, an hourly request budget, and a separate secondary limit that fires on request *rate* to catch bursts. Exhausting either returns 403, and the two are not related.

The specific confusion to avoid: reading the quota, seeing thousands of requests remaining, and concluding that polling is safe. `gh api rate_limit` reported 4985 of 5000 remaining while every call to the runs endpoint returned 403. The visible metric was not the one doing the constraining, so checking it gave false reassurance.

When polling any external system, wait once for a realistic duration and then check once. A workflow that takes 30 seconds deserves one check after 45, not five checks. When a check does come back limited, stop rather than retrying: retrying is what extends the cooldown, and it is also what makes the failure look intermittent rather than caused.

This came out of step 29, after roughly a dozen commits each followed by an `until gh run list ...; do sleep 8; done` loop.

## Verify shapes against live protocol output before committing to them

A spec draft describes what someone intended a protocol to return. Live output shows what it does return, and the two diverge in ways no amount of reading closes. Type declarations are better evidence than prose and still not conclusive: they can be right about the fields and wrong about what the fields mean.

Before making a structural decision about data an external protocol produces, call the protocol and look. One request usually settles what a day of reading cannot.

Two findings from step 32 make the case. The planned event id format composed `txHash` with an event index, and a single `getEvents` call against testnet showed three consecutive events with three different `txHash` values but identical `transactionIndex` and `operationIndex`, so neither index identified an event and the format was not constructible. The same response showed the event `id` is the real per-event identifier and doubles as the paging cursor, while the SDK's own type declaration describes it as "the JSON-RPC request ID", which is wrong.

`pulsar-core`'s showcase contract, deployed to testnet at `v0.1.0-contracts`, exists partly for this. It is a known emitter whose event shapes are fixed, so a live call against it answers questions about real decoded output rather than about a fixture somebody wrote to match their own expectations.

The cost of skipping this is asymmetric. Checking costs one request. Not checking means the wrong shape reaches a schema, a database constraint, and a consumer's storage key before anything disagrees with it.

## A library's convenience converter is not a wire mapping

Helpers that turn a protocol value into an idiomatic native one optimize for the common case, and they drop what does not fit. They are the right tool for reading a value and the wrong one for defining a representation others will store.

`scValToNative` on a Soroban map with the same key twice returns one entry: the last write wins, and the first is gone with no error. On a `contractInstance` it declines entirely, handing back the raw XDR struct rather than a native value. Neither is a bug. A JavaScript object cannot hold a duplicate key, and there is no idiomatic native form for a contract instance.

The consequence is that a decoder must not be built by wrapping the convenience converter and trusting the result. Where the wire form carries something the native form cannot, the decoder handles that variant itself. Test for it directly: build the input the converter loses, decode it, and assert the loss did not happen.

## Another ecosystem's conventions are not evidence about this protocol

REST pagination ends with a null cursor. JSON APIs use null for absence. A list endpoint that returns nothing has nothing more to give. None of these are facts about Stellar RPC, and reaching for them is how a wrong assumption gets into a type without anyone noticing it was an assumption.

Live RPC never returns a null cursor. A page carrying events returns the last event's id as the cursor; an empty page returns a positional marker and paging continues from it. A tail has no end, so nothing in the protocol expresses one. Encoding an exhaustion signal would have meant inventing it.

Four draft-time assumptions have now been wrong when checked against live output: the `getEvents` cursor mode being a flat object rather than a discriminated union, `eventIndex` meaning a position within a transaction, the `ScVal` coverage the taxonomy needed, and the null cursor. Verify each behavioral assumption against a live call before it reaches a type or a schema.

## Sample beyond the reference contract

`pulsar-core`'s showcase contract is a known emitter with fixed shapes, which makes it the right fixture for checking that decoding works. It is the wrong fixture for discovering what decoding has to handle, because its event surface is narrow by design.

Everything the showcase does not emit is invisible while it is the only source: `ScVal` variants outside the four it uses, events whose first topic is not a Symbol, and events from reverted calls. That last one is not rare. A five-event sample from arbitrary testnet contracts contained one event with `inSuccessfulContractCall` false, which is what surfaced ADR-026.

When verifying a decoder, take a second sample from unfiltered testnet traffic. The reference contract shows the decoder works; real traffic shows what it is missing.

## Mock at the transport layer, not at the class layer

When the code under test wraps a third-party SDK, intercept the network underneath that SDK rather than stubbing the SDK itself. A stubbed class tests the wrapper against an imagined SDK. Intercepting the transport keeps the real one inside the test surface, so its parsing, its validation, and its own error handling are all still running.

The difference is not theoretical. `rpc.ts` is tested through the real `rpc.Server` with msw intercepting fetch, and that is the only reason a boundary showed up where nobody expected one: an event arriving with no `contractId` never reaches this SDK's mapper at all, because the upstream SDK builds a `Contract` while parsing the response and throws first. The failure therefore surfaces as a transport failure rather than a validation one. A stubbed client would have handed the mapper a clean object and confirmed a guard that production never reaches.

The fixtures for such a test should be real captured responses. A hand-written fixture encodes what the author believes the wire looks like, which is the belief being tested.

## Check that an assertion observes the behaviour and not the arrangement

A test whose fixture holds shared state can pass because of the fixture rather than because of the code. Request counters, insertion order, and generator sequence are the usual sources: the arrangement advances on its own, and the assertion reads that movement instead of the thing it names.

The independent-traversal test for `liveEventStream` had exactly this shape. Its handler served pages by request count, so the second traversal received page two, and the assertion comparing the two traversals was comparing different pages. The stream was independent, the test was not testing it. Rewriting the handler to answer by what the request actually asked for, a start ledger or a cursor, made a fresh traversal genuinely see the first page again.

The check is to ask what the fixture would return if the code under test were wrong in the specific way the test exists to catch. If the answer is the same either way, the assertion is reading the arrangement.

## A test that cannot run must skip, not return

A test whose precondition is unmet, an unreachable service, an absent fixture, a missing credential, has to say so through the runner. An early `return` reports the test as passed, which is indistinguishable in the output from a run that actually verified something. The suite then gets greener the more of it stops working.

Use the runner's own mechanism, `context.skip(reason)` in vitest, so the result reads as skipped and carries why. The reason matters as much as the status: "Stellar RPC unreachable" is actionable, a silent green is not.

This is the same failure as an assertion that reads the arrangement instead of the behaviour. Both produce a passing test that establishes nothing, and both are invisible precisely because passing is what you wanted to see.

## A verification needs a negative control

A check that something works passes vacuously when that something never loaded. A consumer typecheck that resolves no declarations at all reports the same green as one where every type is correct, because in both cases nothing complained.

Pair the positive case with a negative one: assert that a specific thing which ought to fail does fail, and for the right reason. Step 37's consumer check imports `PulsarClient` and expects success, then imports `requestMaybe`, now internal, and expects TS2305. The second is what proves the first was reading real declarations.

This is the same defect as an assertion that reads the arrangement, and as a test that returns instead of skipping. All three produce green from a system that verified nothing, and all three are invisible because green is what you were hoping for.

## Never point another package manager at pnpm's store

Verifying consumer experience means building a scratch project outside the repo and pointing it at the package. The tempting shortcut is to symlink into `node_modules/.pnpm/`, which is shared, content-addressed, and not yours.

Running `npm install` in such a project prunes through those symlinks. It walks what it believes is its own tree, finds packages absent from its lockfile, and deletes them. That emptied `@types+node@22.20.1` in this repo's store, which broke lint and typecheck across the whole package with errors pointing at files nobody had touched, in `decode.ts` and `http.ts`, which is a confusing way to learn what happened. Recovering took a full `rm -rf node_modules` and reinstall, because `pnpm install`, even with `--force`, checks the lockfile rather than whether the files are still there.

Use `npm pack` and install the tarball, or copy the built package into the scratch project. Both give a truer reading of consumer experience anyway, since that is what a consumer actually receives. If a symlink is unavoidable, never run another package manager's install in that project afterwards.

## A pipe hides the exit status of everything left of it

Bash reports a pipeline's status from its rightmost command. `go vet ./... | tail -3` exits 0 whenever `tail` succeeds, which is always, no matter what `go vet` did. A pre-commit check written that way reports green from a command that failed.

This shipped a red commit in Sprint 3. Step 39 left `indexer/` with a `go.mod` and no packages. `go vet ./...` and `go test ./...` both exit 1 on a module with no packages, and `go build ./...` exits 0, so the module genuinely could not satisfy `ci-go.yml`. The local check said all three passed, because each was piped through `tail` to keep the output short, and `$?` was read after the pipe. CI found it on the first run.

The filter is the tell. Piping through `tail`, `head`, `grep`, or `sed` to keep output readable is exactly when the status gets swallowed, and it is invisible because the output still looks like the output of the command you ran.

Either set `set -o pipefail` before the check, or capture the status without a pipe at all:

```sh
out=$(go vet ./... 2>&1); rc=$?
```

Same defect as an assertion that reads the arrangement, a test that returns instead of skipping, and a verification with no negative control. All of them produce green from something that did not pass.

## Check what a mechanism actually does before replacing it

A check that reports nothing can be broken, or it can be working correctly on
input that does not contain what it looks for. Those need opposite responses,
and the output alone does not distinguish them.

`verify-env-parity.sh` printed "no environment lookups found in source" for the
whole of the indexer's early build, and that was read as the script not
scanning Go. It always scanned Go. Phase A wrote it for all three sub-stacks,
with `os.Getenv` and `os.LookupEnv` in its pattern and `--include='*.go'` on its
grep. The real gap was narrower and further along: `internal/config` takes an
injected `Getenv` function, so it has no `os.Getenv` call sites at all, and the
names live in string literals passed to helper functions. The scanner matched
what it was written to match, and the codebase had stopped containing it.

Extending the pattern to the helper shape closed it in a few lines. Replacing
the script would have thrown away a working scanner and its TypeScript coverage
to rebuild the same thing around one blind spot.

The tell is a claim about a tool's behaviour that came from reading its output
rather than reading the tool. Open the file first. It is cheaper than the
rewrite, and it is the difference between a gap and a defect.

The same reading error has a second form worth naming. When the pattern was
extended, the helper list was assembled from memory and omitted `positiveInt`,
leaving two variables invisible. What caught it was an independent count:
asserting the scanner finds as many names as `grep` finds distinct literals in
`config.go`. A check whose coverage is a list somebody wrote out needs a
separate count to confirm the list is complete.

## A mutation that breaks the build has not tested anything

Checking that a test catches a defect means introducing the defect and watching
the test go red. That only works if the test still runs. Delete a field or a
type a test file references and the package stops compiling: the suite reports
failure, the assertion never executes, and the two outcomes are indistinguishable
from the outside.

Step 50 hit this. Deleting `LastIndexedLedger` from the contract struct produced
`FAIL ... [build failed]`, which was read as the schema-drift test working. It
was not: the test file referenced the field, so nothing in it ran. Renaming the
`db` tag instead keeps the file valid Go, and the test then fired properly and
printed the mismatch it was written to print.

Refine the mutation until the file still compiles:

- rename a struct tag rather than delete the field
- change a constant's value rather than remove the constant
- invert or weaken the logic inside a function rather than delete the function
- ignore an error rather than remove the call that produces it

The tell is a failure message that names the build rather than the assertion. If
the output does not contain the words the test was written to print, the test did
not run, and the mutation has proved nothing.

## A stale marker says the rest of the section is current

Marking part of a document stale makes a claim about everything you did not
mark. A reader who sees one caveat at the top of a section reasonably concludes
the remaining content was checked and stands, which is exactly the conclusion
the marker's author usually has not earned.

Section 7.1's schema carried a stale marker about the `UNIQUE` constraint,
correctly pointing at ADR-022. The same section was also missing a column ADR-026
requires, `in_successful_contract_call`, and nothing said so. The narrow marker
read as "the constraint is wrong, the rest is fine", and the migration written
from that section shipped ten of the eleven fields the SDK requires. It was found
at step 51, three migrations later, rather than at step 47 where the schema was
written.

When marking something stale, either say what the marker does not cover, or mark
the whole section. "Stale on the constraint; the column list has not been checked
against the ADRs recorded since" costs a sentence and does not make a promise
nobody verified.

The general shape: a partial warning is read as a complete one. This is the
documentation form of a test that passes by observing nothing.

## Where the two kinds of test live

- `tests/*.test.ts` runs by default and must not touch the network.
- `tests/*.test-d.ts` holds type-level assertions and runs under `vitest --typecheck`. Use it for what a runtime test cannot reach, such as the difference between a schema's input and output types, which is invisible at runtime because both parse the same values.
- `tests/*.integration.test.ts` is excluded from the default run. It may reach the network and must skip, not fail, when the network is unavailable.
