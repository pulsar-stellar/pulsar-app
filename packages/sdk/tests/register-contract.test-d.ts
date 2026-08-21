import { describe, expectTypeOf, it } from 'vitest';

import { PulsarClient } from '../src/client.js';
import type { ContractInfo } from '../src/types.js';

const client = new PulsarClient({ indexerUrl: 'http://indexer.test' });

describe('registerContract signature', () => {
  it('takes a contract ID string and resolves to ContractInfo', () => {
    expectTypeOf<Parameters<PulsarClient['registerContract']>>().toEqualTypeOf<[string]>();
    expectTypeOf(
      client.registerContract('CDNWTVUDKCCGW7GOC6SBLUFXXUCD2YDHWRDUSXZ6CYBQKQWLCUYYWI5L'),
    ).resolves.toEqualTypeOf<ContractInfo>();
  });

  it('reports an unindexed contract as null rather than undefined', () => {
    expectTypeOf<ContractInfo['firstIndexedLedger']>().toEqualTypeOf<number | null>();
  });
});
