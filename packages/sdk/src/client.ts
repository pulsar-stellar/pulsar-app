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
import { request, requestMaybe } from './http.js';
import {
  ContractIdSchema,
  ContractInfoPayloadSchema,
  ContractListPayloadSchema,
  EventListPayloadSchema,
  EventQuerySchema,
  HealthPayloadSchema,
  PulsarConfigSchema,
  toContractInfo,
  toDecodedEvent,
  type ContractInfo,
  type EventQuery,
  type EventsPage,
  type PingResult,
  type PulsarConfig,
  type ResolvedPulsarConfig,
} from './types.js';

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

  /**
   * Checks that the indexer is reachable and reports what it says about itself.
   *
   * Use it to fail fast at startup rather than on a user's first query. The
   * round-trip time is measured here and reported separately from the
   * indexer's own `took_ms`, since the difference between the two is the
   * network, which is usually the thing worth knowing.
   *
   * A healthy indexer that reports `ok: false` is a successful call returning
   * `ok: false`, not a thrown error. The request worked; the answer was bad
   * news.
   *
   * @throws {PulsarNetworkError} if the indexer is unreachable, times out,
   * returns a non-success status, answers with something that is not JSON, or
   * returns the error envelope.
   * @throws {PulsarValidationError} if the response is JSON but not the shape
   * this SDK version expects.
   */
  async ping(): Promise<PingResult> {
    const result = await request(this.#config, {
      path: '/health',
      schema: HealthPayloadSchema,
      operation: 'client.ping',
    });

    return {
      ok: result.data.ok,
      version: result.data.version,
      latestLedger: result.data.latest_ledger ?? null,
      trackedContracts: result.data.tracked_contracts ?? null,
      latencyMs: result.latencyMs,
      serverTookMs: result.serverTookMs,
    };
  }

  /**
   * Asks the indexer to start tracking a contract.
   *
   * Idempotent per ADR-018: registering a contract that is already tracked
   * succeeds and returns the existing record, with its indexing progress
   * untouched. A caller that loses the response to a network failure can
   * simply call again.
   *
   * The contract ID is validated here before any request is sent, so an
   * obvious mistake costs nothing and reports the problem where it was made.
   *
   * @param contractId - A Soroban contract ID: `C` followed by 55 base32
   * characters.
   * @throws {PulsarValidationError} if the contract ID is malformed, or if the
   * indexer's response is not the shape this SDK version expects.
   * @throws {PulsarNetworkError} if the indexer is unreachable, times out,
   * returns a non-success status, answers with something that is not JSON, or
   * returns the error envelope.
   */
  async registerContract(contractId: string): Promise<ContractInfo> {
    const validated = ContractIdSchema.safeParse(contractId);

    if (!validated.success) {
      throw PulsarValidationError.fromZodError(validated.error, {
        operation: 'client.registerContract',
        details: { contractId },
      });
    }

    const result = await request(this.#config, {
      path: '/contracts',
      method: 'POST',
      body: { contract_id: validated.data },
      schema: ContractInfoPayloadSchema,
      operation: 'client.registerContract',
    });

    return toContractInfo(result.data);
  }

  /**
   * Looks up one contract the indexer is tracking.
   *
   * Returns null when the indexer says it is not tracking that contract, which
   * per ADR-019 means a 404 carrying a `not_found` envelope. A 404 without
   * that envelope is a routing or proxy problem rather than an absent record,
   * and throws.
   *
   * @param contractId - A Soroban contract ID.
   * @throws {PulsarValidationError} if the contract ID is malformed, or if the
   * response is not the shape this SDK version expects.
   * @throws {PulsarNetworkError} if the indexer is unreachable, times out, or
   * fails in any way other than a well-formed absence.
   */
  async getContract(contractId: string): Promise<ContractInfo | null> {
    const validated = ContractIdSchema.safeParse(contractId);

    if (!validated.success) {
      throw PulsarValidationError.fromZodError(validated.error, {
        operation: 'client.getContract',
        details: { contractId },
      });
    }

    const result = await requestMaybe(this.#config, {
      path: `/contracts/${encodeURIComponent(validated.data)}`,
      schema: ContractInfoPayloadSchema,
      operation: 'client.getContract',
    });

    return result === null ? null : toContractInfo(result.data);
  }

  /**
   * Lists every contract the indexer is tracking.
   *
   * Takes no arguments and is not paginated. The tracked-contract list is
   * bounded by what an operator has registered, unlike event history, which
   * grows with protocol activity.
   *
   * An empty list is an empty array, not null: the indexer answered, and the
   * answer is that it tracks nothing yet. Absence of a resource and absence of
   * contents are different facts.
   *
   * @throws {PulsarNetworkError} if the indexer is unreachable, times out,
   * returns a non-success status, or answers with something that is not JSON.
   * @throws {PulsarValidationError} if the response is not the shape this SDK
   * version expects.
   */
  async listContracts(): Promise<ContractInfo[]> {
    const result = await request(this.#config, {
      path: '/contracts',
      schema: ContractListPayloadSchema,
      operation: 'client.listContracts',
    });

    return result.data.items.map(toContractInfo);
  }

  /**
   * Queries one contract's decoded event history.
   *
   * Paging is by opaque cursor: pass `nextCursor` from a page back as
   * `query.cursor` to get the next one, and stop when `nextCursor` is null.
   * The cursor is never interpreted here, so the indexer can change its
   * encoding without breaking a caller.
   *
   * A contract the indexer is not tracking throws rather than returning an
   * empty page, per ADR-021. "Not indexed" and "no matching events" are
   * different facts, and the most common integration mistake, querying a
   * contract nobody registered, should not look like a valid empty result.
   *
   * @param contractId - The contract whose events to read.
   * @param query - Optional filters. Every field is optional; `limit` and
   * `order` take defaults when omitted.
   * @throws {PulsarValidationError} if the contract ID or the query is
   * malformed, or if the response is not the shape this SDK version expects.
   * @throws {PulsarNetworkError} if the indexer is unreachable, times out, or
   * returns a non-success status, including the 404 for an untracked contract.
   */
  async events(contractId: string, query?: EventQuery): Promise<EventsPage> {
    const validatedId = ContractIdSchema.safeParse(contractId);

    if (!validatedId.success) {
      throw PulsarValidationError.fromZodError(validatedId.error, {
        operation: 'client.events',
        details: { contractId },
      });
    }

    const validatedQuery = EventQuerySchema.safeParse(query ?? {});

    if (!validatedQuery.success) {
      throw PulsarValidationError.fromZodError(validatedQuery.error, {
        operation: 'client.events',
        details: { contractId },
      });
    }

    const filters = validatedQuery.data;
    const result = await request(this.#config, {
      path: `/contracts/${encodeURIComponent(validatedId.data)}/events`,
      schema: EventListPayloadSchema,
      operation: 'client.events',
      query: {
        name: filters.name,
        from_ledger: filters.fromLedger,
        to_ledger: filters.toLedger,
        topic_contains: filters.topicContains,
        limit: filters.limit,
        cursor: filters.cursor,
        order: filters.order,
      },
    });

    return {
      items: result.data.items.map(toDecodedEvent),
      nextCursor: result.nextCursor,
    };
  }
}
