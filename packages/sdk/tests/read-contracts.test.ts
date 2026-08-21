import { http, HttpResponse } from 'msw';
import { describe, expect, it } from 'vitest';

import { PulsarClient } from '../src/client.js';
import { PulsarNetworkError, PulsarValidationError } from '../src/errors.js';

import { trackedContractPayload } from './mocks/handlers.js';
import { INDEXER_URL, server } from './mocks/server.js';

const client = new PulsarClient({ indexerUrl: INDEXER_URL, timeoutMs: 200 });
const SHOWCASE_ID = trackedContractPayload.id;
const contractUrl = `${INDEXER_URL}/contracts/${SHOWCASE_ID}`;
const contractsUrl = `${INDEXER_URL}/contracts`;

/** Absence as ADR-019 defines it: a 404 and a not_found envelope together. */
const absent = () =>
  HttpResponse.json(
    { error: { code: 'not_found', message: 'contract not tracked' } },
    { status: 404 },
  );

describe('getContract on a tracked contract', () => {
  it('returns the record in camelCase', async () => {
    const contract = await client.getContract(SHOWCASE_ID);
    expect(contract).toEqual({
      contractId: SHOWCASE_ID,
      addedAt: trackedContractPayload.added_at,
      firstIndexedLedger: trackedContractPayload.first_indexed_ledger,
      lastIndexedLedger: trackedContractPayload.last_indexed_ledger,
      status: 'active',
    });
  });

  it('requests the single-contract route with a GET', async () => {
    let seen: { method: string; url: string } | undefined;
    server.use(
      http.get(`${INDEXER_URL}/contracts/:id`, ({ request }) => {
        seen = { method: request.method, url: request.url };
        return HttpResponse.json({ data: trackedContractPayload });
      }),
    );
    await client.getContract(SHOWCASE_ID);
    expect(seen?.method).toBe('GET');
    expect(seen?.url).toBe(contractUrl);
  });
});

describe('getContract on an absent contract', () => {
  it('returns null for a 404 carrying a not_found envelope', async () => {
    server.use(http.get(`${INDEXER_URL}/contracts/:id`, absent));
    const contract = await client.getContract(SHOWCASE_ID);
    expect(contract).toBeNull();
  });

  it('returns exactly null and never undefined', async () => {
    server.use(http.get(`${INDEXER_URL}/contracts/:id`, absent));
    const contract = await client.getContract(SHOWCASE_ID);
    expect(contract).not.toBeUndefined();
    expect(contract === null).toBe(true);
  });

  it('throws on a bare 404, which is a routing problem rather than an absence', async () => {
    server.use(
      http.get(`${INDEXER_URL}/contracts/:id`, () => HttpResponse.json({}, { status: 404 })),
    );
    try {
      await client.getContract(SHOWCASE_ID);
      expect.unreachable('getContract should have thrown');
    } catch (error) {
      expect(error).toBeInstanceOf(PulsarNetworkError);
      expect((error as PulsarNetworkError).status).toBe(404);
    }
  });

  it('throws on a 404 whose envelope carries a different code', async () => {
    server.use(
      http.get(`${INDEXER_URL}/contracts/:id`, () =>
        HttpResponse.json({ error: { code: 'internal', message: 'oops' } }, { status: 404 }),
      ),
    );
    await expect(client.getContract(SHOWCASE_ID)).rejects.toThrow(PulsarNetworkError);
  });

  it('throws a validation error when a 200 carries a not_found envelope', async () => {
    server.use(
      http.get(`${INDEXER_URL}/contracts/:id`, () =>
        HttpResponse.json({ error: { code: 'not_found', message: 'contradictory' } }),
      ),
    );
    try {
      await client.getContract(SHOWCASE_ID);
      expect.unreachable('getContract should have thrown');
    } catch (error) {
      expect(error).toBeInstanceOf(PulsarValidationError);
      expect((error as PulsarValidationError).details['stage']).toBe('envelope');
    }
  });
});

describe('getContract input validation', () => {
  const failOnRequest = () =>
    server.use(
      http.get(`${INDEXER_URL}/contracts/:id`, () => {
        throw new Error('request should not have been sent');
      }),
    );

  it.each([
    ['an empty string', ''],
    ['a value that is not a string', undefined as never],
    ['an ID one character short', SHOWCASE_ID.slice(0, 55)],
  ])('rejects %s before sending anything', async (_label, candidate) => {
    failOnRequest();
    await expect(client.getContract(candidate)).rejects.toThrow(PulsarValidationError);
  });

  it.each([
    ['an account ID, which starts with G', `G${SHOWCASE_ID.slice(1)}`],
    ['a lowercased contract ID', SHOWCASE_ID.toLowerCase()],
    ['an ID pasted with surrounding whitespace', ` ${SHOWCASE_ID}\n`],
  ])('rejects %s', async (_label, candidate) => {
    failOnRequest();
    await expect(client.getContract(candidate)).rejects.toThrow(PulsarValidationError);
  });
});

describe('getContract server failures', () => {
  it('throws on a server error', async () => {
    server.use(
      http.get(`${INDEXER_URL}/contracts/:id`, () =>
        HttpResponse.json({ error: { code: 'internal', message: 'db down' } }, { status: 500 }),
      ),
    );
    await expect(client.getContract(SHOWCASE_ID)).rejects.toThrow(PulsarNetworkError);
  });

  it('throws when the record has the wrong shape', async () => {
    server.use(
      http.get(`${INDEXER_URL}/contracts/:id`, () =>
        HttpResponse.json({ data: { ...trackedContractPayload, last_indexed_ledger: 'many' } }),
      ),
    );
    try {
      await client.getContract(SHOWCASE_ID);
      expect.unreachable('getContract should have thrown');
    } catch (error) {
      const validationError = error as PulsarValidationError;
      expect(validationError.details['stage']).toBe('payload');
      expect(validationError.issues[0]?.path.join('.')).toBe('last_indexed_ledger');
    }
  });
});

describe('listContracts', () => {
  it('returns the tracked contracts as an array', async () => {
    const contracts = await client.listContracts();
    expect(contracts).toHaveLength(1);
    expect(contracts[0]?.contractId).toBe(SHOWCASE_ID);
  });

  it('returns an empty array when nothing is tracked, never null', async () => {
    server.use(http.get(contractsUrl, () => HttpResponse.json({ data: { items: [] } })));
    const contracts = await client.listContracts();
    expect(contracts).toEqual([]);
    expect(contracts).not.toBeNull();
  });

  it('throws on a server error', async () => {
    server.use(
      http.get(contractsUrl, () =>
        HttpResponse.json({ error: { code: 'internal', message: 'db down' } }, { status: 500 }),
      ),
    );
    await expect(client.listContracts()).rejects.toThrow(PulsarNetworkError);
  });

  it('throws when the payload is a bare array without the items wrapper', async () => {
    server.use(http.get(contractsUrl, () => HttpResponse.json({ data: [trackedContractPayload] })));
    try {
      await client.listContracts();
      expect.unreachable('listContracts should have thrown');
    } catch (error) {
      const validationError = error as PulsarValidationError;
      expect(validationError.details['stage']).toBe('payload');
      // An array fails the object schema at the root, so the issue has no path.
      expect(validationError.issues[0]?.path).toHaveLength(0);
      expect(validationError.message).toContain('<root>');
    }
  });

  it('throws when an entry inside items has the wrong shape', async () => {
    server.use(
      http.get(contractsUrl, () =>
        HttpResponse.json({ data: { items: [{ ...trackedContractPayload, status: 'stopped' }] } }),
      ),
    );
    try {
      await client.listContracts();
      expect.unreachable('listContracts should have thrown');
    } catch (error) {
      expect((error as PulsarValidationError).issues[0]?.path.join('.')).toBe('items.0.status');
    }
  });
});
