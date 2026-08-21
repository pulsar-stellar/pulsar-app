/**
 * The error hierarchy every SDK method throws from.
 *
 * Three rules hold across all of it:
 *
 * - **Nothing throws a bare `Error`.** A caller can branch on what went wrong
 *   with `instanceof` instead of matching on message text.
 * - **Every error carries the operation it came from**, so a failure two
 *   layers down still says which call the caller made.
 * - **Wrapping preserves the original.** The underlying failure is set as
 *   `cause` under ES2022 semantics, so the chain can be walked rather than
 *   flattened into a string.
 *
 * Absence is always `null`, never `undefined`. A missing contract, an absent
 * HTTP status, and an empty search result all say so explicitly.
 */

import type { ZodError, ZodIssue } from 'zod';

/** Structured context attached to every Pulsar error. */
export interface PulsarErrorOptions {
  /** The SDK operation that failed, such as `events.query` or `client.ping`. */
  readonly operation: string;
  /** The underlying failure, if this error wraps one. */
  readonly cause?: unknown;
  /** Identifiers worth having in a log line, such as a contract ID. */
  readonly details?: Readonly<Record<string, unknown>>;
}

/**
 * Base class for every error the SDK throws.
 *
 * Abstract on purpose: an error should say what kind of failure it is, and a
 * bare `PulsarError` says only that something went wrong. Catch it to handle
 * every SDK failure at once; never construct it.
 */
export abstract class PulsarError extends Error {
  /** The SDK operation that failed. */
  readonly operation: string;

  /** Identifiers describing the failure, frozen so a handler cannot edit them. */
  readonly details: Readonly<Record<string, unknown>>;

  protected constructor(message: string, options: PulsarErrorOptions) {
    super(message, options.cause === undefined ? undefined : { cause: options.cause });
    this.name = 'PulsarError';
    this.operation = options.operation;
    this.details = Object.freeze({ ...options.details });
  }

  /**
   * Renders the error for a log line, including the operation and any
   * identifiers.
   *
   * `Error.prototype.toString` gives only name and message, which drops
   * exactly the context that makes a production log entry actionable.
   */
  override toString(): string {
    const context = Object.entries(this.details)
      .map(([key, value]) => `${key}=${String(value)}`)
      .join(' ');
    const suffix = context === '' ? '' : ` ${context}`;
    return `${this.name}: ${this.message} [operation=${this.operation}${suffix}]`;
  }
}

/** Options for a network failure, adding the HTTP specifics when there are any. */
export interface PulsarNetworkErrorOptions extends PulsarErrorOptions {
  /** HTTP status, or null when the request never produced one. */
  readonly status?: number | null;
  /** The URL that was called, or null when the failure precedes a request. */
  readonly url?: string | null;
}

/**
 * A request did not produce a usable response.
 *
 * Covers an unreachable indexer or RPC endpoint, a timeout, a non-success HTTP
 * status, and a response body that could not be read. It does not cover a
 * response that arrived intact but failed validation; that is a
 * {@link PulsarValidationError}.
 */
export class PulsarNetworkError extends PulsarError {
  /** HTTP status, or null when the request never got one. */
  readonly status: number | null;

  /** The URL called, or null when the failure happened before the request. */
  readonly url: string | null;

  constructor(message: string, options: PulsarNetworkErrorOptions) {
    const status = options.status ?? null;
    const url = options.url ?? null;
    super(message, {
      ...options,
      details: { ...options.details, ...(status === null ? {} : { status }), ...(url === null ? {} : { url }) },
    });
    this.name = 'PulsarNetworkError';
    this.status = status;
    this.url = url;
  }
}

/**
 * A value did not match the schema it was parsed against.
 *
 * Thrown for caller input the SDK rejects and for a server response that does
 * not match the shape this SDK version expects. The Zod issues are exposed
 * rather than flattened, so a caller can report the offending field rather
 * than the whole message.
 */
export class PulsarValidationError extends PulsarError {
  /** Every issue the schema reported, in the order Zod produced them. */
  readonly issues: readonly ZodIssue[];

  constructor(
    message: string,
    options: PulsarErrorOptions & { readonly issues?: readonly ZodIssue[] },
  ) {
    super(message, options);
    this.name = 'PulsarValidationError';
    this.issues = Object.freeze([...(options.issues ?? [])]);
  }

  /**
   * Wraps a `ZodError` from a failed parse.
   *
   * The message names the first offending path so a log line is useful without
   * expanding the issue list, and the original error is kept as `cause`.
   */
  static fromZodError(
    error: ZodError,
    options: Omit<PulsarErrorOptions, 'cause'>,
  ): PulsarValidationError {
    const [first] = error.issues;
    const path = first === undefined || first.path.length === 0 ? '<root>' : first.path.join('.');
    const summary =
      first === undefined
        ? 'no issues reported'
        : `${error.issues.length} issue${error.issues.length === 1 ? '' : 's'}, first at ${path}: ${first.message}`;

    return new PulsarValidationError(`Validation failed: ${summary}`, {
      ...options,
      cause: error,
      issues: error.issues,
    });
  }
}

/**
 * Finds the first Pulsar error in a cause chain.
 *
 * Useful where an SDK error has been wrapped by a caller's own error and the
 * original context is needed. Returns null when the chain holds none, never
 * undefined, so a caller can branch on `=== null` without a second check.
 *
 * Chains are followed defensively: a cause cycle terminates the walk instead
 * of looping forever.
 */
export function findPulsarError(error: unknown): PulsarError | null {
  const seen = new Set<unknown>();
  let current: unknown = error;

  while (current !== null && current !== undefined && !seen.has(current)) {
    if (current instanceof PulsarError) {
      return current;
    }
    seen.add(current);
    current = current instanceof Error ? current.cause : null;
  }

  return null;
}
