/**
 * The SDK's entry point.
 *
 * A client is configured once and validated at construction, so a
 * misconfiguration surfaces where it was made rather than at the first request
 * that happens to use it. Configuration is parsed, not merely type-checked:
 * callers in plain JavaScript, and callers passing values read from the
 * environment, get the same guarantees as a TypeScript caller.
 */

import { PulsarValidationError } from './errors.js';
import { PulsarConfigSchema, type PulsarConfig, type ResolvedPulsarConfig } from './types.js';

/**
 * A configured client for the Pulsar indexer and for direct RPC reads.
 *
 * Construction validates the whole configuration and fills in defaults, so
 * every method can rely on a complete config rather than re-checking it.
 *
 * @throws {PulsarValidationError} if the configuration is not valid.
 */
export class PulsarClient {
  readonly #config: ResolvedPulsarConfig;

  /**
   * Validates `config` and keeps the parsed result.
   *
   * @param config - Indexer URL, and optionally an RPC URL, network, fetch
   * implementation, and timeout.
   * @throws {PulsarValidationError} carrying every schema issue, with the
   * underlying `ZodError` as its cause.
   */
  constructor(config: PulsarConfig) {
    const result = PulsarConfigSchema.safeParse(config);

    if (!result.success) {
      throw PulsarValidationError.fromZodError(result.error, {
        operation: 'configureClient',
      });
    }

    this.#config = Object.freeze(result.data);
  }

  /**
   * The configuration in force, with defaults filled in.
   *
   * Frozen: a client's configuration is fixed at construction, and a caller
   * that wants different settings builds a second client rather than mutating
   * this one out from under in-flight requests.
   */
  get config(): ResolvedPulsarConfig {
    return this.#config;
  }
}
