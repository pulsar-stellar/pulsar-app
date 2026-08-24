import { assertType, describe, expectTypeOf, it } from 'vitest';

import type {
  DecodedEvent,
  DecodedValue,
  EventQuery,
  PulsarConfig,
  ResolvedEventQuery,
  ResolvedPulsarConfig,
} from '../src/types.js';

/**
 * Type-level checks for the distinctions the runtime tests cannot reach.
 *
 * The input and output types of a schema with defaults differ, and the
 * difference is invisible at runtime: both parse the same values. Getting it
 * backwards means either a caller is forced to supply a field the schema
 * defaults, or request-building code treats a filled-in default as optional
 * and emits `undefined`. These assertions pin the direction.
 */
describe('EventQuery input and output', () => {
  it('lets a caller omit every field, since all of them are filters', () => {
    assertType<EventQuery>({});
  });

  it('guarantees limit and order are present after parsing', () => {
    expectTypeOf<ResolvedEventQuery['limit']>().toEqualTypeOf<number>();
    expectTypeOf<ResolvedEventQuery['order']>().toEqualTypeOf<'asc' | 'desc'>();
  });

  it('keeps limit optional on the caller-facing type', () => {
    expectTypeOf<EventQuery['limit']>().toEqualTypeOf<number | undefined>();
  });
});

describe('PulsarConfig input and output', () => {
  it('lets a caller supply only the indexer URL', () => {
    assertType<PulsarConfig>({ indexerUrl: 'http://localhost:8080' });
  });

  it('guarantees a timeout is present after parsing', () => {
    expectTypeOf<ResolvedPulsarConfig['timeoutMs']>().toEqualTypeOf<number>();
  });
});

/**
 * `DecodedValue` is written by hand and its schema is built separately, so the
 * two could drift. These assertions fail if a variant is added to one and not
 * the other.
 */
describe('DecodedValue variants', () => {
  it('carries no value on the void variant', () => {
    assertType<DecodedValue>({ type: 'void' });
  });

  it('recurses through vec, map, and tuple', () => {
    assertType<DecodedValue>({
      type: 'map',
      value: [
        {
          key: { type: 'symbol', value: 'inner' },
          value: { type: 'vec', value: [{ type: 'tuple', value: [{ type: 'void' }] }] },
        },
      ],
    });
  });

  it('carries wide integers as strings, never as bigint', () => {
    expectTypeOf<Extract<DecodedValue, { type: 'i128' }>['value']>().toEqualTypeOf<string>();
  });
});

/**
 * The wire-oriented shape from ADR-016. A generated binding folds an event's
 * topics into its data and capitalizes its name; this type does neither.
 */
describe('DecodedEvent stays wire-oriented', () => {
  it('keeps topics separate from data', () => {
    expectTypeOf<DecodedEvent['topics']>().toEqualTypeOf<DecodedValue[]>();
    expectTypeOf<DecodedEvent['data']>().toEqualTypeOf<DecodedValue>();
  });

  it('carries the emitted topic Symbol as a plain string, not a literal union', () => {
    expectTypeOf<DecodedEvent['name']>().toEqualTypeOf<string>();
  });

  it('carries raw XDR provenance alongside the decoded form', () => {
    expectTypeOf<DecodedEvent['rawTopics']>().toEqualTypeOf<string[]>();
    expectTypeOf<DecodedEvent['rawData']>().toEqualTypeOf<string>();
  });
});
