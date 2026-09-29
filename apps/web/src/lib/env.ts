/**
 * Boundary for the one piece of environment the explorer needs: the base URL of
 * the Pulsar indexer it reads from.
 *
 * The value is public by design (it is inlined into the browser under the
 * `NEXT_PUBLIC_` prefix), so nothing secret passes through here. It is still
 * validated at the boundary: a missing or malformed URL is a deployment mistake
 * worth failing loudly for, not a silent `undefined` that surfaces as an opaque
 * fetch error deep in a request.
 */

/** Thrown when the indexer URL is absent or not a usable http(s) URL. */
export class IndexerUrlError extends Error {
  constructor(message: string) {
    super(message);
    this.name = 'IndexerUrlError';
  }
}

/**
 * Resolve and validate the indexer base URL, with any trailing slash removed so
 * callers can join `/graphql` without doubling it.
 *
 * Reads `process.env.NEXT_PUBLIC_PULSAR_INDEXER_URL` as a full literal so Next
 * inlines it in client bundles; server code reads the same value at runtime.
 */
export function getIndexerBaseUrl(): string {
  const raw = process.env.NEXT_PUBLIC_PULSAR_INDEXER_URL;
  if (!raw) {
    throw new IndexerUrlError(
      'NEXT_PUBLIC_PULSAR_INDEXER_URL is not set; point it at the Pulsar indexer',
    );
  }

  let url: URL;
  try {
    url = new URL(raw);
  } catch {
    throw new IndexerUrlError(
      `NEXT_PUBLIC_PULSAR_INDEXER_URL is not a valid URL: ${raw}`,
    );
  }

  if (url.protocol !== 'http:' && url.protocol !== 'https:') {
    throw new IndexerUrlError(
      `NEXT_PUBLIC_PULSAR_INDEXER_URL must use http or https: ${raw}`,
    );
  }

  return raw.replace(/\/+$/, '');
}
