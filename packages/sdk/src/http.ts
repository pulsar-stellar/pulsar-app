/**
 * The one place the SDK talks to the indexer over HTTP.
 *
 * Every response goes through the same sequence: send with a timeout, check
 * for a transport failure, check for the error envelope, unwrap `data`, then
 * validate it against the schema for that route. Doing it once here means no
 * method can skip a step, and a caller sees the same error types whichever
 * method they called.
 *
 * The wire format is fixed by ADR-017.
 */

import type { z } from 'zod';

import { PulsarNetworkError, PulsarValidationError } from './errors.js';
import { EnvelopeSchema, ErrorEnvelopeSchema, type ResolvedPulsarConfig } from './types.js';

/** What a single request needs to know beyond the client's configuration. */
export interface RequestOptions<T extends z.ZodTypeAny> {
  /** Path relative to the indexer URL, such as `/health`. */
  readonly path: string;
  /** Schema for the payload inside the envelope's `data`. */
  readonly schema: T;
  /** The SDK operation making the request, used in error context. */
  readonly operation: string;
}

/** A validated response, with the timings that came with it. */
export interface RequestResult<T> {
  readonly data: T;
  /** Round-trip time measured here, in milliseconds. */
  readonly latencyMs: number;
  /** The indexer's own timing, or null when it did not report one. */
  readonly serverTookMs: number | null;
  /** Cursor for the next page, or null on a response that does not paginate. */
  readonly nextCursor: string | null;
}

/** Joins the configured base URL to a path without doubling or dropping a slash. */
function buildUrl(baseUrl: string, path: string): string {
  return `${baseUrl.replace(/\/+$/, '')}${path.startsWith('/') ? path : `/${path}`}`;
}

/**
 * Reads a response body as JSON, converting a parse failure into a network
 * error.
 *
 * A body that is not JSON means the thing that answered is not the indexer, or
 * is not well, so it belongs with transport failures rather than with schema
 * validation. A caller retrying on `PulsarNetworkError` should retry this.
 */
async function readJson(
  response: Response,
  url: string,
  operation: string,
): Promise<unknown> {
  try {
    return await response.json();
  } catch (cause) {
    throw new PulsarNetworkError('Response body was not valid JSON', {
      operation,
      cause,
      status: response.status,
      url,
    });
  }
}

/**
 * Sends one request to the indexer and validates the result.
 *
 * @throws {PulsarNetworkError} if the request fails, times out, returns a
 * non-success status, returns a body that is not JSON, or returns the error
 * envelope.
 * @throws {PulsarValidationError} if the body is JSON but does not match the
 * expected shape.
 */
export async function request<T extends z.ZodTypeAny>(
  config: ResolvedPulsarConfig,
  options: RequestOptions<T>,
): Promise<RequestResult<z.infer<T>>> {
  const url = buildUrl(config.indexerUrl, options.path);
  const fetchImpl = config.fetchImpl ?? globalThis.fetch;
  const startedAt = performance.now();

  let response: Response;
  try {
    response = await fetchImpl(url, {
      signal: AbortSignal.timeout(config.timeoutMs),
      headers: { accept: 'application/json' },
    });
  } catch (cause) {
    const timedOut = cause instanceof DOMException && cause.name === 'TimeoutError';
    throw new PulsarNetworkError(
      timedOut ? `Request timed out after ${config.timeoutMs}ms` : 'Request failed',
      { operation: options.operation, cause, url, status: null },
    );
  }

  const latencyMs = performance.now() - startedAt;
  const body = await readJson(response, url, options.operation);

  const asError = ErrorEnvelopeSchema.safeParse(body);
  if (asError.success) {
    throw new PulsarNetworkError(`Indexer reported ${asError.data.error.code}`, {
      operation: options.operation,
      status: response.status,
      url,
      details: { code: asError.data.error.code, indexerMessage: asError.data.error.message },
    });
  }

  if (!response.ok) {
    throw new PulsarNetworkError(`Indexer returned HTTP ${response.status}`, {
      operation: options.operation,
      status: response.status,
      url,
    });
  }

  const envelope = EnvelopeSchema.safeParse(body);
  if (!envelope.success) {
    throw PulsarValidationError.fromZodError(envelope.error, {
      operation: options.operation,
      details: { url, stage: 'envelope' },
    });
  }

  const payload = options.schema.safeParse(envelope.data.data);
  if (!payload.success) {
    throw PulsarValidationError.fromZodError(payload.error, {
      operation: options.operation,
      details: { url, stage: 'payload' },
    });
  }

  return {
    data: payload.data,
    latencyMs,
    serverTookMs: envelope.data.meta?.took_ms ?? null,
    nextCursor: envelope.data.next_cursor ?? null,
  };
}
