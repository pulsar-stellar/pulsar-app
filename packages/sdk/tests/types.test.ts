import { describe, expect, it } from 'vitest';

import {
  ContractIdSchema,
  ContractInfoSchema,
  DecodedEventSchema,
  DecodedValueSchema,
  DEFAULT_TIMEOUT_MS,
  EVENT_QUERY_DEFAULT_LIMIT,
  EVENT_QUERY_MAX_LIMIT,
  EventQuerySchema,
  PulsarConfigSchema,
  type DecodedValue,
} from '../src/types.js';

/** The pulsar-core v0.1.0-contracts showcase contract, deployed to testnet. */
const SHOWCASE_ID = 'CDNWTVUDKCCGW7GOC6SBLUFXXUCD2YDHWRDUSXZ6CYBQKQWLCUYYWI5L';

const validEvent = {
  id: '42',
  contractId: SHOWCASE_ID,
  ledger: 1_234_567,
  txHash: 'ab'.repeat(32),
  eventIndex: 0,
  name: 'deposit',
  topics: [
    { type: 'symbol', value: 'deposit' },
    { type: 'address', value: SHOWCASE_ID },
  ],
  data: { type: 'i128', value: '1000000000' },
  rawTopics: ['AAAADwAAAAdkZXBvc2l0AA=='],
  rawData: 'AAAACgAAAAAAAAAAAAAAADuaygA=',
  emittedAt: '2026-08-20T12:00:00Z',
};

describe('ContractIdSchema', () => {
  it('accepts a real testnet contract ID', () => {
    expect(ContractIdSchema.parse(SHOWCASE_ID)).toBe(SHOWCASE_ID);
  });

  it('rejects an account ID, which starts with G rather than C', () => {
    const accountId = `G${SHOWCASE_ID.slice(1)}`;
    expect(ContractIdSchema.safeParse(accountId).success).toBe(false);
  });

  it('rejects a truncated contract ID', () => {
    expect(ContractIdSchema.safeParse(SHOWCASE_ID.slice(0, 55)).success).toBe(false);
  });

  it('rejects a character outside the base32 alphabet', () => {
    const withZero = `C0${SHOWCASE_ID.slice(2)}`;
    expect(ContractIdSchema.safeParse(withZero).success).toBe(false);
  });
});

describe('DecodedValueSchema', () => {
  it('accepts a void value carrying no payload', () => {
    expect(DecodedValueSchema.parse({ type: 'void' })).toEqual({ type: 'void' });
  });

  it('accepts values nested three levels deep', () => {
    const nested: DecodedValue = {
      type: 'vec',
      value: [
        {
          type: 'map',
          value: {
            amounts: { type: 'tuple', value: [{ type: 'i128', value: '1' }] },
          },
        },
      ],
    };
    expect(DecodedValueSchema.parse(nested)).toEqual(nested);
  });

  it('rejects a variant it does not know', () => {
    expect(DecodedValueSchema.safeParse({ type: 'u256', value: '1' }).success).toBe(false);
  });

  it('rejects a bool carrying a string', () => {
    expect(DecodedValueSchema.safeParse({ type: 'bool', value: 'true' }).success).toBe(false);
  });

  it('rejects a malformed value nested inside a valid container', () => {
    const bad = { type: 'vec', value: [{ type: 'i128', value: 1 }] };
    expect(DecodedValueSchema.safeParse(bad).success).toBe(false);
  });
});

describe('DecodedEventSchema', () => {
  it('accepts an event shaped as the indexer serves it', () => {
    expect(DecodedEventSchema.parse(validEvent)).toMatchObject({ name: 'deposit' });
  });

  it('strips a field the SDK does not know, so a newer indexer stays readable', () => {
    const parsed = DecodedEventSchema.parse({ ...validEvent, futureField: 'ignored' });
    expect(parsed).not.toHaveProperty('futureField');
    expect(parsed.name).toBe('deposit');
  });

  it('rejects a timestamp that is not ISO 8601', () => {
    const bad = { ...validEvent, emittedAt: '2026-08-20 12:00:00' };
    expect(DecodedEventSchema.safeParse(bad).success).toBe(false);
  });

  it('rejects a negative ledger', () => {
    expect(DecodedEventSchema.safeParse({ ...validEvent, ledger: -1 }).success).toBe(false);
  });

  it('rejects an event whose contract ID is malformed', () => {
    const bad = { ...validEvent, contractId: 'not-a-contract' };
    expect(DecodedEventSchema.safeParse(bad).success).toBe(false);
  });

  it('rejects an event missing its raw XDR provenance', () => {
    const { rawData: _omitted, ...withoutRaw } = validEvent;
    expect(DecodedEventSchema.safeParse(withoutRaw).success).toBe(false);
  });
});

describe('ContractInfoSchema', () => {
  it('accepts a null firstIndexedLedger before the first poll completes', () => {
    const parsed = ContractInfoSchema.parse({
      contractId: SHOWCASE_ID,
      addedAt: '2026-08-20T12:00:00Z',
      firstIndexedLedger: null,
      lastIndexedLedger: 0,
      status: 'active',
    });
    expect(parsed.firstIndexedLedger).toBeNull();
  });

  it('rejects a status the indexer never reports', () => {
    const bad = {
      contractId: SHOWCASE_ID,
      addedAt: '2026-08-20T12:00:00Z',
      firstIndexedLedger: 1,
      lastIndexedLedger: 2,
      status: 'stopped',
    };
    expect(ContractInfoSchema.safeParse(bad).success).toBe(false);
  });
});

describe('EventQuerySchema', () => {
  it('fills in the default limit and order', () => {
    const parsed = EventQuerySchema.parse({ contractId: SHOWCASE_ID });
    expect(parsed.limit).toBe(EVENT_QUERY_DEFAULT_LIMIT);
    expect(parsed.order).toBe('desc');
  });

  it('rejects an unknown key rather than ignoring a typo', () => {
    const result = EventQuerySchema.safeParse({ contractId: SHOWCASE_ID, contractid: 'x' });
    expect(result.success).toBe(false);
  });

  it('rejects a limit above the maximum page size', () => {
    const bad = { contractId: SHOWCASE_ID, limit: EVENT_QUERY_MAX_LIMIT + 1 };
    expect(EventQuerySchema.safeParse(bad).success).toBe(false);
  });

  it('rejects a limit of zero', () => {
    expect(EventQuerySchema.safeParse({ contractId: SHOWCASE_ID, limit: 0 }).success).toBe(false);
  });

  it('accepts a cursor alongside a ledger range, unlike Soroban RPC getEvents', () => {
    const parsed = EventQuerySchema.parse({
      contractId: SHOWCASE_ID,
      cursor: 'opaque-cursor',
      fromLedger: 100,
      toLedger: 200,
    });
    expect(parsed.cursor).toBe('opaque-cursor');
    expect(parsed.fromLedger).toBe(100);
  });
});

describe('PulsarConfigSchema', () => {
  it('fills in the default timeout', () => {
    const parsed = PulsarConfigSchema.parse({ indexerUrl: 'http://localhost:8080' });
    expect(parsed.timeoutMs).toBe(DEFAULT_TIMEOUT_MS);
  });

  it('rejects an indexer URL that is not absolute', () => {
    expect(PulsarConfigSchema.safeParse({ indexerUrl: '/api' }).success).toBe(false);
  });

  it('rejects an unknown configuration key', () => {
    const bad = { indexerUrl: 'http://localhost:8080', retries: 3 };
    expect(PulsarConfigSchema.safeParse(bad).success).toBe(false);
  });

  it('accepts a supplied fetch implementation', () => {
    const parsed = PulsarConfigSchema.parse({
      indexerUrl: 'http://localhost:8080',
      fetchImpl: globalThis.fetch,
    });
    expect(typeof parsed.fetchImpl).toBe('function');
  });

  it('rejects a fetch implementation that is not callable', () => {
    const bad = { indexerUrl: 'http://localhost:8080', fetchImpl: {} };
    expect(PulsarConfigSchema.safeParse(bad).success).toBe(false);
  });

  it('rejects a non-positive timeout', () => {
    const bad = { indexerUrl: 'http://localhost:8080', timeoutMs: 0 };
    expect(PulsarConfigSchema.safeParse(bad).success).toBe(false);
  });
});
