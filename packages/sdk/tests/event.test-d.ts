import { describe, expectTypeOf, it } from 'vitest';

import { PulsarClient } from '../src/client.js';
import type { DecodedEvent } from '../src/types.js';

const client = new PulsarClient({ indexerUrl: 'http://indexer.test' });

describe('event signature', () => {
  it('takes only an event id, since ids are unique across the indexer', () => {
    expectTypeOf<Parameters<PulsarClient['event']>>().toEqualTypeOf<[string]>();
  });

  it('resolves to a decoded event or null, never undefined', () => {
    expectTypeOf(client.event('1')).resolves.toEqualTypeOf<DecodedEvent | null>();
  });

  it('types the id as a string, so a numeric id cannot be passed', () => {
    // @ts-expect-error event ids are strings: a number loses precision past 2^53.
    void client.event(42);
  });
});
