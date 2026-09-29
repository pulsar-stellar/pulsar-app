import { describe, expect, it } from 'vitest';

import { categoryForCode, GraphQLRequestError } from '@/lib/graphql/errors';

describe('categoryForCode', () => {
  it('maps VALIDATION_* codes to validation', () => {
    expect(categoryForCode('VALIDATION_CONTRACT_ID')).toBe('validation');
    expect(categoryForCode('VALIDATION_LEDGER_RANGE')).toBe('validation');
  });

  it('maps NOT_FOUND_* codes to not_found', () => {
    expect(categoryForCode('NOT_FOUND_CONTRACT')).toBe('not_found');
    expect(categoryForCode('NOT_FOUND_EVENT')).toBe('not_found');
  });

  it('maps INTERNAL_* codes to internal', () => {
    expect(categoryForCode('INTERNAL_STORE')).toBe('internal');
    expect(categoryForCode('INTERNAL_PANIC')).toBe('internal');
  });

  it('falls back to unknown for an absent or unrecognized code', () => {
    expect(categoryForCode(null)).toBe('unknown');
    expect(categoryForCode(undefined)).toBe('unknown');
    expect(categoryForCode('SOMETHING_ELSE')).toBe('unknown');
  });
});

describe('GraphQLRequestError', () => {
  it('carries category, code, and a client-safe message', () => {
    const err = new GraphQLRequestError('contract is not tracked', {
      category: 'not_found',
      code: 'NOT_FOUND_CONTRACT',
    });
    expect(err).toBeInstanceOf(Error);
    expect(err.name).toBe('GraphQLRequestError');
    expect(err.category).toBe('not_found');
    expect(err.code).toBe('NOT_FOUND_CONTRACT');
    expect(err.message).toBe('contract is not tracked');
  });

  it('defaults code to null and preserves a cause', () => {
    const cause = new Error('socket hang up');
    const err = new GraphQLRequestError('Could not reach the indexer', {
      category: 'network',
      cause,
    });
    expect(err.code).toBeNull();
    expect(err.cause).toBe(cause);
  });

  it('exposes isNotFound for the common null-render decision', () => {
    const notFound = new GraphQLRequestError('gone', {
      category: 'not_found',
      code: 'NOT_FOUND_EVENT',
    });
    const other = new GraphQLRequestError('boom', { category: 'internal' });
    expect(notFound.isNotFound).toBe(true);
    expect(other.isNotFound).toBe(false);
  });
});
