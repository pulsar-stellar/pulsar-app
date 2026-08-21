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

## Trust but verify a background command

An operation that outlives the window in which it was watched does not get to be assumed complete. Installs, builds, deploys, and coverage runs all qualify. If subsequent work depends on its output, run a fresh verification pass first rather than reading the last output you happened to see.

A command moved to the background can also be stopped without leaving any record that says so. Absence of a failure is not evidence of success.

This came out of step 18 as well. A coverage run was backgrounded, left no completion record, and the checks were rerun from scratch before the commit rather than trusting the interrupted output.

## Where the two kinds of test live

- `tests/*.test.ts` runs by default and must not touch the network.
- `tests/*.test-d.ts` holds type-level assertions and runs under `vitest --typecheck`. Use it for what a runtime test cannot reach, such as the difference between a schema's input and output types, which is invisible at runtime because both parse the same values.
- `tests/*.integration.test.ts` is excluded from the default run. It may reach the network and must skip, not fail, when the network is unavailable.
