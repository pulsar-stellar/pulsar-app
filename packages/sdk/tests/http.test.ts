import { http, HttpResponse } from 'msw';
import { describe, expect, it } from 'vitest';
import { z } from 'zod';

import { request } from '../src/http.js';
import { PulsarConfigSchema } from '../src/types.js';

import { INDEXER_URL, server } from './mocks/server.js';

const config = PulsarConfigSchema.parse({ indexerUrl: INDEXER_URL, timeoutMs: 200 });
const schema = z.object({ ok: z.boolean() });

describe('URL building', () => {
  it('accepts a path with no leading slash', async () => {
    server.use(http.get(`${INDEXER_URL}/health`, () => HttpResponse.json({ data: { ok: true } })));
    const result = await request(config, { path: 'health', schema, operation: 'test' });
    expect(result.data.ok).toBe(true);
  });

  it('collapses a trailing slash on the base against a leading slash on the path', async () => {
    const trailing = PulsarConfigSchema.parse({ indexerUrl: `${INDEXER_URL}///` });
    server.use(http.get(`${INDEXER_URL}/health`, () => HttpResponse.json({ data: { ok: true } })));
    const result = await request(trailing, { path: '/health', schema, operation: 'test' });
    expect(result.data.ok).toBe(true);
  });
});

describe('query strings', () => {
  it('omits the question mark when every parameter is undefined', async () => {
    let seenUrl = '';
    server.use(
      http.get(`${INDEXER_URL}/health`, ({ request }) => {
        seenUrl = request.url;
        return HttpResponse.json({ data: { ok: true } });
      }),
    );

    await request(config, {
      path: '/health',
      schema,
      operation: 'test',
      query: { name: undefined, limit: undefined },
    });

    expect(seenUrl).toBe(`${INDEXER_URL}/health`);
  });
});

describe('envelope handling', () => {
  it('reports null for a cursor on a response that does not paginate', async () => {
    server.use(http.get(`${INDEXER_URL}/health`, () => HttpResponse.json({ data: { ok: true } })));
    const result = await request(config, { path: '/health', schema, operation: 'test' });
    expect(result.nextCursor).toBeNull();
  });

  it('surfaces a cursor when the response carries one', async () => {
    server.use(
      http.get(`${INDEXER_URL}/health`, () =>
        HttpResponse.json({ data: { ok: true }, next_cursor: 'page-2' }),
      ),
    );
    const result = await request(config, { path: '/health', schema, operation: 'test' });
    expect(result.nextCursor).toBe('page-2');
  });
});
