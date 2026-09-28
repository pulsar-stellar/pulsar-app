/**
 * Write-path authentication.
 *
 * The indexer gates its write routes behind a static bearer token (ADR-044),
 * returning a 401 carrying the `unauthorized` wire class. These tests pin the
 * three things the SDK must get right about that: it sends the token on writes
 * and only on writes, it refuses to leak the token over a plaintext transport,
 * and it maps a 401 to a distinct `PulsarAuthError` a caller will not retry as
 * if it were a transient network fault.
 */

import { inspect } from 'node:util';

import { http, HttpResponse } from 'msw';
import { describe, expect, it } from 'vitest';

import { PulsarClient } from '../src/client.js';
import { PulsarAuthError, PulsarError, PulsarNetworkError, PulsarValidationError } from '../src/errors.js';

import { trackedContractPayload } from './mocks/handlers.js';
import { server } from './mocks/server.js';

/** A token that clears the ADR-044 minimum length of 16 characters. */
const TOKEN = 'admin-token-value-long-enough';
const SHOWCASE_ID = trackedContractPayload.id;

/** An https indexer, so the secure-transport guard is satisfied. */
const SECURE_URL = 'https://indexer.test';
const secureContractsUrl = `${SECURE_URL}/contracts`;

describe('sending the token on write calls', () => {
  it('adds an Authorization bearer header when an admin token is configured', async () => {
    const client = new PulsarClient({ indexerUrl: SECURE_URL, adminToken: TOKEN, timeoutMs: 200 });
    let seenAuth: string | null = null;
    server.use(
      http.post(secureContractsUrl, ({ request }) => {
        seenAuth = request.headers.get('authorization');
        return HttpResponse.json({ data: trackedContractPayload });
      }),
    );

    await client.registerContract(SHOWCASE_ID);
    expect(seenAuth).toBe(`Bearer ${TOKEN}`);
  });

  it('sends no Authorization header when no admin token is configured', async () => {
    const client = new PulsarClient({ indexerUrl: SECURE_URL, timeoutMs: 200 });
    let seenAuth: string | null = 'unset';
    server.use(
      http.post(secureContractsUrl, ({ request }) => {
        seenAuth = request.headers.get('authorization');
        return HttpResponse.json({ data: trackedContractPayload });
      }),
    );

    await client.registerContract(SHOWCASE_ID);
    expect(seenAuth).toBeNull();
  });

  it('never sends the token on a read, so a public GET does not carry a secret', async () => {
    const client = new PulsarClient({ indexerUrl: SECURE_URL, adminToken: TOKEN, timeoutMs: 200 });
    let seenAuth: string | null = 'unset';
    server.use(
      http.get(`${SECURE_URL}/contracts`, ({ request }) => {
        seenAuth = request.headers.get('authorization');
        return HttpResponse.json({ data: { items: [trackedContractPayload] } });
      }),
    );

    await client.listContracts();
    expect(seenAuth).toBeNull();
  });
});

describe('refusing to leak the token over a plaintext transport', () => {
  it('rejects an admin token over http to a non-loopback host at construction', () => {
    expect(() => new PulsarClient({ indexerUrl: 'http://indexer.example.com', adminToken: TOKEN })).toThrow(
      PulsarValidationError,
    );
  });

  it('allows an admin token over http to localhost, the usual dev loopback', () => {
    expect(() => new PulsarClient({ indexerUrl: 'http://localhost:8080', adminToken: TOKEN })).not.toThrow();
  });

  it('allows an admin token over http to the loopback IP', () => {
    expect(() => new PulsarClient({ indexerUrl: 'http://127.0.0.1:8080', adminToken: TOKEN })).not.toThrow();
  });

  it('allows an admin token over https to any host', () => {
    expect(() => new PulsarClient({ indexerUrl: SECURE_URL, adminToken: TOKEN })).not.toThrow();
  });

  it('rejects an admin token shorter than the ADR-044 minimum of 16 characters', () => {
    expect(() => new PulsarClient({ indexerUrl: SECURE_URL, adminToken: 'too-short' })).toThrow(
      PulsarValidationError,
    );
  });
});

describe('mapping the unauthorized wire class', () => {
  const rejectUnauthorized = () =>
    server.use(
      http.post(secureContractsUrl, () =>
        HttpResponse.json(
          { error: { code: 'unauthorized', message: 'missing or invalid bearer token' } },
          { status: 401 },
        ),
      ),
    );

  it('throws a PulsarAuthError on a 401 carrying the unauthorized envelope', async () => {
    const client = new PulsarClient({ indexerUrl: SECURE_URL, adminToken: TOKEN, timeoutMs: 200 });
    rejectUnauthorized();
    await expect(client.registerContract(SHOWCASE_ID)).rejects.toThrow(PulsarAuthError);
  });

  it('carries the status and the indexer message on the auth error', async () => {
    const client = new PulsarClient({ indexerUrl: SECURE_URL, adminToken: TOKEN, timeoutMs: 200 });
    rejectUnauthorized();
    try {
      await client.registerContract(SHOWCASE_ID);
      expect.unreachable('registerContract should have thrown');
    } catch (error) {
      const authError = error as PulsarAuthError;
      expect(authError.status).toBe(401);
      expect(authError.details['indexerMessage']).toBe('missing or invalid bearer token');
      expect(authError.operation).toBe('client.registerContract');
    }
  });

  it('is a PulsarError but not a PulsarNetworkError, so retry-on-network logic skips it', async () => {
    const client = new PulsarClient({ indexerUrl: SECURE_URL, adminToken: TOKEN, timeoutMs: 200 });
    rejectUnauthorized();
    try {
      await client.registerContract(SHOWCASE_ID);
      expect.unreachable('registerContract should have thrown');
    } catch (error) {
      expect(error).toBeInstanceOf(PulsarError);
      expect(error).not.toBeInstanceOf(PulsarNetworkError);
    }
  });

  it('maps a bare 401 whose JSON body is not the error envelope to a PulsarAuthError as well', async () => {
    const client = new PulsarClient({ indexerUrl: SECURE_URL, adminToken: TOKEN, timeoutMs: 200 });
    server.use(
      http.post(secureContractsUrl, () => HttpResponse.json({ message: 'denied by proxy' }, { status: 401 })),
    );
    await expect(client.registerContract(SHOWCASE_ID)).rejects.toThrow(PulsarAuthError);
  });
});

describe('keeping the configured token out of an accidental log', () => {
  it('omits the admin token from JSON.stringify of the config', () => {
    const client = new PulsarClient({ indexerUrl: SECURE_URL, adminToken: TOKEN });
    expect(JSON.stringify(client.config)).not.toContain(TOKEN);
  });

  it('omits the admin token when the config is inspected for a log line', () => {
    const client = new PulsarClient({ indexerUrl: SECURE_URL, adminToken: TOKEN });
    expect(inspect(client.config)).not.toContain(TOKEN);
  });

  it('still exposes the token by direct access, so the SDK can authenticate a write', () => {
    const client = new PulsarClient({ indexerUrl: SECURE_URL, adminToken: TOKEN });
    expect(client.config.adminToken).toBe(TOKEN);
  });
});
