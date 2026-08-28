/**
 * Tests for the direct-RPC path.
 *
 * These run against the real `rpc.Server` with msw intercepting the transport,
 * rather than against a stubbed client. That keeps the SDK's own response
 * parsing inside the test: `contractId` really does arrive as a `Contract`
 * instance, topics really are parsed from base64, and a change in either would
 * show up here rather than in production.
 *
 * The event fixtures are real testnet output, captured from an unfiltered
 * `getEvents` call on 2026-08-28. One of them carries
 * `inSuccessfulContractCall: false`, which is what surfaced ADR-026.
 */

import { xdr } from '@stellar/stellar-sdk';
import { http, HttpResponse } from 'msw';
import { describe, expect, it } from 'vitest';

import { PulsarNetworkError, PulsarValidationError } from '../src/errors.js';
import {
  fetchLiveEvents,
  liveEventStream,
  RPC_ID_PREFIX,
  toLiveDecodedEvent,
} from '../src/rpc.js';
import { EventIdSchema, type ResolvedPulsarConfig } from '../src/types.js';

import { server } from './mocks/server.js';

const RPC_URL = 'https://rpc.example';

const CONFIG: ResolvedPulsarConfig = {
  indexerUrl: 'https://indexer.example',
  rpcUrl: RPC_URL,
  timeoutMs: 5000,
};

/** A real testnet event, from a call whose contract call reverted. */
const REVERTED_EVENT = {
  type: 'contract',
  ledger: 4_378_751,
  ledgerClosedAt: '2026-08-28T11:42:22Z',
  contractId: 'CDLZFC3SYJYDZT7K67VZ75HPJVIEUVNIXF47ZG2FB2RMQQVU2HHGCYSC',
  id: '0018806592342327296-0000000000',
  operationIndex: 0,
  transactionIndex: 0,
  txHash: '819fde6f35783789b752603c9abf361f8ade5538a330b9f4e376fabb3e409fdd',
  inSuccessfulContractCall: false,
  topic: ['AAAADwAAAANmZWUA', 'AAAAEgAAAAAAAAAAA+0oimKONzC713z+Cl8orRHR8/s0JY+JMothA0fY7bY='],
  value: 'AAAACgAAAAAAAAAAAAAAAAAAAGQ=',
} as const;

/** The next event from the same ledger, whose call committed. */
const COMMITTED_EVENT = {
  ...REVERTED_EVENT,
  id: '0018806592342327296-0000000001',
  txHash: 'f49e58cea767591b0cbd2c9e63dd0e93c6b57d9d5a0f4dcb0b2e5f8c3a1d7e42',
  inSuccessfulContractCall: true,
} as const;

type RawEvent = Record<string, unknown>;

/** Builds the JSON-RPC envelope Stellar RPC actually returns. */
function rpcResult(events: readonly RawEvent[], cursor: string) {
  return HttpResponse.json({
    jsonrpc: '2.0',
    id: 1,
    result: {
      events,
      cursor,
      latestLedger: 4_378_765,
      oldestLedger: 4_257_806,
      latestLedgerCloseTime: '1787917352',
      oldestLedgerCloseTime: '1787311504',
    },
  });
}

/**
 * Serves a fixed list of pages, and counts requests.
 *
 * The count is the loop bound. A mutation that drops the stream's termination
 * or its cursor shows up as a request count that runs past the pages provided,
 * which fails here instead of hanging the runner. That bound lives at the
 * request layer on purpose: bounding items instead lets an empty-page loop
 * spin forever.
 */
function servePages(pages: ReadonlyArray<{ events: readonly RawEvent[]; cursor: string }>): {
  requests: () => number;
  bodies: () => unknown[];
} {
  const bodies: unknown[] = [];

  server.use(
    http.post(`${RPC_URL}/`, async ({ request }) => {
      const body: unknown = await request.json();
      bodies.push(body);

      if (bodies.length > pages.length) {
        return HttpResponse.json(
          { jsonrpc: '2.0', id: 1, error: { code: -32600, message: 'page bound exceeded' } },
          { status: 200 },
        );
      }

      const page = pages[bodies.length - 1];
      return rpcResult(page?.events ?? [], page?.cursor ?? '');
    }),
  );

  return { requests: (): number => bodies.length, bodies: (): unknown[] => bodies };
}

/** Reads the `params` a captured JSON-RPC request body carried. */
function paramsOf(body: unknown): Record<string, unknown> {
  const params = (body as { params?: unknown }).params;
  return (params ?? {}) as Record<string, unknown>;
}

describe('fetchLiveEvents, reading a page', () => {
  it('returns decoded events and the cursor for a ledger-range query', async () => {
    servePages([
      { events: [REVERTED_EVENT, COMMITTED_EVENT], cursor: '0018806592342327296-0000000001' },
    ]);

    const page = await fetchLiveEvents(CONFIG, { startLedger: 4_378_751 });

    expect(page.cursor).toBe('0018806592342327296-0000000001');
    expect(page.latestLedger).toBe(4_378_765);
    expect(page.events).toHaveLength(2);
    expect(page.events[0]).toEqual({
      id: 'rpc:0018806592342327296-0000000000',
      contractId: 'CDLZFC3SYJYDZT7K67VZ75HPJVIEUVNIXF47ZG2FB2RMQQVU2HHGCYSC',
      ledger: 4_378_751,
      txHash: REVERTED_EVENT.txHash,
      eventIndex: 0,
      name: 'fee',
      topics: [
        { type: 'symbol', value: 'fee' },
        { type: 'address', value: 'GAB62KEKMKHDOMF3256P4CS7FCWRDUPT7M2CLD4JGKFWCA2H3DW3NRRB' },
      ],
      data: { type: 'i128', value: '100' },
      rawTopics: [...REVERTED_EVENT.topic],
      rawData: REVERTED_EVENT.value,
      emittedAt: '2026-08-28T11:42:22Z',
      inSuccessfulContractCall: false,
    });
  });

  it('sends the cursor, and not a start ledger, when continuing', async () => {
    const served = servePages([
      { events: [COMMITTED_EVENT], cursor: '0018806592342327296-0000000001' },
    ]);

    await fetchLiveEvents(CONFIG, { cursor: '0018806592342327296-0000000000' });

    const params = paramsOf(served.bodies()[0]);
    expect(params['pagination']).toMatchObject({ cursor: '0018806592342327296-0000000000' });
    expect(params['startLedger']).toBeUndefined();
  });

  it('carries a cursor for continuation even when the page is empty', async () => {
    servePages([{ events: [], cursor: '0018806656766836735-4294967295' }]);

    const page = await fetchLiveEvents(CONFIG, { startLedger: 4_378_751 });

    expect(page.events).toEqual([]);
    expect(page.cursor).toBe('0018806656766836735-4294967295');
    expect(page.cursor.length).toBeGreaterThan(0);
  });

  it('reports an empty page as an empty array rather than as an absence', async () => {
    servePages([{ events: [], cursor: '0018806656766836735-4294967295' }]);

    const page = await fetchLiveEvents(CONFIG, { startLedger: 4_378_751 });

    expect(page.events).not.toBeNull();
    expect(Array.isArray(page.events)).toBe(true);
    expect(page.latestLedger).toBe(4_378_765);
  });

  it('degrades an ScVal variant it cannot name to the unknown fallback', async () => {
    const instance = xdr.ScVal.scvContractInstance(
      new xdr.ScContractInstance({
        executable: xdr.ContractExecutable.contractExecutableStellarAsset(),
        storage: null,
      }),
    ).toXDR('base64');

    servePages([
      {
        events: [{ ...REVERTED_EVENT, value: instance }],
        cursor: '0018806592342327296-0000000000',
      },
    ]);

    const page = await fetchLiveEvents(CONFIG, { startLedger: 4_378_751 });

    expect(page.events[0]?.data).toEqual({ type: 'unknown', xdr: instance });
    expect(page.events[0]?.topics).toHaveLength(2);
  });

  it('keeps an event whose first topic is not a symbol, with an empty name', async () => {
    servePages([
      {
        events: [{ ...REVERTED_EVENT, topic: ['AAAAAwAAAAE='] }],
        cursor: '0018806592342327296-0000000000',
      },
    ]);

    const page = await fetchLiveEvents(CONFIG, { startLedger: 4_378_751 });

    expect(page.events).toHaveLength(1);
    expect(page.events[0]?.name).toBe('');
    expect(page.events[0]?.topics).toEqual([{ type: 'u32', value: 1 }]);
  });
});

describe('fetchLiveEvents, rejecting a query', () => {
  it('rejects a query setting both a start ledger and a cursor', async () => {
    const served = servePages([{ events: [], cursor: 'c' }]);
    const query = { startLedger: 4_378_751, cursor: 'c' } as unknown as {
      startLedger: number;
    };

    await expect(fetchLiveEvents(CONFIG, query)).rejects.toBeInstanceOf(PulsarValidationError);
    expect(served.requests()).toBe(0);
  });

  it('rejects a query setting neither a start ledger nor a cursor', async () => {
    const served = servePages([{ events: [], cursor: 'c' }]);
    const query = {} as unknown as { startLedger: number };

    await expect(fetchLiveEvents(CONFIG, query)).rejects.toBeInstanceOf(PulsarValidationError);
    expect(served.requests()).toBe(0);
  });

  it.each([
    ['a negative start ledger', { startLedger: -1 }],
    ['a fractional start ledger', { startLedger: 4_378_751.5 }],
    ['a zero start ledger', { startLedger: 0 }],
    ['an empty cursor', { cursor: '' }],
  ])('rejects %s', async (_label, query) => {
    await expect(
      fetchLiveEvents(CONFIG, query as unknown as { startLedger: number }),
    ).rejects.toBeInstanceOf(PulsarValidationError);
  });

  it.each([
    ['zero', 0],
    ['negative', -5],
    ['fractional', 2.5],
    ['past the maximum', 100_000],
  ])('rejects a %s limit', async (_label, limit) => {
    await expect(fetchLiveEvents(CONFIG, { startLedger: 1, limit })).rejects.toBeInstanceOf(
      PulsarValidationError,
    );
  });

  it('rejects a config with no rpcUrl, naming the operation', async () => {
    const withoutRpc: ResolvedPulsarConfig = { indexerUrl: 'https://indexer.example', timeoutMs: 5000 };

    await expect(fetchLiveEvents(withoutRpc, { startLedger: 1 })).rejects.toMatchObject({
      operation: 'rpc.fetchLiveEvents',
    });
  });
});

describe('fetchLiveEvents, failing', () => {
  it('wraps a JSON-RPC error as a network error carrying the cause', async () => {
    server.use(
      http.post(`${RPC_URL}/`, () =>
        HttpResponse.json({
          jsonrpc: '2.0',
          id: 1,
          error: { code: -32600, message: 'startLedger must be within the ledger range' },
        }),
      ),
    );

    const failure = await fetchLiveEvents(CONFIG, { startLedger: 1 }).catch(
      (error: unknown) => error,
    );

    expect(failure).toBeInstanceOf(PulsarNetworkError);
    expect(failure).toMatchObject({ operation: 'rpc.fetchLiveEvents', url: RPC_URL });
    expect((failure as PulsarNetworkError).cause).toBeDefined();
  });

  it('wraps an unreachable endpoint as a network error', async () => {
    server.use(http.post(`${RPC_URL}/`, () => HttpResponse.error()));

    await expect(fetchLiveEvents(CONFIG, { startLedger: 1 })).rejects.toBeInstanceOf(
      PulsarNetworkError,
    );
  });

  it('rejects a response whose shape this SDK cannot read', async () => {
    server.use(
      http.post(`${RPC_URL}/`, () =>
        HttpResponse.json({ jsonrpc: '2.0', id: 1, result: { latestLedger: 1 } }),
      ),
    );

    await expect(fetchLiveEvents(CONFIG, { startLedger: 1 })).rejects.toBeInstanceOf(
      PulsarValidationError,
    );
  });

  it('rejects an event whose id is not in the toid-ordinal form', async () => {
    servePages([{ events: [{ ...REVERTED_EVENT, id: 'not-an-id-at-all' }], cursor: 'c' }]);

    const failure = await fetchLiveEvents(CONFIG, { startLedger: 1 }).catch(
      (error: unknown) => error,
    );

    expect(failure).toBeInstanceOf(PulsarValidationError);
    expect((failure as PulsarValidationError).details).toMatchObject({ rpcId: 'not-an-id-at-all' });
  });
});

describe('liveEventStream', () => {
  it('walks pages with the cursor until the consumer stops', async () => {
    const served = servePages([
      { events: [REVERTED_EVENT], cursor: 'cursor-1' },
      { events: [COMMITTED_EVENT], cursor: 'cursor-2' },
    ]);

    const seen: string[] = [];

    for await (const event of liveEventStream(CONFIG, { startLedger: 4_378_751 })) {
      seen.push(event.id);
      if (seen.length === 2) break;
    }

    expect(seen).toEqual([
      'rpc:0018806592342327296-0000000000',
      'rpc:0018806592342327296-0000000001',
    ]);
    expect(served.requests()).toBe(2);
    expect(paramsOf(served.bodies()[1])['pagination']).toMatchObject({ cursor: 'cursor-1' });
  });

  it('stops requesting as soon as the consumer breaks', async () => {
    const served = servePages([
      { events: [REVERTED_EVENT, COMMITTED_EVENT], cursor: 'cursor-1' },
      { events: [COMMITTED_EVENT], cursor: 'cursor-2' },
    ]);

    for await (const event of liveEventStream(CONFIG, { startLedger: 4_378_751 })) {
      expect(event.id).toBe('rpc:0018806592342327296-0000000000');
      break;
    }

    expect(served.requests()).toBe(1);
  });

  it('polls past an empty page rather than treating it as the end', async () => {
    const served = servePages([
      { events: [], cursor: 'cursor-1' },
      { events: [COMMITTED_EVENT], cursor: 'cursor-2' },
    ]);

    const seen: string[] = [];

    for await (const event of liveEventStream(
      CONFIG,
      { startLedger: 4_378_751 },
      { pollIntervalMs: 1 },
    )) {
      seen.push(event.id);
      break;
    }

    expect(seen).toEqual(['rpc:0018806592342327296-0000000001']);
    expect(served.requests()).toBe(2);
  });

  it('waits the poll interval before asking again after an empty page', async () => {
    servePages([
      { events: [], cursor: 'cursor-1' },
      { events: [COMMITTED_EVENT], cursor: 'cursor-2' },
    ]);

    const started = Date.now();

    for await (const event of liveEventStream(
      CONFIG,
      { startLedger: 4_378_751 },
      { pollIntervalMs: 120 },
    )) {
      void event;
      break;
    }

    expect(Date.now() - started).toBeGreaterThanOrEqual(90);
  });

  it('propagates a failure on a later page to the consumer', async () => {
    let calls = 0;
    server.use(
      http.post(`${RPC_URL}/`, () => {
        calls += 1;
        return calls === 1
          ? rpcResult([REVERTED_EVENT], 'cursor-1')
          : HttpResponse.json({ jsonrpc: '2.0', id: 1, error: { code: -32000, message: 'boom' } });
      }),
    );

    const seen: string[] = [];
    const consume = async (): Promise<void> => {
      for await (const event of liveEventStream(CONFIG, { startLedger: 4_378_751 })) {
        seen.push(event.id);
      }
    };

    await expect(consume()).rejects.toBeInstanceOf(PulsarNetworkError);
    expect(seen).toEqual(['rpc:0018806592342327296-0000000000']);
  });

  it('gives each iteration an independent traversal', async () => {
    /**
     * Answers by what was asked rather than by a request counter, so a second
     * traversal starting from the same ledger really does see the same first
     * page. A counter would make this test pass for the wrong reason.
     */
    let requests = 0;
    server.use(
      http.post(`${RPC_URL}/`, async ({ request }) => {
        requests += 1;
        const body: unknown = await request.json();
        const pagination = (paramsOf(body)['pagination'] ?? {}) as { cursor?: string };
        return pagination.cursor === undefined
          ? rpcResult([REVERTED_EVENT], 'cursor-1')
          : rpcResult([COMMITTED_EVENT], 'cursor-2');
      }),
    );

    const stream = liveEventStream(CONFIG, { startLedger: 4_378_751 });
    const first: string[] = [];
    const second: string[] = [];

    for await (const event of stream) {
      first.push(event.id);
      break;
    }

    for await (const event of stream) {
      second.push(event.id);
      break;
    }

    expect(first).toEqual(['rpc:0018806592342327296-0000000000']);
    expect(second).toEqual(first);
    expect(requests).toBe(2);
  });

  it('carries a caller-supplied limit into every continuation', async () => {
    const served = servePages([
      { events: [REVERTED_EVENT], cursor: 'cursor-1' },
      { events: [COMMITTED_EVENT], cursor: 'cursor-2' },
    ]);

    const seen: string[] = [];

    for await (const event of liveEventStream(CONFIG, { startLedger: 4_378_751, limit: 25 })) {
      seen.push(event.id);
      if (seen.length === 2) break;
    }

    expect(paramsOf(served.bodies()[0])['pagination']).toMatchObject({ limit: 25 });
    expect(paramsOf(served.bodies()[1])['pagination']).toMatchObject({
      cursor: 'cursor-1',
      limit: 25,
    });
  });
});

describe('the ADR-024 id contract', () => {
  it('prefixes every RPC-sourced id', async () => {
    servePages([{ events: [REVERTED_EVENT, COMMITTED_EVENT], cursor: 'c' }]);

    const page = await fetchLiveEvents(CONFIG, { startLedger: 4_378_751 });

    for (const event of page.events) {
      expect(event.id.startsWith(RPC_ID_PREFIX)).toBe(true);
    }
  });

  it('uses the RPC identifier verbatim rather than composing one', async () => {
    servePages([{ events: [REVERTED_EVENT], cursor: 'c' }]);

    const page = await fetchLiveEvents(CONFIG, { startLedger: 4_378_751 });
    const id = page.events[0]?.id ?? '';

    expect(id).toBe(`${RPC_ID_PREFIX}${REVERTED_EVENT.id}`);
    expect(id).not.toContain(REVERTED_EVENT.txHash);
  });

  it('takes eventIndex from the second component of that same identifier', async () => {
    servePages([{ events: [REVERTED_EVENT, COMMITTED_EVENT], cursor: 'c' }]);

    const page = await fetchLiveEvents(CONFIG, { startLedger: 4_378_751 });

    expect(page.events.map((event) => event.eventIndex)).toEqual([0, 1]);
    expect(page.events[0]?.txHash).not.toBe(page.events[1]?.txHash);
    expect(page.events[0]?.ledger).toBe(page.events[1]?.ledger);
  });

  it('produces an id the indexer event lookup refuses', async () => {
    servePages([{ events: [REVERTED_EVENT], cursor: 'c' }]);

    const page = await fetchLiveEvents(CONFIG, { startLedger: 4_378_751 });

    expect(EventIdSchema.safeParse(page.events[0]?.id).success).toBe(false);
  });
});

describe('the ADR-026 success flag', () => {
  it('reports a reverted call and a committed one differently', async () => {
    servePages([{ events: [REVERTED_EVENT, COMMITTED_EVENT], cursor: 'c' }]);

    const page = await fetchLiveEvents(CONFIG, { startLedger: 4_378_751 });

    expect(page.events.map((event) => event.inSuccessfulContractCall)).toEqual([false, true]);
  });

  it('keeps the reverted event rather than filtering it out', async () => {
    servePages([{ events: [REVERTED_EVENT], cursor: 'c' }]);

    const page = await fetchLiveEvents(CONFIG, { startLedger: 4_378_751 });

    expect(page.events).toHaveLength(1);
  });
});

describe('filters', () => {
  it('passes a contract filter through to the request', async () => {
    const served = servePages([{ events: [], cursor: 'c' }]);
    const contractId = 'CDLZFC3SYJYDZT7K67VZ75HPJVIEUVNIXF47ZG2FB2RMQQVU2HHGCYSC';

    await fetchLiveEvents(CONFIG, { startLedger: 1, filter: { contractIds: [contractId] } });

    expect(paramsOf(served.bodies()[0])['filters']).toEqual([
      { type: 'contract', contractIds: [contractId] },
    ]);
  });

  it('defaults to contract events when no filter is given', async () => {
    const served = servePages([{ events: [], cursor: 'c' }]);

    await fetchLiveEvents(CONFIG, { startLedger: 1 });

    expect(paramsOf(served.bodies()[0])['filters']).toEqual([{ type: 'contract' }]);
  });

  it('passes a system filter through', async () => {
    const served = servePages([{ events: [], cursor: 'c' }]);

    await fetchLiveEvents(CONFIG, { startLedger: 1, filter: { type: 'system' } });

    expect(paramsOf(served.bodies()[0])['filters']).toEqual([{ type: 'system' }]);
  });

  it('rejects a filter naming something that is not a contract ID', async () => {
    await expect(
      fetchLiveEvents(CONFIG, { startLedger: 1, filter: { contractIds: ['nope'] } }),
    ).rejects.toBeInstanceOf(PulsarValidationError);
  });
});

describe('events this SDK cannot map', () => {
  /**
   * An event with no `contractId` never reaches this module's mapper: the
   * upstream SDK builds a `Contract` while parsing the response and throws
   * first, so the failure arrives as a transport failure rather than as a
   * validation one. That is worth pinning, because the boundary is not where
   * it looks like it is.
   */
  it('surfaces an event with no contract ID as a failure from the RPC call', async () => {
    const { contractId: _dropped, ...withoutContract } = REVERTED_EVENT;
    servePages([{ events: [withoutContract], cursor: 'c' }]);

    await expect(fetchLiveEvents(CONFIG, { startLedger: 1 })).rejects.toBeInstanceOf(
      PulsarNetworkError,
    );
  });

  /**
   * The mapper guards the same case anyway, since it is exported and the
   * indexer will feed it events from its own poller rather than through
   * `rpc.Server`. Driving it directly is the only way to reach that guard.
   */
  it('rejects an event with no contract ID when the mapper is called directly', () => {
    const raw = {
      id: REVERTED_EVENT.id,
      ledger: REVERTED_EVENT.ledger,
      ledgerClosedAt: REVERTED_EVENT.ledgerClosedAt,
      txHash: REVERTED_EVENT.txHash,
      topic: [xdr.ScVal.scvSymbol('fee')],
      value: xdr.ScVal.scvVoid(),
      inSuccessfulContractCall: true,
      contractId: undefined,
    };

    expect(() => toLiveDecodedEvent(raw, 'test')).toThrow(PulsarValidationError);
  });
});
