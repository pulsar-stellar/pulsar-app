/**
 * Pins the package's public surface.
 *
 * `index.ts` is the boundary: what it exports is the API, and what it does not
 * is internal and free to change. That distinction is only worth anything if a
 * change to it is visible, so this test lists the surface exactly. Adding an
 * export means adding it here, which is the point at which someone decides
 * whether it should be public.
 *
 * The absence assertions matter as much as the presence ones. A barrel file
 * accumulates by accident, one convenient re-export at a time, and each one is
 * a promise that is hard to withdraw later.
 */

import { describe, expect, it } from 'vitest';

import * as sdk from '../src/index.js';

/** Every runtime value the package exports. Types are checked in index.test-d.ts. */
const PUBLIC_SURFACE = [
  'ContractIdSchema',
  'ContractInfoSchema',
  'ContractStatusSchema',
  'DecodedEventSchema',
  'DecodedMapEntrySchema',
  'DecodedValueSchema',
  'DEFAULT_CALL_TIMEOUT_SECONDS',
  'DEFAULT_POLL_INTERVAL_MS',
  'DEFAULT_TIMEOUT_MS',
  'EVENT_QUERY_DEFAULT_LIMIT',
  'EVENT_QUERY_MAX_LIMIT',
  'EventIdSchema',
  'EventQuerySchema',
  'LiveEventFilterSchema',
  'LiveEventQuerySchema',
  'PulsarClient',
  'PulsarConfigSchema',
  'PulsarError',
  'PulsarNetworkError',
  'PulsarNetworkSchema',
  'PulsarValidationError',
  'RPC_ID_PREFIX',
  'asAdminChangeEvent',
  'asDepositEvent',
  'asEmitCustomEvent',
  'asInitializeEvent',
  'asTransferEvent',
  'asWithdrawEvent',
  'buildContractCall',
  'decodeScVal',
  'decodeTopics',
  'eventNameFromTopics',
  'fetchLiveEvents',
  'findPulsarError',
  'liveEventStream',
  'parseTopics',
  'scValToNative',
] as const;

describe('the published surface', () => {
  it('exports exactly what it means to', () => {
    expect(Object.keys(sdk).sort()).toEqual([...PUBLIC_SURFACE].sort());
  });

  it('exports nothing undefined, which a bad re-export would produce', () => {
    for (const name of PUBLIC_SURFACE) {
      expect(sdk[name]).toBeDefined();
    }
  });

  /**
   * These are internal on purpose, and each has a reason recorded in index.ts.
   * The transport is how the SDK talks to the indexer; the payload schemas and
   * mappers describe the snake_case wire shapes between the two; the envelope
   * is ADR-017's contract, which the SDK unwraps so nobody else has to know it.
   */
  it.each([
    'request',
    'requestMaybe',
    'toDecodedEvent',
    'toContractInfo',
    'toLiveDecodedEvent',
    'DecodedEventPayloadSchema',
    'ContractInfoPayloadSchema',
    'ContractListPayloadSchema',
    'EventListPayloadSchema',
    'HealthPayloadSchema',
    'EnvelopeSchema',
    'ErrorEnvelopeSchema',
  ])('keeps %s internal', (name) => {
    expect(Object.keys(sdk)).not.toContain(name);
  });
});

describe('the surface is usable as exported', () => {
  it('constructs a client through the public entry point', () => {
    const client = new sdk.PulsarClient({ indexerUrl: 'https://indexer.example' });

    expect(client.config.indexerUrl).toBe('https://indexer.example');
  });

  it('exposes the error hierarchy so a consumer can branch on it', () => {
    expect(sdk.PulsarNetworkError.prototype).toBeInstanceOf(sdk.PulsarError);
    expect(sdk.PulsarValidationError.prototype).toBeInstanceOf(sdk.PulsarError);
  });

  it('exposes the schemas a consumer validates against', () => {
    expect(sdk.ContractIdSchema.safeParse('not-a-contract').success).toBe(false);
    expect(sdk.EventIdSchema.safeParse('42').success).toBe(true);
  });
});
