import { assertType, describe, expectTypeOf, it } from 'vitest';

import {
  findPulsarError,
  PulsarError,
  PulsarNetworkError,
  type PulsarValidationError,
} from '../src/errors.js';

describe('PulsarError is abstract', () => {
  it('cannot be constructed directly', () => {
    // @ts-expect-error PulsarError is abstract: throw a subclass that says what failed.
    new PulsarError('should not compile', { operation: 'o' });
  });

  it('is still usable as a catch-all type', () => {
    expectTypeOf<PulsarNetworkError>().toExtend<PulsarError>();
    expectTypeOf<PulsarValidationError>().toExtend<PulsarError>();
  });
});

describe('absence is null, never undefined', () => {
  it('types findPulsarError as returning null rather than undefined', () => {
    expectTypeOf(findPulsarError(new Error('x'))).toEqualTypeOf<PulsarError | null>();
  });

  it('types an absent HTTP status as null', () => {
    expectTypeOf<PulsarNetworkError['status']>().toEqualTypeOf<number | null>();
    expectTypeOf<PulsarNetworkError['url']>().toEqualTypeOf<string | null>();
  });
});

describe('constructor contracts', () => {
  it('requires an operation on every error', () => {
    // @ts-expect-error operation is required: an error with no operation cannot say what failed.
    new PulsarNetworkError('missing operation', {});
  });

  it('accepts an unknown cause, since a caught value is not always an Error', () => {
    assertType<PulsarNetworkError>(
      new PulsarNetworkError('wrapped', { operation: 'o', cause: 'a thrown string' }),
    );
  });

  it('exposes issues as readonly', () => {
    expectTypeOf<PulsarValidationError['issues']>().toExtend<readonly unknown[]>();
  });
});
