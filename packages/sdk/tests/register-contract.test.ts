import { http, HttpResponse } from 'msw';
import { describe, expect, it } from 'vitest';

import { PulsarClient } from '../src/client.js';
import { PulsarNetworkError, PulsarValidationError } from '../src/errors.js';

import { trackedContractPayload } from './mocks/handlers.js';
import { INDEXER_URL, server } from './mocks/server.js';

const client = new PulsarClient({ indexerUrl: INDEXER_URL, timeoutMs: 200 });
const contractsUrl = `${INDEXER_URL}/contracts`;
const SHOWCASE_ID = trackedContractPayload.id;

describe('registering a contract successfully', () => {
  it('returns the tracked contract in camelCase', async () => {
    const contract = await client.registerContract(SHOWCASE_ID);
    expect(contract).toEqual({
      contractId: SHOWCASE_ID,
      addedAt: trackedContractPayload.added_at,
      firstIndexedLedger: trackedContractPayload.first_indexed_ledger,
      lastIndexedLedger: trackedContractPayload.last_indexed_ledger,
      status: 'active',
    });
  });

  it('posts the contract ID in the snake_case body the indexer expects', async () => {
    let seenBody: unknown;
    let seenMethod: string | undefined;
    server.use(
      http.post(contractsUrl, async ({ request }) => {
        seenMethod = request.method;
        seenBody = await request.json();
        return HttpResponse.json({ data: trackedContractPayload });
      }),
    );

    await client.registerContract(SHOWCASE_ID);
    expect(seenMethod).toBe('POST');
    expect(seenBody).toEqual({ contract_id: SHOWCASE_ID });
  });

  it('returns the existing record when the contract is already tracked', async () => {
    server.use(
      http.post(contractsUrl, () =>
        HttpResponse.json({ data: { ...trackedContractPayload, last_indexed_ledger: 9_000_000 } }),
      ),
    );
    const contract = await client.registerContract(SHOWCASE_ID);
    expect(contract.lastIndexedLedger).toBe(9_000_000);
  });

  it('reports null for a contract not yet indexed', async () => {
    server.use(
      http.post(contractsUrl, () =>
        HttpResponse.json({
          data: { ...trackedContractPayload, first_indexed_ledger: null, last_indexed_ledger: 0 },
        }),
      ),
    );
    const contract = await client.registerContract(SHOWCASE_ID);
    expect(contract.firstIndexedLedger).toBeNull();
  });
});

describe('client-side validation rejects before any request', () => {
  /** Fails the test if a request escapes, which is the point of validating first. */
  const failOnRequest = () =>
    server.use(
      http.post(contractsUrl, () => {
        throw new Error('request should not have been sent');
      }),
    );

  it('rejects an empty string', async () => {
    failOnRequest();
    await expect(client.registerContract('')).rejects.toThrow(PulsarValidationError);
  });

  it('rejects a value that is not a string', async () => {
    failOnRequest();
    await expect(client.registerContract(undefined as never)).rejects.toThrow(
      PulsarValidationError,
    );
  });

  it('rejects an ID one character short', async () => {
    failOnRequest();
    await expect(client.registerContract(SHOWCASE_ID.slice(0, 55))).rejects.toThrow(
      PulsarValidationError,
    );
  });

  it('names the operation and the offending ID on the error', async () => {
    failOnRequest();
    try {
      await client.registerContract('nope');
      expect.unreachable('registerContract should have thrown');
    } catch (error) {
      const validationError = error as PulsarValidationError;
      expect(validationError.operation).toBe('client.registerContract');
      expect(validationError.details['contractId']).toBe('nope');
    }
  });
});

describe('plausible-wrong contract IDs', () => {
  /**
   * These are what an inattentive caller actually produces: an account ID
   * instead of a contract ID, an ID lowercased by a copy step, and one pasted
   * with surrounding whitespace. Each is close enough to correct that a loose
   * check would wave it through.
   */
  it.each([
    ['an account ID, which starts with G', `G${SHOWCASE_ID.slice(1)}`],
    ['a lowercased contract ID', SHOWCASE_ID.toLowerCase()],
    ['an ID pasted with surrounding whitespace', ` ${SHOWCASE_ID}\n`],
  ])('rejects %s', async (_label, candidate) => {
    server.use(
      http.post(contractsUrl, () => {
        throw new Error('request should not have been sent');
      }),
    );
    await expect(client.registerContract(candidate)).rejects.toThrow(PulsarValidationError);
  });
});

describe('server-side failures', () => {
  it('throws a network error when the indexer rejects the request', async () => {
    server.use(
      http.post(contractsUrl, () =>
        HttpResponse.json(
          { error: { code: 'validation', message: 'contract not found on network' } },
          { status: 400 },
        ),
      ),
    );
    try {
      await client.registerContract(SHOWCASE_ID);
      expect.unreachable('registerContract should have thrown');
    } catch (error) {
      expect(error).toBeInstanceOf(PulsarNetworkError);
      expect((error as PulsarNetworkError).details['indexerMessage']).toBe(
        'contract not found on network',
      );
    }
  });

  it('throws a network error on a non-success status with a JSON body', async () => {
    server.use(
      http.post(contractsUrl, () =>
        HttpResponse.json({ data: trackedContractPayload }, { status: 500 }),
      ),
    );
    await expect(client.registerContract(SHOWCASE_ID)).rejects.toThrow(PulsarNetworkError);
  });

  it('throws a network error when the indexer is unreachable', async () => {
    server.use(http.post(contractsUrl, () => HttpResponse.error()));
    await expect(client.registerContract(SHOWCASE_ID)).rejects.toThrow(PulsarNetworkError);
  });

  it('throws a validation error when the response payload is the wrong shape', async () => {
    server.use(
      http.post(contractsUrl, () =>
        HttpResponse.json({ data: { ...trackedContractPayload, status: 'stopped' } }),
      ),
    );
    try {
      await client.registerContract(SHOWCASE_ID);
      expect.unreachable('registerContract should have thrown');
    } catch (error) {
      const validationError = error as PulsarValidationError;
      expect(validationError.details['stage']).toBe('payload');
      expect(validationError.issues[0]?.path.join('.')).toBe('status');
    }
  });
});
