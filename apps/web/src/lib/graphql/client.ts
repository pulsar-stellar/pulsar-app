import 'server-only';

import { z } from 'zod';


import { categoryForCode, GraphQLRequestError } from './errors';
import { graphqlErrorSchema } from './schemas';

import { getIndexerBaseUrl } from '@/lib/env';

/**
 * The server-side GraphQL transport.
 *
 * This runs in Node (Server Components and the `/api/graphql` route handler),
 * never in the browser: keeping the fetch and its validation server-side is
 * what lets the explorer reuse the SDK schemas without shipping the SDK's
 * Node-only decode path to the client (ADR-046).
 *
 * Every response is checked before it is trusted. Indexer-reported errors take
 * precedence over the data shape, so a field-level failure surfaces as the
 * indexer's own categorized error rather than an opaque shape mismatch.
 */

/** A GraphQL operation to send: the document, its variables, and an optional name. */
export interface GraphQLRequestInput {
  query: string;
  variables?: Record<string, unknown>;
  operationName?: string;
}

interface ExecuteOptions {
  signal?: AbortSignal;
  /** Injectable for tests; defaults to the global fetch. */
  fetchImpl?: typeof fetch;
}

// The envelope, validated loosely first so indexer errors are read before the
// data payload is held to its schema.
const baseEnvelopeSchema = z.object({
  data: z.unknown().nullish(),
  errors: z.array(graphqlErrorSchema).optional(),
});

/**
 * Execute a GraphQL operation against the indexer and return its validated data.
 *
 * @throws GraphQLRequestError on a transport failure, a malformed response, an
 * indexer-reported error, or a data payload that does not match `dataSchema`.
 */
export async function executeGraphQL<T extends z.ZodType>(
  dataSchema: T,
  request: GraphQLRequestInput,
  options: ExecuteOptions = {},
): Promise<z.infer<T>> {
  let url: string;
  try {
    url = `${getIndexerBaseUrl()}/graphql`;
  } catch (cause) {
    throw new GraphQLRequestError('The indexer URL is not configured', {
      category: 'internal',
      cause,
    });
  }
  const doFetch = options.fetchImpl ?? fetch;

  let response: Response;
  try {
    response = await doFetch(url, {
      method: 'POST',
      headers: {
        'content-type': 'application/json',
        accept: 'application/json',
      },
      body: JSON.stringify({
        query: request.query,
        variables: request.variables ?? {},
        operationName: request.operationName,
      }),
      signal: options.signal ?? null,
      cache: 'no-store',
    });
  } catch (cause) {
    throw new GraphQLRequestError('Could not reach the indexer', {
      category: 'network',
      cause,
    });
  }

  let body: unknown;
  try {
    body = await response.json();
  } catch (cause) {
    throw new GraphQLRequestError('The indexer returned a malformed response', {
      category: 'network',
      cause,
    });
  }

  const envelope = baseEnvelopeSchema.safeParse(body);
  if (!envelope.success) {
    throw new GraphQLRequestError(
      'The indexer response was not a valid GraphQL envelope',
      { category: 'internal', cause: envelope.error },
    );
  }

  const { data, errors } = envelope.data;
  if (errors && errors.length > 0) {
    const first = errors[0];
    const code = first?.extensions?.code ?? null;
    throw new GraphQLRequestError(first?.message ?? 'The indexer reported an error', {
      category: categoryForCode(code),
      code,
    });
  }

  const parsed = dataSchema.safeParse(data);
  if (!parsed.success) {
    throw new GraphQLRequestError(
      'The indexer response did not match the expected shape',
      { category: 'internal', cause: parsed.error },
    );
  }

  return parsed.data;
}
