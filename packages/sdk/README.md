# @pulsar-stellar/sdk

[![npm](https://img.shields.io/npm/v/@pulsar-stellar/sdk?logo=npm)](https://www.npmjs.com/package/@pulsar-stellar/sdk) [![license](https://img.shields.io/badge/license-Apache_2.0-blue.svg)](./LICENSE)

TypeScript client for Soroban contract events. Reads decoded event history from a Pulsar indexer, or tails events straight from Stellar RPC.

Published on npm as [`@pulsar-stellar/sdk`](https://www.npmjs.com/package/@pulsar-stellar/sdk).

Stellar RPC keeps roughly a week of history. An indexer keeps all of it. This SDK gives you one event type and one set of methods across both, so the source an event came from is a deployment decision rather than a rewrite.

## Installation

```bash
npm install @pulsar-stellar/sdk @stellar/stellar-sdk
```

`@stellar/stellar-sdk` is a **required peer dependency**, version `>=16.2.0 <17.0.0`. Install it whichever methods you use: it is imported at module load for XDR decoding, not lazily on the RPC path only.

## Quickstart

```ts
import { PulsarClient, liveEventStream } from '@pulsar-stellar/sdk';

const client = new PulsarClient({
  indexerUrl: 'https://indexer.example.com',
  rpcUrl: 'https://soroban-testnet.stellar.org',
  network: 'testnet',
});
```

### Read event history from the indexer

Past the retention wall, oldest first, one page at a time.

```ts
const page = await client.events('CDNWTVUDKCCGW7GOC6SBLUFXXUCD2YDHWRDUSXZ6CYBQKQWLCUYYWI5L');

for (const event of page.items) {
  console.log(event.ledger, event.name, event.data);
}

// Or let the SDK walk the pages for you.
for await (const event of client.eventStream(contractId, { name: 'transfer' })) {
  console.log(event.name, event.topics);
}
```

### Tail live events from RPC

No indexer needed. This reads Stellar RPC directly, so it sees an event as soon as its ledger closes.

```ts
for await (const event of liveEventStream(client.config, { startLedger: 4378751 })) {
  console.log('new event:', event.name, event.eventIndex);
  break; // the stream polls forever; break when you are done
}
```

### Fetch one event by id

```ts
const event = await client.event('12345');

if (event !== null) {
  console.log(event.name, event.emittedAt);
}
```

### Register a contract to index

Registration is a write route. A Pulsar indexer gates its writes behind a static bearer token, so configure `adminToken` to authenticate them. The token rides only on writes, never on the public reads above.

```ts
const client = new PulsarClient({
  indexerUrl: 'https://indexer.example.com',
  adminToken: process.env.PULSAR_INDEXER_ADMIN_TOKEN, // at least 16 characters
});

const contract = await client.registerContract('CDNWTVUDKCCGW7GOC6SBLUFXXUCD2YDHWRDUSXZ6CYBQKQWLCUYYWI5L');
```

A bearer token sent over plaintext http would leak, so the SDK refuses one against an `http` indexer unless the host is loopback (`localhost`, `127.0.0.1`, `[::1]`). Configuring an `adminToken` against any other `http` URL throws a `PulsarValidationError` at construction, before any request is sent. If the indexer rejects the write with a 401, the call throws a `PulsarAuthError`.

## Key concepts

**Decoding is wire-faithful.** A `DecodedEvent` reports what the ledger holds. `name` is the leading topic Symbol exactly as emitted, lowercase, and topics stay separate from data. Integers wider than 32 bits are strings so JSON cannot round them, and every event carries the raw XDR beside the decoded form so you can check the decoding rather than trust it.

**Two paths, one type.** The indexer path (`client.events`, `client.eventStream`, `client.event`) reads stored history and needs a running indexer. The RPC path (`fetchLiveEvents`, `liveEventStream`) reads the live tail and needs only a public RPC endpoint, but reaches back only as far as that node's retention window. Both produce `DecodedEvent`. Ids differ by source: indexer events carry digits, RPC events carry `rpc:` followed by the RPC identifier.

**Paging is by opaque cursor.** Pass `nextCursor` from a page back as `query.cursor`. Stop when it is null. Never parse it.

**The live tail has no end.** RPC returns a cursor on every response, including empty ones, so there is no exhaustion signal to report and `LiveEventsPage.cursor` is never null. `liveEventStream` polls until you break out of the loop. Deciding when to stop is yours, because only the network knows whether more is coming.

**Unknown values degrade, they do not throw.** A `ScVal` variant this version cannot name arrives as `{ type: 'unknown', xdr }` with its base64 intact, so a protocol addition does not break a client that was working.

## Working with contracts

`buildContractCall` assembles an invocation. It is synchronous and touches no network, so the transaction it returns is **unprepared**: run it through `prepareTransaction` before signing.

```ts
import { buildContractCall } from '@pulsar-stellar/sdk';
import { Address, Networks, nativeToScVal, rpc } from '@stellar/stellar-sdk';

const server = new rpc.Server('https://soroban-testnet.stellar.org');
const account = await server.getAccount(callerAddress);

const tx = buildContractCall({
  account,
  contractId,
  method: 'transfer',
  args: [new Address(from).toScVal(), nativeToScVal(100n, { type: 'i128' })],
  networkPassphrase: Networks.TESTNET,
});

const prepared = await server.prepareTransaction(tx);
prepared.sign(keypair);
await server.sendTransaction(prepared);
```

Your `account` is never mutated, so you can build several calls from one.

## Errors

Everything thrown descends from `PulsarError`, so you can branch on the kind rather than match message text.

```ts
import { PulsarAuthError, PulsarNetworkError, PulsarValidationError } from '@pulsar-stellar/sdk';

try {
  await client.registerContract(contractId);
} catch (error) {
  if (error instanceof PulsarAuthError) {
    console.error('write rejected, check adminToken', error.status);
  } else if (error instanceof PulsarNetworkError) {
    console.error('unreachable or refused', error.status, error.url);
  } else if (error instanceof PulsarValidationError) {
    console.error('unexpected shape', error.issues);
  }
}
```

`PulsarAuthError` is a sibling of `PulsarNetworkError`, not a subclass, on purpose: a caller that retries a `PulsarNetworkError` as a transient fault should not retry an auth failure, which only fails again until the token is fixed.

## Prerequisites

- **Node 22.13** or newer
- **TypeScript 5.6** or newer, if you compile against the bundled declarations
- ESM and CommonJS are both supported

## License

Apache-2.0. See [LICENSE](./LICENSE).
