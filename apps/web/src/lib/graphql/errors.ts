/**
 * Errors surfaced by the GraphQL data layer, and the mapping from the indexer's
 * catalog codes to the handful of categories the UI actually branches on.
 *
 * The indexer never sends an internal cause over the wire (ADR-043): a GraphQL
 * error message is already the generic, client-safe catalog message. This layer
 * keeps that message and attaches the machine-readable code and a coarse
 * category so a screen can decide between "not found" (render an empty state)
 * and everything else (render an error) without string-matching messages.
 */

/** The coarse buckets the UI branches on. */
export type GraphQLErrorCategory =
  | 'validation'
  | 'not_found'
  | 'network'
  | 'internal'
  | 'unknown';

/**
 * Bucket a catalog code by its prefix.
 *
 * The indexer's codes are namespaced (`VALIDATION_*`, `NOT_FOUND_*`,
 * `INTERNAL_*`), so a prefix match is stable without enumerating every code and
 * degrades to `unknown` for anything unrecognized.
 */
export function categoryForCode(
  code: string | null | undefined,
): GraphQLErrorCategory {
  if (!code) return 'unknown';
  if (code.startsWith('VALIDATION_')) return 'validation';
  if (code.startsWith('NOT_FOUND_')) return 'not_found';
  if (code.startsWith('INTERNAL_')) return 'internal';
  return 'unknown';
}

interface GraphQLRequestErrorOptions {
  category: GraphQLErrorCategory;
  code?: string | null;
  cause?: unknown;
}

/**
 * A failure from a GraphQL request: a transport problem, a malformed response,
 * or a field-level error the indexer reported.
 */
export class GraphQLRequestError extends Error {
  readonly category: GraphQLErrorCategory;
  readonly code: string | null;

  constructor(message: string, options: GraphQLRequestErrorOptions) {
    super(message, options.cause === undefined ? undefined : { cause: options.cause });
    this.name = 'GraphQLRequestError';
    this.category = options.category;
    this.code = options.code ?? null;
  }

  /** True when the target was looked up and found absent. */
  get isNotFound(): boolean {
    return this.category === 'not_found';
  }
}
