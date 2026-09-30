import { type EventFilters } from '@/lib/events-filters';
import { type EventConnection } from '@/lib/graphql/schemas';

/**
 * The browser-side transport for interactive event pagination.
 *
 * This is the one piece that runs in the browser and talks to the explorer's
 * own `/api/graphql` route (never the indexer directly). That route has already
 * validated the response against the boundary schema server-side, so this layer
 * does not re-run Zod: doing so would pull the SDK-backed schemas (and the
 * decoder they carry) into the client bundle, which ADR-046 keeps out. It types
 * the result and guards the shape structurally instead.
 */

/** The variables for one `events` page request. */
export interface FetchEventsVariables extends EventFilters {
  contractId: string;
  cursor?: string;
  limit?: number;
}

interface FetchOptions {
  signal?: AbortSignal;
  /** Injectable for tests; defaults to the global fetch. */
  fetchImpl?: typeof fetch;
}

/** A failed page request, carrying the category and catalog code for the caller. */
export class EventFetchError extends Error {
  readonly category: string;
  readonly code: string | null;

  constructor(
    message: string,
    category: string,
    code: string | null,
    cause?: unknown,
  ) {
    super(message, cause === undefined ? undefined : { cause });
    this.name = 'EventFetchError';
    this.category = category;
    this.code = code;
  }
}

const GENERIC_MESSAGE = 'The request could not be completed';

/** Pull the `{ error }` shape the route returns on failure, defaulting safely. */
function readError(body: unknown): {
  category: string;
  code: string | null;
  message: string;
} {
  if (typeof body === 'object' && body !== null && 'error' in body) {
    const error = (body).error;
    if (typeof error === 'object' && error !== null) {
      const record = error as Record<string, unknown>;
      return {
        category:
          typeof record.category === 'string' ? record.category : 'unknown',
        code: typeof record.code === 'string' ? record.code : null,
        message:
          typeof record.message === 'string' ? record.message : GENERIC_MESSAGE,
      };
    }
  }
  return { category: 'unknown', code: null, message: GENERIC_MESSAGE };
}

/** Structurally confirm the success payload is an event connection. */
function asEventConnection(data: unknown): EventConnection {
  if (
    typeof data === 'object' &&
    data !== null &&
    'items' in data &&
    Array.isArray((data).items)
  ) {
    return data as EventConnection;
  }
  throw new EventFetchError(
    'The explorer returned an unexpected response',
    'internal',
    null,
  );
}

/**
 * Fetch one page of a contract's events through the explorer's dispatch route.
 *
 * @throws EventFetchError on a transport failure, a non-OK response (carrying
 * the route's category and code), or an unexpected payload shape.
 */
export async function fetchEventsPage(
  variables: FetchEventsVariables,
  options: FetchOptions = {},
): Promise<EventConnection> {
  const doFetch = options.fetchImpl ?? fetch;

  let response: Response;
  try {
    response = await doFetch('/api/graphql', {
      method: 'POST',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify({ operation: 'events', variables }),
      signal: options.signal ?? null,
    });
  } catch (cause) {
    throw new EventFetchError('Could not reach the explorer', 'network', null, cause);
  }

  let body: unknown;
  try {
    body = await response.json();
  } catch (cause) {
    throw new EventFetchError(
      'The explorer returned a malformed response',
      'network',
      null,
      cause,
    );
  }

  if (!response.ok) {
    const error = readError(body);
    throw new EventFetchError(error.message, error.category, error.code);
  }

  return asEventConnection((body as { data?: unknown }).data);
}
