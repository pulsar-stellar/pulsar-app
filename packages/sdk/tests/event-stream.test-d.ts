import { describe, expectTypeOf, it } from 'vitest';

import { PulsarClient } from '../src/client.js';
import type { DecodedEvent, EventQuery } from '../src/types.js';

const client = new PulsarClient({ indexerUrl: 'http://indexer.test' });
const SHOWCASE_ID = 'CDNWTVUDKCCGW7GOC6SBLUFXXUCD2YDHWRDUSXZ6CYBQKQWLCUYYWI5L';

describe('eventStream signature', () => {
  it('takes the same arguments as events', () => {
    expectTypeOf<Parameters<PulsarClient['eventStream']>>().toEqualTypeOf<
      [string, (EventQuery | undefined)?]
    >();
  });

  it('returns an iterable synchronously, not a promise', () => {
    expectTypeOf(client.eventStream(SHOWCASE_ID)).toEqualTypeOf<AsyncIterable<DecodedEvent>>();
  });

  it('yields decoded events', async () => {
    for await (const event of client.eventStream(SHOWCASE_ID)) {
      expectTypeOf(event).toEqualTypeOf<DecodedEvent>();
    }
  });
});
