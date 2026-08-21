import { describe, expect, it } from 'vitest';
import { z } from 'zod';

import { PulsarClient } from '../src/client.js';
import { PulsarError, PulsarValidationError } from '../src/errors.js';
import { DEFAULT_TIMEOUT_MS } from '../src/types.js';

describe('construction with valid configuration', () => {
  it('fills in the defaults the caller omitted', () => {
    const client = new PulsarClient({ indexerUrl: 'http://localhost:8080' });
    expect(client.config.timeoutMs).toBe(DEFAULT_TIMEOUT_MS);
    expect(client.config.indexerUrl).toBe('http://localhost:8080');
  });

  it('keeps the values the caller supplied', () => {
    const client = new PulsarClient({
      indexerUrl: 'https://indexer.example.com',
      rpcUrl: 'https://soroban-testnet.stellar.org',
      network: 'testnet',
      timeoutMs: 500,
    });
    expect(client.config.rpcUrl).toBe('https://soroban-testnet.stellar.org');
    expect(client.config.network).toBe('testnet');
    expect(client.config.timeoutMs).toBe(500);
  });

  it('accepts a caller-supplied fetch implementation', () => {
    const client = new PulsarClient({
      indexerUrl: 'http://localhost:8080',
      fetchImpl: globalThis.fetch,
    });
    expect(typeof client.config.fetchImpl).toBe('function');
  });
});

describe('construction with a missing required field', () => {
  it('throws a PulsarValidationError when indexerUrl is absent', () => {
    expect(() => new PulsarClient({} as never)).toThrow(PulsarValidationError);
  });

  it('reports the offending field in the message', () => {
    try {
      new PulsarClient({} as never);
      expect.unreachable('constructor should have thrown');
    } catch (error) {
      expect(error).toBeInstanceOf(PulsarValidationError);
      expect((error as PulsarValidationError).message).toContain('indexerUrl');
    }
  });
});

describe('construction with a malformed indexer URL', () => {
  it('rejects a URL with no scheme, which would fail at the first request', () => {
    expect(() => new PulsarClient({ indexerUrl: 'localhost:8080' })).toThrow(
      PulsarValidationError,
    );
  });

  it('exposes the schema issue rather than only a message', () => {
    try {
      new PulsarClient({ indexerUrl: 'localhost:8080' });
      expect.unreachable('constructor should have thrown');
    } catch (error) {
      const issues = (error as PulsarValidationError).issues;
      expect(issues).toHaveLength(1);
      expect(issues[0]?.path.join('.')).toBe('indexerUrl');
      expect(issues[0]?.message).toContain('http or https');
    }
  });
});

describe('construction with an invalid network', () => {
  it('rejects a network outside the enum', () => {
    expect(
      () => new PulsarClient({ indexerUrl: 'http://localhost:8080', network: 'Testnet' as never }),
    ).toThrow(PulsarValidationError);
  });

  it('names the expected options in the issue', () => {
    try {
      new PulsarClient({ indexerUrl: 'http://localhost:8080', network: 'Testnet' as never });
      expect.unreachable('constructor should have thrown');
    } catch (error) {
      const [issue] = (error as PulsarValidationError).issues;
      expect(issue?.path.join('.')).toBe('network');
      expect(issue?.message).toContain('testnet');
    }
  });

  it('rejects an unknown configuration key rather than ignoring it', () => {
    expect(
      () => new PulsarClient({ indexerUrl: 'http://localhost:8080', retries: 3 } as never),
    ).toThrow(PulsarValidationError);
  });
});

describe('the thrown error carries usable context', () => {
  it('names the operation that failed', () => {
    try {
      new PulsarClient({ indexerUrl: 'localhost:8080' });
      expect.unreachable('constructor should have thrown');
    } catch (error) {
      expect((error as PulsarValidationError).operation).toBe('configureClient');
    }
  });

  it('keeps the underlying ZodError as cause', () => {
    try {
      new PulsarClient({ indexerUrl: 'localhost:8080' });
      expect.unreachable('constructor should have thrown');
    } catch (error) {
      expect((error as PulsarValidationError).cause).toBeInstanceOf(z.ZodError);
    }
  });

  it('is catchable as a PulsarError by a caller handling every SDK failure', () => {
    expect(() => new PulsarClient({ indexerUrl: 'localhost:8080' })).toThrow(PulsarError);
  });

  it('reports every issue at once rather than only the first', () => {
    try {
      new PulsarClient({ indexerUrl: 'localhost:8080', network: 'Testnet' as never } as never);
      expect.unreachable('constructor should have thrown');
    } catch (error) {
      expect((error as PulsarValidationError).issues.length).toBeGreaterThan(1);
    }
  });
});

describe('the stored configuration is frozen', () => {
  it('cannot be mutated after construction', () => {
    const client = new PulsarClient({ indexerUrl: 'http://localhost:8080' });
    expect(Object.isFrozen(client.config)).toBe(true);
    expect(() => {
      (client.config as { timeoutMs: number }).timeoutMs = 1;
    }).toThrow(TypeError);
    expect(client.config.timeoutMs).toBe(DEFAULT_TIMEOUT_MS);
  });

  it('is a parsed copy, so mutating the input object cannot change it', () => {
    const input = { indexerUrl: 'http://localhost:8080', timeoutMs: 900 };
    const client = new PulsarClient(input);
    input.timeoutMs = 1;
    expect(client.config.timeoutMs).toBe(900);
  });
});
