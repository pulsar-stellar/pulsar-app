import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { POST } from '@/app/api/graphql/route';
import { dispatch } from '@/lib/graphql/dispatch';
import { GraphQLRequestError } from '@/lib/graphql/errors';

// vitest hoists vi.mock above the imports, so the route sees the mocked dispatch.
vi.mock('@/lib/graphql/dispatch', () => ({ dispatch: vi.fn() }));

const dispatchMock = vi.mocked(dispatch);
const URL = 'http://localhost/api/graphql';
const MAX_BODY_BYTES = 16 * 1024;

/** Build a POST request with a unique client key so tests share no rate budget. */
function post(body: BodyInit, ip: string, headers: Record<string, string> = {}): Request {
  return new Request(URL, {
    method: 'POST',
    body,
    headers: { 'x-forwarded-for': ip, ...headers },
  });
}

describe('POST /api/graphql', () => {
  beforeEach(() => {
    dispatchMock.mockReset();
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('returns the dispatched data on success', async () => {
    dispatchMock.mockResolvedValue({ event: { id: '42' } });
    const res = await POST(post(JSON.stringify({ operation: 'event', variables: { id: '42' } }), 'ip-success'));
    expect(res.status).toBe(200);
    await expect(res.json()).resolves.toEqual({ data: { event: { id: '42' } } });
  });

  it('rejects a malformed JSON body with 400 before dispatch', async () => {
    const res = await POST(post('not json', 'ip-badjson'));
    expect(res.status).toBe(400);
    expect(dispatchMock).not.toHaveBeenCalled();
  });

  it('rejects an oversized body declared by content-length with 413', async () => {
    const big = JSON.stringify({ operation: 'x'.repeat(MAX_BODY_BYTES) });
    const res = await POST(post(big, 'ip-biglen'));
    expect(res.status).toBe(413);
    expect(dispatchMock).not.toHaveBeenCalled();
  });

  it('rejects an oversized streamed body with no content-length with 413', async () => {
    const chunk = new TextEncoder().encode('x'.repeat(MAX_BODY_BYTES + 100));
    const stream = new ReadableStream<Uint8Array>({
      start(controller) {
        controller.enqueue(chunk);
        controller.close();
      },
    });
    const req = new Request(URL, {
      method: 'POST',
      body: stream,
      headers: { 'x-forwarded-for': 'ip-bigstream' },
      // Required by the Fetch spec when the body is a stream.
      duplex: 'half',
    } as RequestInit & { duplex: 'half' });
    const res = await POST(req);
    expect(res.status).toBe(413);
    expect(dispatchMock).not.toHaveBeenCalled();
  });

  it('surfaces a validation error message and code verbatim', async () => {
    dispatchMock.mockRejectedValue(
      new GraphQLRequestError('Invalid contract ID', {
        category: 'validation',
        code: 'VALIDATION_CONTRACT_ID',
      }),
    );
    const res = await POST(post(JSON.stringify({ operation: 'events', variables: {} }), 'ip-val'));
    expect(res.status).toBe(400);
    const body = (await res.json()) as { error: { category: string; code: string; message: string } };
    expect(body.error).toEqual({
      category: 'validation',
      code: 'VALIDATION_CONTRACT_ID',
      message: 'Invalid contract ID',
    });
  });

  it('generalizes an internal error message but keeps the code', async () => {
    dispatchMock.mockRejectedValue(
      new GraphQLRequestError('secret store detail 10.0.0.1 leaked', {
        category: 'internal',
        code: 'INTERNAL_STORE',
      }),
    );
    const res = await POST(post(JSON.stringify({ operation: 'events', variables: {} }), 'ip-int'));
    expect(res.status).toBe(500);
    const body = (await res.json()) as { error: { category: string; code: string; message: string } };
    expect(body.error.code).toBe('INTERNAL_STORE');
    expect(body.error.message).not.toContain('secret');
    expect(body.error.message).not.toContain('10.0.0.1');
  });

  it('rate-limits a client that exceeds the window and sets Retry-After', async () => {
    dispatchMock.mockResolvedValue({ ok: true });
    const ip = 'ip-flood';
    let last: Response | undefined;
    for (let i = 0; i < 61; i += 1) {
      last = await POST(post(JSON.stringify({ operation: 'health' }), ip));
    }
    expect(last?.status).toBe(429);
    expect(last?.headers.get('Retry-After')).not.toBeNull();
  });
});
