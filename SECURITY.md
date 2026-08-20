# Security Policy

## Audit status

**This code is unaudited. Do not use it to custody real value.**

`pulsar-app` is the application layer of the Pulsar Stellar toolkit: a TypeScript SDK, a Go indexer daemon, and a Next.js explorer. None of it signs transactions on a user's behalf or holds keys. The SDK builds unsigned transactions and hands them back to the caller, and signing stays with the caller's wallet.

No third-party security audit has been performed on any code in this repository. No audit is scheduled. Deployments before `v1.0.0` target Stellar testnet only.

## Reporting a vulnerability

Report privately through GitHub. Open the repository's **Security** tab and choose **Report a vulnerability**. The report stays private between you and the maintainers until an advisory is published.

Do not open a public issue for a security problem. Do not post details in the Telegram group.

Include what you have:

- Which surface is affected, from the table below
- What an attacker gains
- Steps to reproduce, ideally a failing test, a request that triggers it, or a testnet transaction hash
- Affected version, commit SHA, or deployed URL

Response targets, stated as targets rather than guarantees while this is a solo-maintained project:

| Stage | Target |
|---|---|
| Acknowledge receipt | 3 working days |
| Initial assessment | 10 working days |
| Fix or documented mitigation for a confirmed high or critical issue | 30 days |

If a report goes unacknowledged past the first target, escalate by opening a public issue that says a security report is awaiting acknowledgement. Include no vulnerability details in it.

## Scope

In scope:

| Surface | Path |
|---|---|
| TypeScript SDK | `packages/sdk/` |
| Go indexer, including its HTTP API and database layer | `indexer/` |
| Web explorer | `apps/web/` |
| CI workflows, including supply chain concerns such as unpinned actions | `.github/workflows/` |
| Workspace dependency pins | `package.json`, `pnpm-lock.yaml`, `indexer/go.mod`, `indexer/go.sum` |

Out of scope here:

- The reference contract and the decoder crate. Report those against `pulsar-stellar/pulsar-core`, which has its own policy.
- The documentation site. Report that against `pulsar-stellar/pulsar-docs`.
- Stellar protocol, `@stellar/stellar-sdk`, `github.com/stellar/go`, and Stellar RPC itself. Report those to the Stellar Development Foundation.
- Availability of public testnet RPC endpoints, of the hosted indexer, and of the hosted explorer.
- Findings that require a maintainer to run untrusted code or hand over credentials.

## What this project treats as a vulnerability

In the indexer: SQL injection through any query, an endpoint that returns data across a boundary it should not cross, a missing or bypassable rate limit, unbounded memory growth from an attacker-controlled request, a panic reachable from an HTTP request, or event data from a contract being trusted into the database without validation. The indexer treats contract data as hostile input, so anything that reaches storage or a response unvalidated is a bug.

In the SDK: a response from the indexer or from RPC that is parsed without validation, a credential or secret written to logs, or an input that causes an unhandled crash rather than a typed `PulsarError`.

In the web explorer: cross-site scripting through contract data rendered into the page, a server-side request forgery through a user-supplied URL, or a secret exposed through a `NEXT_PUBLIC_` variable that should have stayed server-side.

In the repository: a committed credential, a dependency pinned to a version with a known advisory, or a CI configuration that lets an untrusted PR reach repository secrets.

## Disclosure

Coordinated disclosure. Once a fix ships, a GitHub Security Advisory is published naming the reporter, unless the reporter asks to stay anonymous.

There is no bug bounty. This project has no funding to pay for one, and saying otherwise would be dishonest.

## Credentials

Secrets never enter this repository. `.env.example` carries placeholders and nothing else. Real values live in `.env.local`, which is gitignored, and in the Vercel and Render dashboards for production. If a credential reaches a commit, the response is to rotate it, not merely to remove the commit.
