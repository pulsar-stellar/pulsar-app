import { assertType, describe, expectTypeOf, it } from 'vitest';

import { PulsarClient } from '../src/client.js';
import type { PulsarConfig, ResolvedPulsarConfig } from '../src/types.js';

/**
 * The constructor takes the input type and exposes the output type. Getting
 * that backwards is invisible at runtime, since both parse the same values,
 * but it would either force callers to pass fields the schema defaults or make
 * every method re-check a value that is already guaranteed.
 */
describe('the config path uses input on the way in and output on the way out', () => {
  it('accepts a config with the defaulted fields omitted', () => {
    assertType<PulsarClient>(new PulsarClient({ indexerUrl: 'http://localhost:8080' }));
  });

  it('takes the caller-facing input type', () => {
    expectTypeOf(PulsarClient).toBeConstructibleWith({ indexerUrl: 'http://localhost:8080' });
    expectTypeOf<ConstructorParameters<typeof PulsarClient>[0]>().toEqualTypeOf<PulsarConfig>();
  });

  it('exposes the resolved type, with defaults no longer optional', () => {
    expectTypeOf<PulsarClient['config']>().toEqualTypeOf<ResolvedPulsarConfig>();
    expectTypeOf<PulsarClient['config']['timeoutMs']>().toEqualTypeOf<number>();
  });

  it('leaves genuinely optional fields optional after parsing', () => {
    expectTypeOf<PulsarClient['config']['rpcUrl']>().toEqualTypeOf<string | undefined>();
  });

  it('rejects a config missing the one required field', () => {
    // @ts-expect-error indexerUrl is required: a client with no indexer has nothing to call.
    new PulsarClient({});
  });

  it('rejects an unknown configuration key at compile time as well as runtime', () => {
    // @ts-expect-error retries is not a configuration option.
    new PulsarClient({ indexerUrl: 'http://localhost:8080', retries: 3 });
  });
});
