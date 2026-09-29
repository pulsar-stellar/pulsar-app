import { describe, expect, it } from 'vitest';
import { z } from 'zod';

import {
  contractNodeSchema,
  eventConnectionSchema,
  eventNodeSchema,
  graphqlErrorSchema,
  graphqlResponseSchema,
  healthSchema,
} from '@/lib/graphql/schemas';

const CONTRACT_ID = 'CDNWTVUDKCCGW7GOC6SBLUFXXUCD2YDHWRDUSXZ6CYBQKQWLCUYYWI5L';

// A fully selected GraphQL Event node. Its shape mirrors the SDK's
// DecodedEventSchema field-for-field (ADR-043 / ADR-046): id as a decimal
// string, camelCase fields, topics/data as the already-decoded taxonomy.
const eventNode = {
  id: '42',
  contractId: CONTRACT_ID,
  ledger: 1234,
  txHash: 'a1b2c3',
  eventIndex: 0,
  name: 'transfer',
  topics: [
    { type: 'symbol', value: 'transfer' },
    { type: 'address', value: CONTRACT_ID },
  ],
  data: { type: 'i128', value: '1000' },
  rawTopics: ['AAAADwAAAAh0cmFuc2Zlcg=='],
  rawData: 'AAAACgAAAAAAAAAAAAAD6A==',
  emittedAt: '2026-09-24T12:00:00Z',
  inSuccessfulContractCall: true,
};

describe('eventNodeSchema (reuses the SDK decoded-event schema)', () => {
  it('accepts a fully decoded event with a string id', () => {
    const parsed = eventNodeSchema.parse(eventNode);
    expect(parsed.id).toBe('42');
    expect(parsed.topics[0]).toEqual({ type: 'symbol', value: 'transfer' });
    expect(parsed.data).toEqual({ type: 'i128', value: '1000' });
  });

  it('rejects an event whose contractId is not a Soroban contract id', () => {
    const bad = { ...eventNode, contractId: 'not-a-contract' };
    expect(eventNodeSchema.safeParse(bad).success).toBe(false);
  });

  it('rejects a topic that is not a valid decoded value', () => {
    const bad = { ...eventNode, topics: [{ type: 'nonsense', value: 'x' }] };
    expect(eventNodeSchema.safeParse(bad).success).toBe(false);
  });

  it('strips unknown keys the indexer may add later', () => {
    const parsed = eventNodeSchema.parse({ ...eventNode, futureField: 'x' });
    expect('futureField' in parsed).toBe(false);
  });
});

describe('eventConnectionSchema', () => {
  it('accepts items with a cursor', () => {
    const parsed = eventConnectionSchema.parse({
      items: [eventNode],
      nextCursor: 'MTIzNA==',
    });
    expect(parsed.items).toHaveLength(1);
    expect(parsed.nextCursor).toBe('MTIzNA==');
  });

  it('accepts an empty page with a null cursor', () => {
    const parsed = eventConnectionSchema.parse({ items: [], nextCursor: null });
    expect(parsed.items).toEqual([]);
    expect(parsed.nextCursor).toBeNull();
  });
});

describe('contractNodeSchema', () => {
  const contractNode = {
    id: CONTRACT_ID,
    addedAt: '2026-09-20T08:00:00Z',
    firstIndexedLedger: 1000,
    lastIndexedLedger: 2000,
    status: 'active',
  };

  it('accepts a contract without nested events', () => {
    const parsed = contractNodeSchema.parse(contractNode);
    expect(parsed.id).toBe(CONTRACT_ID);
    expect(parsed.events).toBeUndefined();
  });

  it('accepts a null firstIndexedLedger (first poll not yet complete)', () => {
    const parsed = contractNodeSchema.parse({
      ...contractNode,
      firstIndexedLedger: null,
    });
    expect(parsed.firstIndexedLedger).toBeNull();
  });

  it('accepts a contract with a nested event connection', () => {
    const parsed = contractNodeSchema.parse({
      ...contractNode,
      events: { items: [eventNode], nextCursor: null },
    });
    expect(parsed.events?.items).toHaveLength(1);
  });

  it('rejects an unknown status', () => {
    const bad = { ...contractNode, status: 'retired' };
    expect(contractNodeSchema.safeParse(bad).success).toBe(false);
  });
});

describe('healthSchema', () => {
  it('accepts a health payload', () => {
    const parsed = healthSchema.parse({
      ok: true,
      version: '0.1.0',
      latestLedger: 5000,
      trackedContracts: 3,
    });
    expect(parsed.ok).toBe(true);
    expect(parsed.trackedContracts).toBe(3);
  });
});

describe('graphqlErrorSchema', () => {
  it('reads the catalog code and class from extensions', () => {
    const parsed = graphqlErrorSchema.parse({
      message: 'contract is not tracked',
      path: ['events'],
      extensions: { code: 'NOT_FOUND_CONTRACT', class: 'not_found' },
    });
    expect(parsed.extensions?.code).toBe('NOT_FOUND_CONTRACT');
    expect(parsed.extensions?.class).toBe('not_found');
  });

  it('accepts an error with no extensions', () => {
    const parsed = graphqlErrorSchema.parse({ message: 'boom' });
    expect(parsed.message).toBe('boom');
    expect(parsed.extensions).toBeUndefined();
  });
});

describe('graphqlResponseSchema', () => {
  const schema = graphqlResponseSchema(z.object({ event: eventNodeSchema.nullable() }));

  it('parses a data-only response', () => {
    const parsed = schema.parse({ data: { event: eventNode } });
    expect(parsed.data?.event?.id).toBe('42');
    expect(parsed.errors).toBeUndefined();
  });

  it('parses a null-data response (looked up, absent)', () => {
    const parsed = schema.parse({ data: { event: null } });
    expect(parsed.data?.event).toBeNull();
  });

  it('parses an errors-only response', () => {
    const parsed = schema.parse({
      data: null,
      errors: [{ message: 'boom', extensions: { code: 'INTERNAL_STORE' } }],
    });
    expect(parsed.errors?.[0]?.extensions?.code).toBe('INTERNAL_STORE');
  });
});
