import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { z } from 'zod';

import { firstRequest } from './support';

import { executeGraphQL } from '@/lib/graphql/client';
import { GraphQLRequestError } from '@/lib/graphql/errors';


const KEY = 'NEXT_PUBLIC_PULSAR_INDEXER_URL';
const dataSchema = z.object({ event: z.object({ id: z.string() }).nullable() });

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json' },
  });
}

describe('executeGraphQL', () => {
  const original = process.env[KEY];

  beforeEach(() => {
    process.env[KEY] = 'http://indexer.test:8080';
  });

  afterEach(() => {
    if (original === undefined) delete process.env[KEY];
    else process.env[KEY] = original;
    vi.restoreAllMocks();
  });

  it('POSTs to the indexer /graphql and returns validated data', async () => {
    const fetchImpl = vi.fn().mockResolvedValue(
      jsonResponse({ data: { event: { id: '42' } } }),
    );

    const data = await executeGraphQL(
      dataSchema,
      { query: 'query($id:ID!){event(id:$id){id}}', variables: { id: '42' } },
      { fetchImpl },
    );

    expect(data.event?.id).toBe('42');
    const { url, init, body } = firstRequest(fetchImpl);
    expect(url).toBe('http://indexer.test:8080/graphql');
    expect(init.method).toBe('POST');
    expect(body).toEqual({
      query: 'query($id:ID!){event(id:$id){id}}',
      variables: { id: '42' },
      operationName: undefined,
    });
  });

  it('maps a GraphQL error to a categorized GraphQLRequestError', async () => {
    const fetchImpl = vi.fn().mockResolvedValue(
      jsonResponse({
        data: null,
        errors: [
          {
            message: 'contract is not tracked',
            extensions: { code: 'NOT_FOUND_CONTRACT', class: 'not_found' },
          },
        ],
      }),
    );

    await expect(
      executeGraphQL(dataSchema, { query: 'q' }, { fetchImpl }),
    ).rejects.toMatchObject({
      name: 'GraphQLRequestError',
      category: 'not_found',
      code: 'NOT_FOUND_CONTRACT',
      message: 'contract is not tracked',
    });
  });

  it('reports a network category when fetch throws', async () => {
    const fetchImpl = vi.fn().mockRejectedValue(new Error('ECONNREFUSED'));
    const err = await executeGraphQL(dataSchema, { query: 'q' }, { fetchImpl }).catch(
      (e: unknown) => e,
    );
    expect(err).toBeInstanceOf(GraphQLRequestError);
    expect((err as GraphQLRequestError).category).toBe('network');
  });

  it('reports an error when the body is not JSON', async () => {
    const fetchImpl = vi.fn().mockResolvedValue(
      new Response('<html>502</html>', {
        status: 502,
        headers: { 'content-type': 'text/html' },
      }),
    );
    const err = await executeGraphQL(dataSchema, { query: 'q' }, { fetchImpl }).catch(
      (e: unknown) => e,
    );
    expect(err).toBeInstanceOf(GraphQLRequestError);
    expect((err as GraphQLRequestError).category).toBe('network');
  });

  it('errors when errors are absent but data does not match the schema', async () => {
    const fetchImpl = vi.fn().mockResolvedValue(
      jsonResponse({ data: { event: { id: 42 } } }), // id should be a string
    );
    const err = await executeGraphQL(dataSchema, { query: 'q' }, { fetchImpl }).catch(
      (e: unknown) => e,
    );
    expect(err).toBeInstanceOf(GraphQLRequestError);
    expect((err as GraphQLRequestError).category).toBe('internal');
  });

  it('errors when both data and errors are absent', async () => {
    const fetchImpl = vi.fn().mockResolvedValue(jsonResponse({}));
    const err = await executeGraphQL(dataSchema, { query: 'q' }, { fetchImpl }).catch(
      (e: unknown) => e,
    );
    expect(err).toBeInstanceOf(GraphQLRequestError);
    expect((err as GraphQLRequestError).category).toBe('internal');
  });

  it('reports an internal error when the indexer URL is not configured', async () => {
    delete process.env[KEY];
    const fetchImpl = vi.fn();
    const err = await executeGraphQL(dataSchema, { query: 'q' }, { fetchImpl }).catch(
      (e: unknown) => e,
    );
    expect(err).toBeInstanceOf(GraphQLRequestError);
    expect((err as GraphQLRequestError).category).toBe('internal');
    expect(fetchImpl).not.toHaveBeenCalled();
  });

  it('checks errors before validating data shape', async () => {
    // data is shape-invalid AND errors present: the reported error must be the
    // indexer's, not a shape-mismatch masking it.
    const fetchImpl = vi.fn().mockResolvedValue(
      jsonResponse({
        data: { event: { id: 42 } },
        errors: [{ message: 'boom', extensions: { code: 'INTERNAL_STORE' } }],
      }),
    );
    await expect(
      executeGraphQL(dataSchema, { query: 'q' }, { fetchImpl }),
    ).rejects.toMatchObject({ code: 'INTERNAL_STORE', category: 'internal' });
  });
});
