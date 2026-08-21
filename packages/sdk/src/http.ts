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

/** HTTP methods the indexer API uses. */
export type HttpMethod = 'GET' | 'POST' | 'DELETE';

/** What a single request needs to know beyond the client's configuration. */
export interface RequestOptions<T extends z.ZodTypeAny> {
  /** Path relative to the indexer URL, such as `/health`. */
  readonly path: string;
  /** Schema for the payload inside the envelope's `data`. */
  readonly schema: T;
  /** The SDK operation making the request, used in error context. */
  readonly operation: string;
  /** Defaults to GET. */
  readonly method?: HttpMethod;
  /** Serialized as JSON. Omit for a request with no body. */
  readonly body?: unknown;
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
async function send<T extends z.ZodTypeAny>(
  config: ResolvedPulsarConfig,
  options: RequestOptions<T>,
  allowNotFound: boolean,
): Promise<RequestResult<z.infer<T>> | null> {
  const url = buildUrl(config.indexerUrl, options.path);
  const fetchImpl = config.fetchImpl ?? globalThis.fetch;
  const startedAt = performance.now();

  const method = options.method ?? 'GET';
  const hasBody = options.body !== undefined;

  let response: Response;
  try {
    response = await fetchImpl(url, {
      method,
      signal: AbortSignal.timeout(config.timeoutMs),
      headers: hasBody
        ? { accept: 'application/json', 'content-type': 'application/json' }
        : { accept: 'application/json' },
      ...(hasBody ? { body: JSON.stringify(options.body) } : {}),
    });
  } catch (cause) {
    const timedOut = cause instanceof DOMException && cause.name === 'TimeoutError';
    throw new PulsarNetworkError(
      timedOut ? `Request timed out after ${config.timeoutMs}ms` : 'Request failed',
      { operation: options.operation, cause, url, status: null, details: { method } },
    );
  }

  const latencyMs = performance.now() - startedAt;
  const body = await readJson(response, url, options.operation);

  const asError = ErrorEnvelopeSchema.safeParse(body);
  if (asError.success) {
    const { code, message } = asError.data.error;
    const details = { code, indexerMessage: message };

    // Absence, per ADR-019: only a 404 carrying not_found counts, and only
    // where the calling method is documented to return null.
    if (allowNotFound && response.status === 404 && code === 'not_found') {
      return null;
    }

    // A success status carrying an error envelope is the server contradicting
    // itself. The transport worked, so this is a response-shape problem.
    if (response.ok) {
      throw new PulsarValidationError(
        `Indexer returned HTTP ${response.status} with a ${code} error envelope`,
        { operation: options.operation, details: { ...details, url, stage: 'envelope' } },
      );
    }

    throw new PulsarNetworkError(`Indexer reported ${code}`, {
      operation: options.operation,
      status: response.status,
      url,
      details,
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

/**
 * Sends a request that must produce a result.
 *
 * @throws {PulsarNetworkError} on any transport failure, non-success status,
 * non-JSON body, or error envelope, including a 404.
 * @throws {PulsarValidationError} if the body does not match the expected
 * shape, or if a success status arrives carrying an error envelope.
 */
export async function request<T extends z.ZodTypeAny>(
  config: ResolvedPulsarConfig,
  options: RequestOptions<T>,
): Promise<RequestResult<z.infer<T>>> {
  const result = await send(config, options, false);

  // Unreachable: send only returns null when allowNotFound is true. Asserting
  // it here keeps the caller's type free of a null it can never receive.
  /* v8 ignore next 3 */
  if (result === null) {
    throw new PulsarNetworkError('Request unexpectedly reported absence', {
      operation: options.operation,
      status: null,
      url: options.path,
    });
  }

  return result;
}

/**
 * Sends a request whose resource may legitimately not exist.
 *
 * Returns null only for a 404 carrying a `not_found` envelope, per ADR-019.
 * Every other failure throws, including a bare 404, so a routing problem is
 * never mistaken for an absent record.
 */
export async function requestMaybe<T extends z.ZodTypeAny>(
  config: ResolvedPulsarConfig,
  options: RequestOptions<T>,
): Promise<RequestResult<z.infer<T>> | null> {
  return send(config, options, true);
}
