import { describe, expect, it } from 'vitest';
import { z } from 'zod';

import {
  findPulsarError,
  PulsarError,
  PulsarNetworkError,
  PulsarValidationError,
} from '../src/errors.js';

const SHOWCASE_ID = 'CDNWTVUDKCCGW7GOC6SBLUFXXUCD2YDHWRDUSXZ6CYBQKQWLCUYYWI5L';

describe('the instanceof chain', () => {
  it('makes a network error catchable as a PulsarError and as an Error', () => {
    const error = new PulsarNetworkError('indexer unreachable', { operation: 'client.ping' });
    expect(error).toBeInstanceOf(PulsarNetworkError);
    expect(error).toBeInstanceOf(PulsarError);
    expect(error).toBeInstanceOf(Error);
  });

  it('makes a validation error catchable as a PulsarError and as an Error', () => {
    const error = new PulsarValidationError('bad shape', { operation: 'events.query' });
    expect(error).toBeInstanceOf(PulsarValidationError);
    expect(error).toBeInstanceOf(PulsarError);
    expect(error).toBeInstanceOf(Error);
  });

  it('keeps the two subclasses distinguishable from each other', () => {
    const network = new PulsarNetworkError('timeout', { operation: 'client.ping' });
    expect(network).not.toBeInstanceOf(PulsarValidationError);
  });

  it('names each class after itself, so a log line says which failure it was', () => {
    expect(new PulsarNetworkError('x', { operation: 'o' }).name).toBe('PulsarNetworkError');
    expect(new PulsarValidationError('x', { operation: 'o' }).name).toBe('PulsarValidationError');
  });
});

describe('message and context quality', () => {
  it('renders the operation and identifiers alongside the message', () => {
    const error = new PulsarNetworkError('indexer returned 503', {
      operation: 'events.query',
      status: 503,
      url: 'http://localhost:8080/events',
      details: { contractId: SHOWCASE_ID },
    });

    const rendered = error.toString();
    expect(rendered).toContain('PulsarNetworkError');
    expect(rendered).toContain('indexer returned 503');
    expect(rendered).toContain('operation=events.query');
    expect(rendered).toContain(`contractId=${SHOWCASE_ID}`);
    expect(rendered).toContain('status=503');
  });

  it('renders cleanly when there are no identifiers to show', () => {
    const error = new PulsarNetworkError('timed out', { operation: 'client.ping' });
    expect(error.toString()).toBe('PulsarNetworkError: timed out [operation=client.ping]');
  });

  it('reports null rather than undefined for an absent HTTP status', () => {
    const error = new PulsarNetworkError('connection refused', { operation: 'client.ping' });
    expect(error.status).toBeNull();
    expect(error.url).toBeNull();
  });

  it('exposes the status and URL as typed properties, not only in the message', () => {
    const error = new PulsarNetworkError('not found', {
      operation: 'contracts.get',
      status: 404,
      url: 'http://localhost:8080/contracts/x',
    });
    expect(error.status).toBe(404);
    expect(error.url).toBe('http://localhost:8080/contracts/x');
  });

  it('freezes details so a handler cannot rewrite the record of what happened', () => {
    const error = new PulsarNetworkError('x', { operation: 'o', details: { attempt: 1 } });
    expect(Object.isFrozen(error.details)).toBe(true);
  });
});

describe('cause chains', () => {
  it('preserves the underlying error as cause', () => {
    const underlying = new TypeError('fetch failed');
    const error = new PulsarNetworkError('indexer unreachable', {
      operation: 'client.ping',
      cause: underlying,
    });
    expect(error.cause).toBe(underlying);
  });

  it('leaves cause undefined when nothing was wrapped', () => {
    const error = new PulsarNetworkError('no cause here', { operation: 'client.ping' });
    expect(error.cause).toBeUndefined();
  });

  it('lets a consumer walk a chain the SDK did not build', () => {
    const sdkError = new PulsarNetworkError('indexer unreachable', { operation: 'client.ping' });
    const wrapped = new Error('loading dashboard failed', { cause: sdkError });
    expect(findPulsarError(wrapped)).toBe(sdkError);
  });

  it('finds an SDK error nested two levels down', () => {
    const sdkError = new PulsarValidationError('bad shape', { operation: 'events.query' });
    const middle = new Error('parsing page failed', { cause: sdkError });
    const outer = new Error('render failed', { cause: middle });
    expect(findPulsarError(outer)).toBe(sdkError);
  });
});

describe('findPulsarError returns null and never undefined', () => {
  it.each([
    ['a plain error with no cause', new Error('unrelated')],
    ['undefined', undefined],
    ['null', null],
    ['a string', 'not an error'],
    ['a number', 42],
    ['a plain object carrying a cause key', { cause: new Error('x') }],
  ])('returns exactly null for %s', (_label, input) => {
    const result = findPulsarError(input);
    expect(result).toBeNull();
    expect(result).not.toBeUndefined();
  });

  it('terminates on a cause cycle instead of looping forever', () => {
    const a = new Error('a');
    const b = new Error('b', { cause: a });
    Object.defineProperty(a, 'cause', { value: b, configurable: true });
    expect(findPulsarError(a)).toBeNull();
  });

  it('returns the error itself when it is already a Pulsar error', () => {
    const error = new PulsarNetworkError('x', { operation: 'o' });
    expect(findPulsarError(error)).toBe(error);
  });
});

describe('PulsarValidationError.fromZodError', () => {
  const schema = z.object({ contractId: z.string().min(56), ledger: z.number().int() });

  it('exposes every issue Zod reported', () => {
    const result = schema.safeParse({ contractId: 'short', ledger: 1.5 });
    expect(result.success).toBe(false);
    if (result.success) return;

    const error = PulsarValidationError.fromZodError(result.error, { operation: 'events.query' });
    expect(error.issues).toHaveLength(result.error.issues.length);
    expect(error.issues.map((issue) => issue.path.join('.'))).toEqual(['contractId', 'ledger']);
  });

  it('names the first offending path in the message', () => {
    const result = schema.safeParse({ contractId: 'short', ledger: 1 });
    if (result.success) return;

    const error = PulsarValidationError.fromZodError(result.error, { operation: 'events.query' });
    expect(error.message).toContain('contractId');
    expect(error.message).toContain('1 issue,');
  });

  it('pluralizes the issue count', () => {
    const result = schema.safeParse({ contractId: 'short', ledger: 1.5 });
    if (result.success) return;

    const error = PulsarValidationError.fromZodError(result.error, { operation: 'events.query' });
    expect(error.message).toContain('2 issues,');
  });

  it('keeps the ZodError as cause so the original is not lost', () => {
    const result = schema.safeParse({ contractId: 'short', ledger: 1 });
    if (result.success) return;

    const error = PulsarValidationError.fromZodError(result.error, { operation: 'events.query' });
    expect(error.cause).toBe(result.error);
    expect(error.cause).toBeInstanceOf(z.ZodError);
  });

  it('carries the operation and details through', () => {
    const result = schema.safeParse({ contractId: 'short', ledger: 1 });
    if (result.success) return;

    const error = PulsarValidationError.fromZodError(result.error, {
      operation: 'events.query',
      details: { contractId: SHOWCASE_ID },
    });
    expect(error.operation).toBe('events.query');
    expect(error.toString()).toContain(`contractId=${SHOWCASE_ID}`);
  });

  it('degrades to a plain message when a ZodError carries no issues', () => {
    const error = PulsarValidationError.fromZodError(new z.ZodError([]), {
      operation: 'events.query',
    });
    expect(error.message).toContain('no issues reported');
    expect(error.issues).toHaveLength(0);
  });

  it('freezes the issue list', () => {
    const result = schema.safeParse({ contractId: 'short', ledger: 1 });
    if (result.success) return;

    const error = PulsarValidationError.fromZodError(result.error, { operation: 'events.query' });
    expect(Object.isFrozen(error.issues)).toBe(true);
  });
});
