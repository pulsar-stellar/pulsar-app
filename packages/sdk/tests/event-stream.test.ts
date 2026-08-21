import { http, HttpResponse } from 'msw';
import { describe, expect, it } from 'vitest';

import { PulsarClient } from '../src/client.js';
import { PulsarNetworkError } from '../src/errors.js';
import type { DecodedEvent } from '../src/types.js';

import { eventPayload, trackedContractPayload } from './mocks/handlers.js';
import { INDEXER_URL, server } from './mocks/server.js';

const client = new PulsarClient({ indexerUrl: INDEXER_URL, timeoutMs: 200 });
const SHOWCASE_ID = trackedContractPayload.id;
const eventsRoute = `${INDEXER_URL}/contracts/:id/events`;

/** Requests one traversal may make before the handler calls it non-terminating. */
const MAX_REQUESTS = 100;

/**
 * Serves three pages and counts requests, so a test can assert how many pages
 * were actually fetched rather than only what came out of the loop.
 *
 * The request cap is the real termination bound. Counting yielded items is not
 * enough: a loop that refetches an empty page forever yields nothing, so an
 * item-based cap never trips and the runner hangs instead of failing.
 */
function servePages(): { requests: () => number } {
  let requests = 0;
  const pages = new Map<string | null, { ids: string[]; next: string | null }>([
    [null, { ids: ['1', '2'], next: 'c1' }],
    ['c1', { ids: ['3', '4'], next: 'c2' }],
    ['c2', { ids: ['5'], next: null }],
  ]);

  server.use(
    http.get(eventsRoute, ({ request }) => {
      requests += 1;
      if (requests > MAX_REQUESTS) {
        throw new Error(`paging did not terminate: over ${MAX_REQUESTS} requests`);
      }
      const cursor = new URL(request.url).searchParams.get('cursor');
      const page = pages.get(cursor);
      return HttpResponse.json({
        data: { items: (page?.ids ?? []).map((id) => ({ ...eventPayload, id })) },
        next_cursor: page?.next ?? null,
      });
    }),
  );

  return { requests: () => requests };
}

/** Collects a stream with a hard page ceiling, so a stuck cursor fails fast. */
async function collect(stream: AsyncIterable<DecodedEvent>, maxItems = 100): Promise<string[]> {
  const ids: string[] = [];
  for await (const event of stream) {
    ids.push(event.id);
    expect(ids.length, 'stream did not terminate').toBeLessThanOrEqual(maxItems);
  }
  return ids;
}

describe('walking a stream to the end', () => {
  it('yields every event across every page in order', async () => {
    const tracker = servePages();
    const ids = await collect(client.eventStream(SHOWCASE_ID));
    expect(ids).toEqual(['1', '2', '3', '4', '5']);
    expect(tracker.requests()).toBe(3);
  });

  it('terminates within a bounded number of pages', async () => {
    let requests = 0;
    server.use(
      http.get(eventsRoute, () => {
        requests += 1;
        if (requests > MAX_REQUESTS) {
          throw new Error(`paging did not terminate: over ${MAX_REQUESTS} requests`);
        }
        return HttpResponse.json({ data: { items: [] }, next_cursor: null });
      }),
    );

    const ids = await collect(client.eventStream(SHOWCASE_ID));
    expect(ids).toEqual([]);
    expect(requests, 'a terminating stream should fetch exactly one empty page').toBe(1);
  });

  it('starts from a caller-supplied cursor', async () => {
    const tracker = servePages();
    const ids = await collect(client.eventStream(SHOWCASE_ID, { cursor: 'c1' }));
    expect(ids).toEqual(['3', '4', '5']);
    expect(tracker.requests()).toBe(2);
  });
});

describe('leaving the loop early', () => {
  it('fetches no further page when the consumer breaks on the first item', async () => {
    const tracker = servePages();

    for await (const event of client.eventStream(SHOWCASE_ID)) {
      expect(event.id).toBe('1');
      break;
    }

    expect(tracker.requests(), 'break should not trigger another fetch').toBe(1);
  });

  it('fetches no further page when the consumer breaks at a page boundary', async () => {
    const tracker = servePages();
    const seen: string[] = [];

    for await (const event of client.eventStream(SHOWCASE_ID)) {
      seen.push(event.id);
      if (event.id === '2') break;
    }

    expect(seen).toEqual(['1', '2']);
    expect(tracker.requests()).toBe(1);
  });

  it('stops fetching when the consumer returns out of the loop', async () => {
    const tracker = servePages();

    const firstId = await (async () => {
      for await (const event of client.eventStream(SHOWCASE_ID)) {
        return event.id;
      }
      return null;
    })();

    expect(firstId).toBe('1');
    expect(tracker.requests()).toBe(1);
  });
});

describe('errors', () => {
  it('propagates a failure on a later page to the consumer', async () => {
    let requests = 0;
    server.use(
      http.get(eventsRoute, () => {
        requests += 1;
        return requests === 1
          ? HttpResponse.json({
              data: { items: [{ ...eventPayload, id: '1' }] },
              next_cursor: 'c1',
            })
          : HttpResponse.json(
              { error: { code: 'internal', message: 'db down' } },
              { status: 500 },
            );
      }),
    );

    const seen: string[] = [];
    await expect(
      (async () => {
        for await (const event of client.eventStream(SHOWCASE_ID)) {
          seen.push(event.id);
        }
      })(),
    ).rejects.toThrow(PulsarNetworkError);

    expect(seen, 'events yielded before the failure stay valid').toEqual(['1']);
  });

  it('surfaces a validation failure from the first page', async () => {
    server.use(
      http.get(eventsRoute, () =>
        HttpResponse.json({ data: { items: [{ ...eventPayload, ledger: 'soon' }] } }),
      ),
    );

    await expect(collect(client.eventStream(SHOWCASE_ID))).rejects.toThrow();
  });

  it('rejects a malformed contract ID on first iteration rather than at call time', async () => {
    const stream = client.eventStream('not-a-contract');
    await expect(collect(stream)).rejects.toThrow();
  });
});

describe('consumer patterns', () => {
  it('supports awaiting inside the loop body', async () => {
    servePages();
    const ids: string[] = [];

    for await (const event of client.eventStream(SHOWCASE_ID)) {
      await new Promise((resolve) => setTimeout(resolve, 1));
      ids.push(event.id);
    }

    expect(ids).toEqual(['1', '2', '3', '4', '5']);
  });

  it('gives each iteration an independent traversal', async () => {
    const tracker = servePages();
    const stream = client.eventStream(SHOWCASE_ID);

    const first = await collect(stream);
    const second = await collect(stream);

    expect(first).toEqual(['1', '2', '3', '4', '5']);
    expect(second).toEqual(first);
    expect(tracker.requests(), 'each traversal pages from the start').toBe(6);
  });

  it('keeps two concurrent traversals of one stream independent', async () => {
    servePages();
    const stream = client.eventStream(SHOWCASE_ID);

    const [first, second] = await Promise.all([collect(stream), collect(stream)]);

    expect(first).toEqual(['1', '2', '3', '4', '5']);
    expect(second).toEqual(['1', '2', '3', '4', '5']);
  });
});
