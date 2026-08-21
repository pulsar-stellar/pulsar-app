import { describe, expectTypeOf, it } from 'vitest';

import { PulsarClient } from '../src/client.js';
import type { ContractInfo } from '../src/types.js';

const client = new PulsarClient({ indexerUrl: 'http://indexer.test' });
const SHOWCASE_ID = 'CDNWTVUDKCCGW7GOC6SBLUFXXUCD2YDHWRDUSXZ6CYBQKQWLCUYYWI5L';

describe('getContract return type', () => {
  it('resolves to ContractInfo or null, never undefined', () => {
    expectTypeOf(client.getContract(SHOWCASE_ID)).resolves.toEqualTypeOf<ContractInfo | null>();
  });
});

describe('listContracts return type', () => {
  it('resolves to a plain array, with no pagination wrapper', () => {
    expectTypeOf(client.listContracts()).resolves.toEqualTypeOf<ContractInfo[]>();
  });

  it('takes no arguments', () => {
    expectTypeOf<Parameters<PulsarClient['listContracts']>>().toEqualTypeOf<[]>();
  });
});
