import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { firstRequest } from './support';

import { dispatch } from '@/lib/graphql/dispatch';
import { GraphQLRequestError } from '@/lib/graphql/errors';


const KEY = 'NEXT_PUBLIC_PULSAR_INDEXER_URL';
const CONTRACT_ID = 'CDNWTVUDKCCGW7GOC6SBLUFXXUCD2YDHWRDUSXZ6CYBQKQWLCUYYWI5L';

const eventNode = {
  id: '42',
  contractId: CONTRACT_ID,
  ledger: 1234,
  txHash: 'a1b2c3',
  eventIndex: 0,
  name: 'transfer',
  topics: [{ type: 'symbol', value: 'transfer' }],
  data: { type: 'i128', value: '1000' },
  rawTopics: ['AAAA'],
  rawData: 'AAAA',
  emittedAt: '2026-09-24T12:00:00Z',
  inSuccessfulContractCall: true,
};

function jsonResponse(body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { 'content-type': 'application/json' },
  });
}

function mockData(payload: unknown) {
  return vi.fn().mockResolvedValue(jsonResponse({ data: payload }));
}

describe('dispatch', () => {
  const original = process.env[KEY];

  beforeEach(() => {
    process.env[KEY] = 'http://indexer.test:8080';
  });

  afterEach(() => {
    if (original === undefined) delete process.env[KEY];
    else process.env[KEY] = original;
    vi.restoreAllMocks();
  });

  it('routes an allowlisted events operation to the events query', async () => {
    const fetchImpl = mockData({ events: { items: [eventNode], nextCursor: 'next' } });
    const result = await dispatch(
      { operation: 'events', variables: { contractId: CONTRACT_ID, limit: 10 } },
      { fetchImpl },
    );
    expect(result).toMatchObject({ nextCursor: 'next' });
    const { body } = firstRequest(fetchImpl);
    expect(body.variables).toMatchObject({
      contractId: CONTRACT_ID,
      limit: 10,
    });
  });

  it('routes contractWithEvents to the nested query', async () => {
    const fetchImpl = mockData({
      contract: {
        id: CONTRACT_ID,
        addedAt: '2026-09-20T08:00:00Z',
        firstIndexedLedger: 1000,
        lastIndexedLedger: 2000,
        status: 'active',
        events: { items: [eventNode], nextCursor: null },
      },
    });
    const result = await dispatch(
      { operation: 'contractWithEvents', variables: { id: CONTRACT_ID, limit: 25 } },
      { fetchImpl },
    );
    expect(result).toMatchObject({ id: CONTRACT_ID });
  });

  it('routes event by id', async () => {
    const fetchImpl = mockData({ event: eventNode });
    const result = await dispatch(
      { operation: 'event', variables: { id: '42' } },
      { fetchImpl },
    );
    expect(result).toMatchObject({ id: '42' });
  });

  it('rejects an operation outside the allowlist without a network call', async () => {
    const fetchImpl = vi.fn();
    const err = await dispatch(
      { operation: '__schema', variables: {} },
      { fetchImpl },
    ).catch((e: unknown) => e);
    expect(err).toBeInstanceOf(GraphQLRequestError);
    expect((err as GraphQLRequestError).category).toBe('validation');
    expect(fetchImpl).not.toHaveBeenCalled();
  });

  it('rejects a body that is not a shaped request without a network call', async () => {
    const fetchImpl = vi.fn();
    const err = await dispatch('not-an-object', { fetchImpl }).catch(
      (e: unknown) => e,
    );
    expect(err).toBeInstanceOf(GraphQLRequestError);
    expect((err as GraphQLRequestError).category).toBe('validation');
    expect(fetchImpl).not.toHaveBeenCalled();
  });

  it('rejects a variable of the wrong transport type without a network call', async () => {
    const fetchImpl = vi.fn();
    const err = await dispatch(
      { operation: 'events', variables: { contractId: CONTRACT_ID, limit: 'ten' } },
      { fetchImpl },
    ).catch((e: unknown) => e);
    expect(err).toBeInstanceOf(GraphQLRequestError);
    expect((err as GraphQLRequestError).category).toBe('validation');
    expect(fetchImpl).not.toHaveBeenCalled();
  });

  it('propagates a malformed contract id as VALIDATION_CONTRACT_ID', async () => {
    const fetchImpl = vi.fn();
    const err = await dispatch(
      { operation: 'events', variables: { contractId: 'nope' } },
      { fetchImpl },
    ).catch((e: unknown) => e);
    expect((err as GraphQLRequestError).code).toBe('VALIDATION_CONTRACT_ID');
    expect(fetchImpl).not.toHaveBeenCalled();
  });
});
