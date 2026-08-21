import { http, HttpResponse } from 'msw';
import { describe, expect, it } from 'vitest';

import { PulsarClient } from '../src/client.js';
import { PulsarNetworkError, PulsarValidationError } from '../src/errors.js';

import { healthyHealthPayload } from './mocks/handlers.js';
import { INDEXER_URL, server } from './mocks/server.js';

const client = new PulsarClient({ indexerUrl: INDEXER_URL, timeoutMs: 200 });
const healthUrl = `${INDEXER_URL}/health`;

describe('ping on a healthy indexer', () => {
  it('returns what the indexer reports about itself', async () => {
    const result = await client.ping();
    expect(result.ok).toBe(true);
    expect(result.version).toBe('0.1.0');
    expect(result.latestLedger).toBe(healthyHealthPayload.latest_ledger);
    expect(result.trackedContracts).toBe(healthyHealthPayload.tracked_contracts);
  });

  it('measures its own round-trip time separately from the indexer timing', async () => {
    const result = await client.ping();
    expect(result.latencyMs).toBeGreaterThanOrEqual(0);
    expect(result.serverTookMs).toBe(4);
  });

  it('reports null rather than undefined for fields the indexer omitted', async () => {
    server.use(
      http.get(healthUrl, () => HttpResponse.json({ data: { ok: true, version: '0.1.0' } })),
    );
    const result = await client.ping();
    expect(result.latestLedger).toBeNull();
    expect(result.trackedContracts).toBeNull();
    expect(result.serverTookMs).toBeNull();
  });

  it('treats a reachable but unhealthy indexer as a successful call', async () => {
    server.use(
      http.get(healthUrl, () => HttpResponse.json({ data: { ok: false, version: '0.1.0' } })),
    );
    const result = await client.ping();
    expect(result.ok).toBe(false);
  });

  it('tolerates a trailing slash on the configured indexer URL', async () => {
    const trailing = new PulsarClient({ indexerUrl: `${INDEXER_URL}/`, timeoutMs: 200 });
    await expect(trailing.ping()).resolves.toMatchObject({ ok: true });
  });
});

describe('ping against a non-success status', () => {
  /**
   * A JSON body is deliberate. An empty body makes `response.json()` throw
   * first, so the assertion would pass through the malformed-JSON branch and
   * never reach the status check it claims to cover.
   */
  const unavailable = () =>
    HttpResponse.json({ data: healthyHealthPayload }, { status: 503 });

  it('throws a PulsarNetworkError carrying the status', async () => {
    server.use(http.get(healthUrl, unavailable));
    await expect(client.ping()).rejects.toThrow(PulsarNetworkError);
  });

  it('reports the status in the message rather than only in a field', async () => {
    server.use(http.get(healthUrl, unavailable));
    try {
      await client.ping();
      expect.unreachable('ping should have thrown');
    } catch (error) {
      expect((error as PulsarNetworkError).message).toContain('503');
    }
  });

  it('throws for a non-success status with an empty body too', async () => {
    server.use(http.get(healthUrl, () => new HttpResponse(null, { status: 503 })));
    await expect(client.ping()).rejects.toThrow(PulsarNetworkError);
  });

  it('exposes the status and URL on the error', async () => {
    server.use(http.get(healthUrl, unavailable));
    try {
      await client.ping();
      expect.unreachable('ping should have thrown');
    } catch (error) {
      const networkError = error as PulsarNetworkError;
      expect(networkError.status).toBe(503);
      expect(networkError.url).toBe(healthUrl);
      expect(networkError.operation).toBe('client.ping');
    }
  });

  it('surfaces the error envelope when the indexer returns one', async () => {
    server.use(
      http.get(healthUrl, () =>
        HttpResponse.json(
          { error: { code: 'internal', message: 'database unavailable' } },
          { status: 500 },
        ),
      ),
    );
    try {
      await client.ping();
      expect.unreachable('ping should have thrown');
    } catch (error) {
      const networkError = error as PulsarNetworkError;
      expect(networkError.message).toContain('internal');
      expect(networkError.details['indexerMessage']).toBe('database unavailable');
    }
  });
});

describe('ping against a timeout', () => {
  it('throws a PulsarNetworkError naming the configured timeout', async () => {
    server.use(
      http.get(healthUrl, async () => {
        await new Promise((resolve) => setTimeout(resolve, 2_000));
        return HttpResponse.json({ data: healthyHealthPayload });
      }),
    );
    try {
      await client.ping();
      expect.unreachable('ping should have thrown');
    } catch (error) {
      expect(error).toBeInstanceOf(PulsarNetworkError);
      expect((error as PulsarNetworkError).message).toContain('200ms');
      expect((error as PulsarNetworkError).status).toBeNull();
    }
  });
});

describe('ping against an unreachable indexer', () => {
  it('throws a PulsarNetworkError with no status', async () => {
    server.use(http.get(healthUrl, () => HttpResponse.error()));
    try {
      await client.ping();
      expect.unreachable('ping should have thrown');
    } catch (error) {
      expect(error).toBeInstanceOf(PulsarNetworkError);
      expect((error as PulsarNetworkError).status).toBeNull();
    }
  });

  it('keeps the underlying fetch failure as cause', async () => {
    server.use(http.get(healthUrl, () => HttpResponse.error()));
    try {
      await client.ping();
      expect.unreachable('ping should have thrown');
    } catch (error) {
      expect((error as PulsarNetworkError).cause).toBeDefined();
      expect((error as PulsarNetworkError).cause).toBeInstanceOf(Error);
    }
  });
});

describe('ping against a malformed response', () => {
  it('throws a PulsarNetworkError when the body is not JSON', async () => {
    server.use(http.get(healthUrl, () => new HttpResponse('not json at all', { status: 200 })));
    await expect(client.ping()).rejects.toThrow(PulsarNetworkError);
  });

  it('throws a PulsarValidationError when the payload has the wrong shape', async () => {
    server.use(http.get(healthUrl, () => HttpResponse.json({ data: { ok: 'yes' } })));
    await expect(client.ping()).rejects.toThrow(PulsarValidationError);
  });

  it('distinguishes an envelope failure from a payload failure', async () => {
    server.use(http.get(healthUrl, () => HttpResponse.json({ data: { ok: true } })));
    try {
      await client.ping();
      expect.unreachable('ping should have thrown');
    } catch (error) {
      const validationError = error as PulsarValidationError;
      expect(validationError.details['stage']).toBe('payload');
      expect(validationError.issues[0]?.path.join('.')).toBe('version');
    }
  });

  it('rejects a bare payload that is missing the envelope', async () => {
    server.use(http.get(healthUrl, () => HttpResponse.json(healthyHealthPayload)));
    await expect(client.ping()).rejects.toThrow(PulsarValidationError);
  });
});
