import { describe, expectTypeOf, it } from 'vitest';

import { PulsarClient } from '../src/client.js';
import type { PingResult } from '../src/types.js';

const client = new PulsarClient({ indexerUrl: 'http://indexer.test' });

describe('ping result shape', () => {
  it('resolves to a PingResult', () => {
    expectTypeOf(client.ping()).resolves.toEqualTypeOf<PingResult>();
  });

  it('types the fields the indexer may omit as null, never undefined', () => {
    expectTypeOf<PingResult['latestLedger']>().toEqualTypeOf<number | null>();
    expectTypeOf<PingResult['trackedContracts']>().toEqualTypeOf<number | null>();
    expectTypeOf<PingResult['serverTookMs']>().toEqualTypeOf<number | null>();
  });

  it('types the fields the indexer always sends as non-nullable', () => {
    expectTypeOf<PingResult['ok']>().toEqualTypeOf<boolean>();
    expectTypeOf<PingResult['version']>().toEqualTypeOf<string>();
    expectTypeOf<PingResult['latencyMs']>().toEqualTypeOf<number>();
  });

  it('exposes the result as readonly, since it is a snapshot of one call', () => {
    expectTypeOf<PingResult>().toEqualTypeOf<Readonly<PingResult>>();
  });

  it('takes no arguments', () => {
    expectTypeOf<Parameters<PulsarClient['ping']>>().toEqualTypeOf<[]>();
  });
});
