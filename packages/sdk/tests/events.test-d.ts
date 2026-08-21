import { describe, expectTypeOf, it } from 'vitest';

import { PulsarClient } from '../src/client.js';
import type { DecodedEvent, EventQuery, EventsPage } from '../src/types.js';

const client = new PulsarClient({ indexerUrl: 'http://indexer.test' });
const SHOWCASE_ID = 'CDNWTVUDKCCGW7GOC6SBLUFXXUCD2YDHWRDUSXZ6CYBQKQWLCUYYWI5L';

describe('events signature', () => {
  it('takes a contract ID and an optional query', () => {
    expectTypeOf<Parameters<PulsarClient['events']>>().toEqualTypeOf<
      [string, (EventQuery | undefined)?]
    >();
  });

  it('resolves to a page that is never null', () => {
    expectTypeOf(client.events(SHOWCASE_ID)).resolves.toEqualTypeOf<EventsPage>();
  });
});

describe('EventsPage shape', () => {
  it('types the cursor as string or null, never undefined', () => {
    expectTypeOf<EventsPage['nextCursor']>().toEqualTypeOf<string | null>();
  });

  it('types items as a plain array of decoded events', () => {
    expectTypeOf<EventsPage['items']>().toEqualTypeOf<DecodedEvent[]>();
  });

  it('keeps the contract out of the query, since it is a path parameter', () => {
    // @ts-expect-error contractId is the first argument to events, not a filter.
    const _query: EventQuery = { contractId: SHOWCASE_ID };
  });
});
