import { NextResponse } from 'next/server';

import { dispatch } from '@/lib/graphql/dispatch';
import {
  GraphQLRequestError,
  type GraphQLErrorCategory,
} from '@/lib/graphql/errors';
import { createRateLimiter } from '@/lib/http/rate-limit';

/**
 * The browser-facing refetch endpoint for the explorer's Hybrid data layer.
 *
 * This runs in Node (never the browser) and is a thin transport shell over
 * {@link dispatch}: it rate-limits the caller, bounds the body, parses JSON, and
 * shapes the reply. All routing, the operation allowlist, and validation live in
 * dispatch, so this file stays trivial (it is excluded from coverage; dispatch,
 * the limiter, and the error mapping are unit-tested).
 *
 * The endpoint is intentionally public and read-only, mirroring the indexer's
 * public GraphQL surface (ADR-043); it is not an arbitrary-query proxy, so the
 * only inputs it forwards are an allowlisted operation and its variables.
 */

export const runtime = 'nodejs';
export const dynamic = 'force-dynamic';

/** A dispatch request is a small JSON object; anything larger is rejected. */
const MAX_BODY_BYTES = 16 * 1024;

/** Per-client cap: each accepted request drives one indexer round-trip (ADR-046). */
const RATE_LIMIT = 60;
const RATE_WINDOW_MS = 60_000;

const limiter = createRateLimiter({ limit: RATE_LIMIT, windowMs: RATE_WINDOW_MS });

const STATUS_BY_CATEGORY: Record<GraphQLErrorCategory, number> = {
  validation: 400,
  not_found: 404,
  network: 502,
  internal: 500,
  unknown: 500,
};

/**
 * Categories whose wire message may carry detail we did not author. `validation`
 * and `not_found` messages are safe to surface; for anything else we return a
 * fixed generic string and keep only the machine-readable code, so a stray
 * upstream error (a gateway, a proxy) cannot leak internal detail to the browser.
 */
const SAFE_MESSAGE_CATEGORIES: ReadonlySet<GraphQLErrorCategory> = new Set([
  'validation',
  'not_found',
]);

const GENERIC_MESSAGE = 'The request could not be completed';

function jsonError(
  category: GraphQLErrorCategory,
  code: string | null,
  message: string,
  status: number,
): NextResponse {
  return NextResponse.json({ error: { category, code, message } }, { status });
}

function errorResponse(error: GraphQLRequestError): NextResponse {
  const message = SAFE_MESSAGE_CATEGORIES.has(error.category)
    ? error.message
    : GENERIC_MESSAGE;
  return jsonError(
    error.category,
    error.code,
    message,
    STATUS_BY_CATEGORY[error.category],
  );
}

/** The client key for rate limiting: the first forwarded hop, or a shared bucket. */
function clientKey(request: Request): string {
  const forwarded = request.headers.get('x-forwarded-for');
  const first = forwarded?.split(',')[0]?.trim();
  return first || 'unknown';
}

/**
 * Read the request body with a running byte budget, aborting once it is
 * exceeded so an oversized or slow-streamed body never fully buffers in memory.
 * Returns `null` when the body is too large (declared or observed).
 */
async function readBoundedBody(
  request: Request,
  maxBytes: number,
): Promise<string | null> {
  const declared = request.headers.get('content-length');
  if (declared !== null) {
    const length = Number(declared);
    if (Number.isFinite(length) && length > maxBytes) return null;
  }

  const body = request.body;
  if (!body) return '';

  const reader = body.getReader();
  const chunks: Uint8Array[] = [];
  let total = 0;
  try {
    for (;;) {
      const { done, value } = await reader.read();
      if (done) break;
      if (!value) continue;
      total += value.byteLength;
      if (total > maxBytes) {
        await reader.cancel();
        return null;
      }
      chunks.push(value);
    }
  } finally {
    reader.releaseLock();
  }

  const merged = new Uint8Array(total);
  let offset = 0;
  for (const chunk of chunks) {
    merged.set(chunk, offset);
    offset += chunk.byteLength;
  }
  return new TextDecoder().decode(merged);
}

export async function POST(request: Request): Promise<NextResponse> {
  const { allowed, resetAt } = limiter.check(clientKey(request));
  if (!allowed) {
    const retryAfter = Math.max(1, Math.ceil((resetAt - Date.now()) / 1000));
    const response = jsonError(
      'unknown',
      'RATE_LIMITED',
      'Too many requests; slow down and try again shortly',
      429,
    );
    response.headers.set('Retry-After', String(retryAfter));
    return response;
  }

  const raw = await readBoundedBody(request, MAX_BODY_BYTES);
  if (raw === null) {
    return jsonError('validation', null, 'Request body too large', 413);
  }

  let body: unknown;
  try {
    body = JSON.parse(raw);
  } catch {
    return jsonError('validation', null, 'Request body must be JSON', 400);
  }

  try {
    const data = await dispatch(body, { signal: request.signal });
    return NextResponse.json({ data });
  } catch (error) {
    if (error instanceof GraphQLRequestError) return errorResponse(error);
    throw error;
  }
}
