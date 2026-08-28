/**
 * Direct reads from Stellar RPC, bypassing the indexer.
 *
 * The indexer is the right source for history: it has decoded and stored
 * everything, and it pages backwards cheaply. RPC is the right source for the
 * present, since it sees an event the moment its ledger closes and needs no
 * infrastructure beyond a public endpoint. This module is the second path.
 *
 * What comes back is a {@link DecodedEvent}, the same type the indexer path
 * produces, so a consumer writes one handler either way. Three things differ
 * and are documented where they arise: ids are prefixed per ADR-024, the
 * retention window is roughly a week rather than all of history, and there is
 * no exhaustion signal, because a live tail has no end.
 *
 * `@stellar/stellar-sdk` is imported statically and stays a required peer
 * dependency. A dynamic import would make the failure mode a rejected promise
 * deep inside a call rather than an unresolved module at install time, which
 * is the wrong place to learn that a peer is missing.
 */

import { rpc, type xdr } from '@stellar/stellar-sdk';
import { z } from 'zod';

import { decodeScVal, decodeTopics, eventNameFromTopics } from './decode.js';
import { PulsarNetworkError, PulsarValidationError } from './errors.js';
import {
  ContractIdSchema,
  DecodedEventSchema,
  EVENT_QUERY_MAX_LIMIT,
  type DecodedEvent,
  type ResolvedPulsarConfig,
} from './types.js';

/** The prefix every RPC-sourced event id carries, per ADR-024. */
export const RPC_ID_PREFIX = 'rpc:';

/** Which contracts and topics to ask RPC for. */
export const LiveEventFilterSchema = z.strictObject({
  /** Restrict to these contracts. Omit to take every contract's events. */
  contractIds: z.array(ContractIdSchema).min(1).optional(),
  /**
   * Event type. Soroban RPC offers `contract` and `system` only.
   *
   * There is no `diagnostic` member, despite an example in the SDK's own
   * JSDoc showing one, per ADR-013.
   */
  type: z.enum(['contract', 'system']).optional(),
});

/** Which contracts and topics to ask RPC for. */
export type LiveEventFilter = z.infer<typeof LiveEventFilterSchema>;

const limitField = z.number().int().positive().max(EVENT_QUERY_MAX_LIMIT).optional();

/**
 * A live query, in exactly one of its two modes.
 *
 * Mirrors the protocol's own constraint rather than flattening it: RPC's
 * `getEvents` takes a starting ledger or a cursor and rejects both together.
 * ADR-025 enforces that twice, here at runtime and in {@link LiveEventQuery}
 * at compile time, because a JavaScript caller reaches this function with no
 * type checking behind them.
 */
export const LiveEventQuerySchema = z.union([
  z.strictObject({
    startLedger: z.number().int().positive(),
    cursor: z.undefined().optional(),
    limit: limitField,
    filter: LiveEventFilterSchema.optional(),
  }),
  z.strictObject({
    cursor: z.string().min(1),
    startLedger: z.undefined().optional(),
    limit: limitField,
    filter: LiveEventFilterSchema.optional(),
  }),
]);

/**
 * Where to start reading, by ledger or by cursor, never both.
 *
 * The `never` members are what reject the two bad shapes at compile time:
 * passing both fields, and passing neither.
 *
 * ```ts
 * let query: LiveEventQuery = { startLedger: 4378751 };
 * const page = await fetchLiveEvents(config, query);
 * query = { cursor: page.cursor };
 * ```
 *
 * Build each continuation fresh, as above. Spreading the previous query,
 * `{ ...query, cursor }`, carries `startLedger` forward and is a compile
 * error, which is the one place this shape costs a caller anything. It fails
 * loudly rather than sending a request the protocol rejects.
 */
export type LiveEventQuery =
  | { startLedger: number; cursor?: never; limit?: number; filter?: LiveEventFilter }
  | { cursor: string; startLedger?: never; limit?: number; filter?: LiveEventFilter };

/** One page of live events, with the cursor that continues it. */
export interface LiveEventsPage {
  /** The events in this page, oldest first. Empty when none matched yet. */
  readonly events: DecodedEvent[];
  /**
   * Where to resume, always present.
   *
   * Never null, per ADR-025. A page carrying events returns the last one's id;
   * an empty page returns a positional marker so paging continues from where
   * the scan stopped. RPC has no exhaustion signal to report, so neither does
   * this. The caller decides when to stop.
   */
  readonly cursor: string;
  /** The newest ledger RPC had closed when it answered. */
  readonly latestLedger: number;
}

/** How to poll in {@link liveEventStream}. */
export interface LiveEventStreamOptions {
  /** Milliseconds to wait after an empty page before asking again. */
  readonly pollIntervalMs?: number;
}

/** Default gap between polls, roughly a third of a ledger close. */
export const DEFAULT_POLL_INTERVAL_MS = 2000;

/**
 * Splits an RPC event id into its ledger-wide ordinal.
 *
 * The id has the form `{toid}-{eventOrder}`, and the second component is the
 * event's position within its ledger, per ADR-022. RPC offers no other index:
 * its `transactionIndex` and `operationIndex` repeat across events from
 * different transactions.
 *
 * @throws {PulsarValidationError} if the id is not that shape, carrying the id
 * it could not read. The id and the ordinal come from one field, so a
 * malformed id fails both at once rather than producing an event with a
 * plausible-looking wrong index.
 */
function eventIndexFromRpcId(rpcId: string, operation: string): number {
  const parts = rpcId.split('-');
  const ordinal = parts.length === 2 ? parts[1] : undefined;

  if (ordinal === undefined || !/^\d+$/.test(ordinal)) {
    throw new PulsarValidationError('RPC event ID is not in the expected {toid}-{ordinal} form', {
      operation,
      details: { rpcId },
    });
  }

  return Number(ordinal);
}

/** The shape this module reads off an RPC event, named so tests can build one. */
export interface RawRpcEvent {
  readonly id: string;
  readonly ledger: number;
  readonly ledgerClosedAt: string;
  readonly txHash: string;
  readonly topic: readonly xdr.ScVal[];
  readonly value: xdr.ScVal;
  readonly inSuccessfulContractCall: boolean;
  readonly contractId?: { toString(): string } | undefined;
}

/**
 * Maps one RPC event onto the type the indexer path also produces.
 *
 * @throws {PulsarValidationError} if the event does not decode into a valid
 * {@link DecodedEvent}, which covers a malformed id and a contract id RPC
 * reported in a shape this SDK does not recognize.
 */
export function toLiveDecodedEvent(event: RawRpcEvent, operation: string): DecodedEvent {
  const topics = decodeTopics([...event.topic]);
  const candidate = {
    id: `${RPC_ID_PREFIX}${event.id}`,
    contractId: event.contractId === undefined ? '' : String(event.contractId),
    ledger: event.ledger,
    txHash: event.txHash,
    eventIndex: eventIndexFromRpcId(event.id, operation),
    name: eventNameFromTopics(topics),
    topics,
    data: decodeScVal(event.value),
    rawTopics: event.topic.map((topic) => topic.toXDR('base64')),
    rawData: event.value.toXDR('base64'),
    emittedAt: event.ledgerClosedAt,
    inSuccessfulContractCall: event.inSuccessfulContractCall,
  };

  const result = DecodedEventSchema.safeParse(candidate);

  if (!result.success) {
    throw PulsarValidationError.fromZodError(result.error, {
      operation,
      details: { rpcId: event.id },
    });
  }

  return result.data;
}

/**
 * Builds the RPC server this config points at, failing clearly when it has none.
 *
 * Returns the URL alongside the server so the caller can name it in an error
 * without re-narrowing `config.rpcUrl`, which TypeScript cannot carry across
 * this call.
 */
function serverFor(
  config: ResolvedPulsarConfig,
  operation: string,
): { server: rpc.Server; url: string } {
  if (config.rpcUrl === undefined) {
    throw new PulsarValidationError('rpcUrl is required for direct RPC reads', {
      operation,
      details: { hint: 'Construct the client with an rpcUrl to use the live path' },
    });
  }

  return { server: new rpc.Server(config.rpcUrl), url: config.rpcUrl };
}

/**
 * Reads one page of events straight from Stellar RPC.
 *
 * Unlike the indexer path, this reaches only as far back as the node's
 * retention window, roughly a week at stock configuration. A `startLedger`
 * older than that is rejected by RPC, not by this SDK, because the window is
 * per-node and moves between calls.
 *
 * @param config - A resolved client config carrying `rpcUrl`.
 * @param query - Exactly one of `startLedger` or `cursor`, per ADR-025.
 * @throws {PulsarValidationError} if the query sets both modes or neither, if
 * the config has no `rpcUrl`, or if an event does not decode into a valid
 * {@link DecodedEvent}.
 * @throws {PulsarNetworkError} if RPC is unreachable or answers with an error,
 * with the underlying failure preserved as `cause`.
 */
export async function fetchLiveEvents(
  config: ResolvedPulsarConfig,
  query: LiveEventQuery,
): Promise<LiveEventsPage> {
  const operation = 'rpc.fetchLiveEvents';
  const validated = LiveEventQuerySchema.safeParse(query);

  if (!validated.success) {
    throw PulsarValidationError.fromZodError(validated.error, { operation });
  }

  const parsed = validated.data;
  const { server, url } = serverFor(config, operation);
  const filters = [
    {
      type: parsed.filter?.type ?? 'contract',
      ...(parsed.filter?.contractIds === undefined
        ? {}
        : { contractIds: parsed.filter.contractIds }),
    },
  ] as const;

  const request =
    parsed.cursor === undefined
      ? { startLedger: parsed.startLedger, filters: [...filters], limit: parsed.limit }
      : { cursor: parsed.cursor, filters: [...filters], limit: parsed.limit };

  let response;

  try {
    response = await server.getEvents(request as Parameters<rpc.Server['getEvents']>[0]);
  } catch (cause) {
    throw new PulsarNetworkError('Stellar RPC request failed', {
      operation,
      cause,
      url,
      status: null,
    });
  }

  if (typeof response?.cursor !== 'string' || !Array.isArray(response.events)) {
    throw new PulsarValidationError('Stellar RPC returned a response this SDK cannot read', {
      operation,
      details: { url },
    });
  }

  return {
    events: response.events.map((event) => toLiveDecodedEvent(event as RawRpcEvent, operation)),
    cursor: response.cursor,
    latestLedger: response.latestLedger,
  };
}

/** Resolves after `ms`, so an empty poll does not spin. */
function delay(ms: number): Promise<void> {
  return new Promise((resolve) => {
    setTimeout(resolve, ms);
  });
}

/**
 * Follows a contract's events as they close, fetching pages as needed.
 *
 * ```ts
 * for await (const event of liveEventStream(config, { startLedger: latest })) {
 *   if (event.name === 'deposit') break;
 * }
 * ```
 *
 * The loop does not end on its own. RPC reports no exhaustion, so reaching the
 * tip means empty pages rather than a final one, and the stream waits
 * `pollIntervalMs` and asks again. Breaking out of the `for await` is how a
 * caller stops, and nothing is prefetched, so breaking costs no extra request.
 *
 * Each iteration starts a fresh traversal from the original query, matching
 * `client.eventStream`. Iterating one value twice replays rather than sharing
 * a position.
 *
 * @throws {PulsarValidationError} or {@link PulsarNetworkError} out of the
 * `for await` loop, on whichever page fails. Events already yielded stay
 * valid; only the rest is lost.
 */
export function liveEventStream(
  config: ResolvedPulsarConfig,
  query: LiveEventQuery,
  options?: LiveEventStreamOptions,
): AsyncIterable<DecodedEvent> {
  const pollIntervalMs = options?.pollIntervalMs ?? DEFAULT_POLL_INTERVAL_MS;

  async function* walk(): AsyncGenerator<DecodedEvent> {
    let next: LiveEventQuery = query;

    for (;;) {
      const page = await fetchLiveEvents(config, next);

      for (const event of page.events) {
        yield event;
      }

      next = query.limit === undefined ? { cursor: page.cursor } : { cursor: page.cursor, limit: query.limit };

      if (page.events.length === 0) {
        await delay(pollIntervalMs);
      }
    }
  }

  return {
    [Symbol.asyncIterator]: (): AsyncIterator<DecodedEvent> => walk(),
  };
}
