import { http, HttpResponse } from 'msw';
import { describe, expect, it } from 'vitest';

import { PulsarClient } from '../src/client.js';
import { PulsarNetworkError, PulsarValidationError } from '../src/errors.js';

import { eventPayload, trackedContractPayload } from './mocks/handlers.js';
import { INDEXER_URL, server } from './mocks/server.js';

const client = new PulsarClient({ indexerUrl: INDEXER_URL, timeoutMs: 200 });
const SHOWCASE_ID = trackedContractPayload.id;
const eventsRoute = `${INDEXER_URL}/contracts/:id/events`;

describe('querying events', () => {
  it('returns decoded events in camelCase', async () => {
    const page = await client.events(SHOWCASE_ID);
    expect(page.items).toHaveLength(1);
    expect(page.items[0]).toEqual({
      id: eventPayload.id,
      contractId: SHOWCASE_ID,
      ledger: eventPayload.ledger,
      txHash: eventPayload.tx_hash,
      eventIndex: 0,
      name: 'deposit',
      topics: eventPayload.topics_json,
      data: eventPayload.data_json,
      rawTopics: eventPayload.raw_topics,
      rawData: eventPayload.raw_data,
      emittedAt: eventPayload.emitted_at,
      inSuccessfulContractCall: true,
    });
  });

  it('keeps an event id beyond 2^53 exact, since it arrives as a string', async () => {
    const page = await client.events(SHOWCASE_ID);
    const id = page.items[0]?.id;
    expect(id).toBe('9007199254740993');
    // Round-tripping through a JSON number is exactly what ADR-021 avoids:
    // the last digit does not survive, and nothing reports an error.
    expect(String(Number(id))).toBe('9007199254740992');
    expect(String(Number(id))).not.toBe(id);
  });

  it('sends no query parameters when none were asked for', async () => {
    let seenUrl = '';
    server.use(
      http.get(eventsRoute, ({ request }) => {
        seenUrl = request.url;
        return HttpResponse.json({ data: { items: [] } });
      }),
    );
    await client.events(SHOWCASE_ID);
    expect(seenUrl).toBe(`${INDEXER_URL}/contracts/${SHOWCASE_ID}/events?limit=50&order=desc`);
  });

  it('maps filters to their snake_case query parameters', async () => {
    let seen: URLSearchParams | undefined;
    server.use(
      http.get(eventsRoute, ({ request }) => {
        seen = new URL(request.url).searchParams;
        return HttpResponse.json({ data: { items: [] } });
      }),
    );

    await client.events(SHOWCASE_ID, {
      name: 'deposit',
      fromLedger: 100,
      toLedger: 200,
      topicContains: 'GABC',
      limit: 10,
      order: 'asc',
    });

    expect(seen?.get('name')).toBe('deposit');
    expect(seen?.get('from_ledger')).toBe('100');
    expect(seen?.get('to_ledger')).toBe('200');
    expect(seen?.get('topic_contains')).toBe('GABC');
    expect(seen?.get('limit')).toBe('10');
    expect(seen?.get('order')).toBe('asc');
  });

  it('omits an unspecified filter rather than sending it empty', async () => {
    let seen: URLSearchParams | undefined;
    server.use(
      http.get(eventsRoute, ({ request }) => {
        seen = new URL(request.url).searchParams;
        return HttpResponse.json({ data: { items: [] } });
      }),
    );
    await client.events(SHOWCASE_ID);
    expect(seen?.has('name')).toBe(false);
    expect(seen?.has('cursor')).toBe(false);
  });
});

describe('cursor pagination', () => {
  it('reports null on the last page', async () => {
    const page = await client.events(SHOWCASE_ID);
    expect(page.nextCursor).toBeNull();
  });

  it('treats an absent next_cursor the same as an explicit null', async () => {
    server.use(http.get(eventsRoute, () => HttpResponse.json({ data: { items: [] } })));
    const page = await client.events(SHOWCASE_ID);
    expect(page.nextCursor).toBeNull();
  });

  it('surfaces a cursor and passes it back verbatim on the next request', async () => {
    const opaque = 'eyJsZWRnZXIiOjEyMzQ1Njd9';
    let seenCursor: string | null = null;
    server.use(
      http.get(eventsRoute, ({ request }) => {
        const cursor = new URL(request.url).searchParams.get('cursor');
        seenCursor = cursor;
        return cursor === null
          ? HttpResponse.json({ data: { items: [eventPayload] }, next_cursor: opaque })
          : HttpResponse.json({ data: { items: [] }, next_cursor: null });
      }),
    );

    const first = await client.events(SHOWCASE_ID);
    expect(first.nextCursor).toBe(opaque);

    const second = await client.events(SHOWCASE_ID, { cursor: first.nextCursor ?? undefined });
    expect(seenCursor).toBe(opaque);
    expect(second.nextCursor).toBeNull();
  });

  it('walks every page until the cursor runs out', async () => {
    const pages = new Map([
      [null, { items: [{ ...eventPayload, id: '1' }], next_cursor: 'c1' }],
      ['c1', { items: [{ ...eventPayload, id: '2' }], next_cursor: 'c2' }],
      ['c2', { items: [{ ...eventPayload, id: '3' }], next_cursor: null }],
    ]);
    server.use(
      http.get(eventsRoute, ({ request }) => {
        const cursor = new URL(request.url).searchParams.get('cursor');
        const page = pages.get(cursor);
        return HttpResponse.json({ data: { items: page?.items ?? [] }, next_cursor: page?.next_cursor });
      }),
    );

    const seen: string[] = [];
    let cursor: string | undefined;
    // Bounded deliberately. A paging loop whose cursor stops advancing runs
    // forever, so without this cap a regression hangs the runner instead of
    // failing it. See .agent/test-strategy.md.
    const maxPages = 5;
    let requests = 0;

    do {
      requests += 1;
      expect(requests, 'paging did not terminate').toBeLessThanOrEqual(maxPages);
      const page = await client.events(SHOWCASE_ID, cursor === undefined ? undefined : { cursor });
      seen.push(...page.items.map((event) => event.id));
      cursor = page.nextCursor ?? undefined;
    } while (cursor !== undefined);

    expect(requests).toBe(3);
    expect(seen).toEqual(['1', '2', '3']);
  });

  it('rejects an empty string as a cursor rather than treating it as absent', async () => {
    await expect(client.events(SHOWCASE_ID, { cursor: '' })).rejects.toThrow(
      PulsarValidationError,
    );
  });
});

describe('empty results', () => {
  it('returns an empty array, not null, when nothing matches', async () => {
    server.use(http.get(eventsRoute, () => HttpResponse.json({ data: { items: [] } })));
    const page = await client.events(SHOWCASE_ID);
    expect(page.items).toEqual([]);
    expect(page.items).not.toBeNull();
  });
});

describe('input validation', () => {
  const failOnRequest = () =>
    server.use(
      http.get(eventsRoute, () => {
        throw new Error('request should not have been sent');
      }),
    );

  it.each([
    ['an account ID, which starts with G', `G${SHOWCASE_ID.slice(1)}`],
    ['a lowercased contract ID', SHOWCASE_ID.toLowerCase()],
    ['an ID pasted with surrounding whitespace', ` ${SHOWCASE_ID}\n`],
  ])('rejects %s before sending anything', async (_label, candidate) => {
    failOnRequest();
    await expect(client.events(candidate)).rejects.toThrow(PulsarValidationError);
  });

  it('rejects a limit above the maximum page size', async () => {
    failOnRequest();
    await expect(client.events(SHOWCASE_ID, { limit: 501 })).rejects.toThrow(
      PulsarValidationError,
    );
  });

  it('rejects a negative ledger bound', async () => {
    failOnRequest();
    await expect(client.events(SHOWCASE_ID, { fromLedger: -1 })).rejects.toThrow(
      PulsarValidationError,
    );
  });

  it('rejects an unknown filter key rather than ignoring a typo', async () => {
    failOnRequest();
    await expect(
      client.events(SHOWCASE_ID, { fromLedgr: 100 } as never),
    ).rejects.toThrow(PulsarValidationError);
  });
});

describe('server failures', () => {
  it('throws for an untracked contract rather than returning an empty page', async () => {
    server.use(
      http.get(eventsRoute, () =>
        HttpResponse.json(
          { error: { code: 'not_found', message: 'contract not tracked' } },
          { status: 404 },
        ),
      ),
    );
    try {
      await client.events(SHOWCASE_ID);
      expect.unreachable('events should have thrown');
    } catch (error) {
      expect(error).toBeInstanceOf(PulsarNetworkError);
      expect((error as PulsarNetworkError).status).toBe(404);
    }
  });

  it('throws on a server error', async () => {
    server.use(
      http.get(eventsRoute, () =>
        HttpResponse.json({ error: { code: 'internal', message: 'db down' } }, { status: 500 }),
      ),
    );
    await expect(client.events(SHOWCASE_ID)).rejects.toThrow(PulsarNetworkError);
  });

  it('throws when an event in the page has the wrong shape', async () => {
    server.use(
      http.get(eventsRoute, () =>
        HttpResponse.json({ data: { items: [{ ...eventPayload, ledger: 'soon' }] } }),
      ),
    );
    try {
      await client.events(SHOWCASE_ID);
      expect.unreachable('events should have thrown');
    } catch (error) {
      expect((error as PulsarValidationError).issues[0]?.path.join('.')).toBe('items.0.ledger');
    }
  });

  it('rejects an event id sent as a number, which would lose precision', async () => {
    // Any number is rejected, not just an unsafe one. A schema that accepted
    // small ids would pass every test and corrupt real ones past 2^53.
    server.use(
      http.get(eventsRoute, () =>
        HttpResponse.json({ data: { items: [{ ...eventPayload, id: 42 }] } }),
      ),
    );
    await expect(client.events(SHOWCASE_ID)).rejects.toThrow(PulsarValidationError);
  });

  it('rejects an empty-string cursor from the server', async () => {
    server.use(
      http.get(eventsRoute, () => HttpResponse.json({ data: { items: [] }, next_cursor: '' })),
    );
    await expect(client.events(SHOWCASE_ID)).rejects.toThrow(PulsarValidationError);
  });
});
