import type { Mock } from 'vitest';

/** The JSON body the client POSTs to the indexer. */
export interface GraphQLRequestBody {
  query: string;
  variables?: Record<string, unknown>;
  operationName?: string;
}

/**
 * Extract the first request a mocked fetch received: its URL, its init, and its
 * parsed JSON body. Centralizes the casts the mock's untyped `calls` require so
 * individual tests stay free of unsafe member access.
 */
export function firstRequest(fetchImpl: Mock): {
  url: string;
  init: RequestInit;
  body: GraphQLRequestBody;
} {
  const call = fetchImpl.mock.calls[0];
  if (!call) throw new Error('fetch was not called');
  const [url, init] = call as [string, RequestInit];
  const body = JSON.parse(init.body as string) as GraphQLRequestBody;
  return { url, init, body };
}
