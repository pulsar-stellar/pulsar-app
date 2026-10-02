import { describe, expect, it, vi } from 'vitest';

import { EventFetchError, fetchEventsPage, fetchHealth } from '@/lib/graphql/browser';

const CONTRACT_ID = 'CDNWTVUDKCCGW7GOC6SBLUFXXUCD2YDHWRDUSXZ6CYBQKQWLCUYYWI5L';

/** A minimal fetch double returning the given status and JSON body. */
function fetchReturning(ok: boolean, body: unknown): typeof fetch {
  return vi.fn(() =>
    Promise.resolve({ ok, json: () => Promise.resolve(body) } as unknown as Response),
  );
}

describe('fetchEventsPage', () => {
  it('posts the events operation and returns the connection', async () => {
    const fetchImpl = fetchReturning(true, {
      data: { items: [{ id: '1' }], nextCursor: 'cur2' },
    });

    const page = await fetchEventsPage(
      { contractId: CONTRACT_ID, cursor: 'cur1', limit: 25 },
      { fetchImpl },
    );

    expect(page.nextCursor).toBe('cur2');
    const call = (fetchImpl as unknown as ReturnType<typeof vi.fn>).mock.calls[0];
    const [url, init] = call as [string, RequestInit];
    expect(url).toBe('/api/graphql');
    expect(JSON.parse(init.body as string)).toEqual({
      operation: 'events',
      variables: { contractId: CONTRACT_ID, cursor: 'cur1', limit: 25 },
    });
  });

  it('throws with the route category and code on a non-OK response', async () => {
    const fetchImpl = fetchReturning(false, {
      error: { category: 'validation', code: 'VALIDATION_LIMIT', message: 'bad limit' },
    });

    await expect(
      fetchEventsPage({ contractId: CONTRACT_ID }, { fetchImpl }),
    ).rejects.toMatchObject({
      name: 'EventFetchError',
      category: 'validation',
      code: 'VALIDATION_LIMIT',
      message: 'bad limit',
    });
  });

  it('maps a transport failure to a network error', async () => {
    const fetchImpl = vi.fn(() => {
      throw new Error('offline');
    }) as unknown as typeof fetch;

    await expect(
      fetchEventsPage({ contractId: CONTRACT_ID }, { fetchImpl }),
    ).rejects.toMatchObject({ category: 'network' });
  });

  it('maps a malformed body to a network error', async () => {
    const fetchImpl = vi.fn(() =>
      Promise.resolve({
        ok: true,
        json: () => Promise.reject(new Error('not json')),
      } as unknown as Response),
    ) as unknown as typeof fetch;

    await expect(
      fetchEventsPage({ contractId: CONTRACT_ID }, { fetchImpl }),
    ).rejects.toBeInstanceOf(EventFetchError);
  });

  it('rejects an unexpected success payload', async () => {
    const fetchImpl = fetchReturning(true, { data: { unexpected: true } });

    await expect(
      fetchEventsPage({ contractId: CONTRACT_ID }, { fetchImpl }),
    ).rejects.toMatchObject({ category: 'internal' });
  });
});

describe('fetchHealth', () => {
  it('posts the health operation with no variables and returns the record', async () => {
    const fetchImpl = fetchReturning(true, {
      data: { ok: true, version: '0.1.0', latestLedger: 1284913, trackedContracts: 3 },
    });

    const health = await fetchHealth({ fetchImpl });

    expect(health).toMatchObject({ ok: true, latestLedger: 1284913 });
    const call = (fetchImpl as unknown as ReturnType<typeof vi.fn>).mock.calls[0];
    const [url, init] = call as [string, RequestInit];
    expect(url).toBe('/api/graphql');
    expect(JSON.parse(init.body as string)).toEqual({ operation: 'health' });
  });

  it('rejects a health payload missing required fields', async () => {
    const fetchImpl = fetchReturning(true, { data: { ok: true } });

    await expect(fetchHealth({ fetchImpl })).rejects.toMatchObject({
      category: 'internal',
    });
  });

  it('maps a non-OK health response to an EventFetchError', async () => {
    const fetchImpl = fetchReturning(false, {
      error: { category: 'internal', code: 'INTERNAL_STORE', message: 'store down' },
    });

    await expect(fetchHealth({ fetchImpl })).rejects.toBeInstanceOf(EventFetchError);
  });
});
