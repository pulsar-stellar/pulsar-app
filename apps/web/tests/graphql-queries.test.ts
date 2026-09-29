import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { firstRequest } from './support';

import { GraphQLRequestError } from '@/lib/graphql/errors';
import {
  getContract,
  getContractWithEvents,
  getEvent,
  getEvents,
  getHealth,
} from '@/lib/graphql/queries';


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

const contractNode = {
  id: CONTRACT_ID,
  addedAt: '2026-09-20T08:00:00Z',
  firstIndexedLedger: 1000,
  lastIndexedLedger: 2000,
  status: 'active',
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

describe('query wrappers', () => {
  const original = process.env[KEY];

  beforeEach(() => {
    process.env[KEY] = 'http://indexer.test:8080';
  });

  afterEach(() => {
    if (original === undefined) delete process.env[KEY];
    else process.env[KEY] = original;
    vi.restoreAllMocks();
  });

  it('getHealth returns the health payload', async () => {
    const fetchImpl = mockData({
      health: { ok: true, version: '0.1.0', latestLedger: 5000, trackedContracts: 2 },
    });
    const health = await getHealth({ fetchImpl });
    expect(health.ok).toBe(true);
    expect(health.trackedContracts).toBe(2);
  });

  it('getContract returns the contract', async () => {
    const fetchImpl = mockData({ contract: contractNode });
    const contract = await getContract(CONTRACT_ID, { fetchImpl });
    expect(contract?.id).toBe(CONTRACT_ID);
  });

  it('getContract returns null when the contract is absent', async () => {
    const fetchImpl = mockData({ contract: null });
    const contract = await getContract(CONTRACT_ID, { fetchImpl });
    expect(contract).toBeNull();
  });

  it('getContract rejects a malformed id before any network call', async () => {
    const fetchImpl = vi.fn();
    const err = await getContract('not-a-contract', { fetchImpl }).catch(
      (e: unknown) => e,
    );
    expect(err).toBeInstanceOf(GraphQLRequestError);
    expect((err as GraphQLRequestError).category).toBe('validation');
    expect((err as GraphQLRequestError).code).toBe('VALIDATION_CONTRACT_ID');
    expect(fetchImpl).not.toHaveBeenCalled();
  });

  it('getEvent returns the event and passes the id as a variable', async () => {
    const fetchImpl = mockData({ event: eventNode });
    const event = await getEvent('42', { fetchImpl });
    expect(event?.id).toBe('42');
    const { body } = firstRequest(fetchImpl);
    expect(body.variables).toEqual({ id: '42' });
  });

  it('getEvent rejects a malformed id before any network call', async () => {
    const fetchImpl = vi.fn();
    const err = await getEvent('-1', { fetchImpl }).catch((e: unknown) => e);
    expect(err).toBeInstanceOf(GraphQLRequestError);
    expect((err as GraphQLRequestError).code).toBe('VALIDATION_EVENT_ID');
    expect(fetchImpl).not.toHaveBeenCalled();
  });

  it('getEvents returns a connection and forwards filter variables', async () => {
    const fetchImpl = mockData({
      events: { items: [eventNode], nextCursor: 'next' },
    });
    const page = await getEvents(
      {
        contractId: CONTRACT_ID,
        name: 'transfer',
        fromLedger: 1000,
        toLedger: 2000,
        topicContains: 'abc',
        limit: 10,
        cursor: 'cur',
        order: 'asc',
      },
      { fetchImpl },
    );
    expect(page.items).toHaveLength(1);
    expect(page.nextCursor).toBe('next');
    const { body } = firstRequest(fetchImpl);
    expect(body.variables).toMatchObject({
      contractId: CONTRACT_ID,
      name: 'transfer',
      fromLedger: 1000,
      toLedger: 2000,
      topicContains: 'abc',
      limit: 10,
      cursor: 'cur',
      order: 'asc',
    });
  });

  it('getEvents rejects a malformed contractId before any network call', async () => {
    const fetchImpl = vi.fn();
    const err = await getEvents({ contractId: 'nope' }, { fetchImpl }).catch(
      (e: unknown) => e,
    );
    expect((err as GraphQLRequestError).code).toBe('VALIDATION_CONTRACT_ID');
    expect(fetchImpl).not.toHaveBeenCalled();
  });

  it('getContractWithEvents nests the event connection under the contract', async () => {
    const fetchImpl = mockData({
      contract: { ...contractNode, events: { items: [eventNode], nextCursor: null } },
    });
    const contract = await getContractWithEvents(
      CONTRACT_ID,
      { limit: 25 },
      { fetchImpl },
    );
    expect(contract?.events?.items).toHaveLength(1);
    expect(contract?.events?.nextCursor).toBeNull();
  });
});
