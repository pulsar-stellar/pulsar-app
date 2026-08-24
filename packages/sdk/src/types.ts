/**
 * Public types for the Pulsar SDK, each paired with the Zod schema that
 * validates it at runtime.
 *
 * The schemas are the source of truth and the types are inferred from them, so
 * the two cannot drift apart. Nothing here trusts its input: data arriving from
 * the indexer, from RPC, or from a caller is parsed before it is used.
 *
 * Two validation postures are used deliberately:
 *
 * - **Caller input is strict.** A misspelled key in a query is a mistake worth
 *   reporting, not a field to ignore silently.
 * - **Server responses are lenient.** Unknown keys are stripped, so an indexer
 *   that adds a field does not break an SDK built before it existed.
 *
 * Field names follow the shapes verified in ADR-013 rather than the planning
 * draft, which was written before the current `@stellar/stellar-sdk` release.
 */

import { z } from 'zod';

/** Networks the toolkit can address. */
export const PulsarNetworkSchema = z.enum(['testnet', 'futurenet', 'mainnet', 'local']);

/** A Stellar network name. */
export type PulsarNetwork = z.infer<typeof PulsarNetworkSchema>;

/**
 * Matches a Soroban contract ID: the letter C followed by 55 base32 characters,
 * 56 in total.
 *
 * Checking the shape here is not a claim that the contract exists or that its
 * checksum is valid. It rejects the common mistakes, a truncated paste or an
 * account ID starting with G, before they reach a network call.
 */
export const ContractIdSchema = z
  .string()
  .regex(/^C[A-Z2-7]{55}$/, { error: 'Contract ID must be 56 characters starting with C' });

/**
 * One entry of a decoded Soroban map.
 *
 * A map is carried as an ordered array of these rather than as an object or a
 * JavaScript `Map`, per ADR-023. Soroban map keys are `ScVal`s, so a key can be
 * a Symbol, an Address, or an integer, and two keys of different types can
 * render to the same string. The array keeps the key's type, the wire ordering,
 * and any duplicate entries, and it survives `JSON.stringify` unchanged.
 */
export interface DecodedMapEntry {
  readonly key: DecodedValue;
  readonly value: DecodedValue;
}

/**
 * A single decoded contract value.
 *
 * Mirrors the `ScVal` variants the decoder produces, per ADR-023.
 *
 * Integers wider than 32 bits are carried as strings so that no precision is
 * lost passing through JSON, the same reasoning ADR-021 applies to event ids.
 * `u32` and `i32` stay numbers because they always fit. `bytes` is hex rather
 * than base64 so a value is readable next to the raw XDR it came from.
 *
 * `timepoint` and `duration` are unsigned 64-bit second counts, so they are
 * strings for the same reason.
 *
 * `tuple` is not reachable from XDR alone, since Soroban encodes a tuple as a
 * vector. Only a decoder holding the contract's spec can tell the two apart,
 * so the RPC path emits `vec` and the variant exists for the indexer.
 *
 * `unknown` carries the base64 XDR of anything this SDK version cannot name,
 * so a protocol addition degrades to an opaque value instead of failing the
 * whole page.
 */
export type DecodedValue =
  | { type: 'address'; value: string }
  | { type: 'symbol'; value: string }
  | { type: 'string'; value: string }
  | { type: 'bool'; value: boolean }
  | { type: 'bytes'; value: string }
  | { type: 'u32'; value: number }
  | { type: 'i32'; value: number }
  | { type: 'u64'; value: string }
  | { type: 'i64'; value: string }
  | { type: 'u128'; value: string }
  | { type: 'i128'; value: string }
  | { type: 'u256'; value: string }
  | { type: 'i256'; value: string }
  | { type: 'timepoint'; value: string }
  | { type: 'duration'; value: string }
  | { type: 'vec'; value: DecodedValue[] }
  | { type: 'map'; value: DecodedMapEntry[] }
  | { type: 'tuple'; value: DecodedValue[] }
  | { type: 'void' }
  | { type: 'unknown'; xdr: string };

/** Validates one {@link DecodedMapEntry}. */
export const DecodedMapEntrySchema: z.ZodType<DecodedMapEntry> = z.lazy(() =>
  z.object({ key: DecodedValueSchema, value: DecodedValueSchema }),
);

/**
 * Validates a {@link DecodedValue}.
 *
 * Declared with an explicit annotation and built lazily because the type is
 * recursive: `vec`, `map`, and `tuple` each contain more decoded values, and
 * inference cannot close that loop on its own.
 */
export const DecodedValueSchema: z.ZodType<DecodedValue> = z.lazy(() =>
  z.discriminatedUnion('type', [
    z.object({ type: z.literal('address'), value: z.string() }),
    z.object({ type: z.literal('symbol'), value: z.string() }),
    z.object({ type: z.literal('string'), value: z.string() }),
    z.object({ type: z.literal('bool'), value: z.boolean() }),
    z.object({ type: z.literal('bytes'), value: z.string() }),
    z.object({ type: z.literal('u32'), value: z.number().int().nonnegative() }),
    z.object({ type: z.literal('i32'), value: z.number().int() }),
    z.object({ type: z.literal('u64'), value: z.string() }),
    z.object({ type: z.literal('i64'), value: z.string() }),
    z.object({ type: z.literal('u128'), value: z.string() }),
    z.object({ type: z.literal('i128'), value: z.string() }),
    z.object({ type: z.literal('u256'), value: z.string() }),
    z.object({ type: z.literal('i256'), value: z.string() }),
    z.object({ type: z.literal('timepoint'), value: z.string() }),
    z.object({ type: z.literal('duration'), value: z.string() }),
    z.object({ type: z.literal('vec'), value: z.array(DecodedValueSchema) }),
    z.object({ type: z.literal('map'), value: z.array(DecodedMapEntrySchema) }),
    z.object({ type: z.literal('tuple'), value: z.array(DecodedValueSchema) }),
    z.object({ type: z.literal('void') }),
    z.object({ type: z.literal('unknown'), xdr: z.string().min(1) }),
  ]),
);

/**
 * One contract event as the indexer stores and serves it.
 *
 * Both the decoded form and the raw XDR it came from are carried. The decoded
 * form is what a caller reads; the raw form is provenance, so anyone can check
 * the decoding rather than take it on trust.
 */
export const DecodedEventSchema = z.object({
  /** The indexer's own primary key. Opaque, and not the RPC event ID. */
  id: z.string().min(1),
  contractId: ContractIdSchema,
  ledger: z.number().int().nonnegative(),
  txHash: z.string().min(1),
  /**
   * The event's ordinal position within its ledger, per ADR-022.
   *
   * Ledger-wide rather than per-transaction, because Soroban RPC has no
   * per-transaction index to offer: its `transactionIndex` and
   * `operationIndex` repeat across events, and only the event id's second
   * component increments. Events from one transaction stay contiguous, so
   * ordering within a transaction still works.
   */
  eventIndex: z.number().int().nonnegative(),
  /** The event's name, decoded from its first topic. */
  name: z.string().min(1),
  topics: z.array(DecodedValueSchema),
  data: DecodedValueSchema,
  /** Base64 XDR of each topic, in the same order as `topics`. */
  rawTopics: z.array(z.string()),
  /** Base64 XDR of the event payload. */
  rawData: z.string(),
  /** Ledger close time as an ISO 8601 timestamp. */
  emittedAt: z.iso.datetime({ offset: true }),
});

/** A decoded contract event. */
export type DecodedEvent = z.infer<typeof DecodedEventSchema>;

/** Whether the indexer is currently following a contract. */
export const ContractStatusSchema = z.enum(['active', 'paused', 'error']);

/** Indexing status for a tracked contract. */
export type ContractStatus = z.infer<typeof ContractStatusSchema>;

/**
 * What the indexer knows about a contract it tracks.
 *
 * `firstIndexedLedger` is null until the first poll completes, so a contract
 * registered a moment ago reports null rather than a misleading zero.
 */
export const ContractInfoSchema = z.object({
  contractId: ContractIdSchema,
  /** When the contract was registered, ISO 8601. */
  addedAt: z.iso.datetime({ offset: true }),
  firstIndexedLedger: z.number().int().nonnegative().nullable(),
  lastIndexedLedger: z.number().int().nonnegative(),
  status: ContractStatusSchema,
});

/** A contract tracked by the indexer. */
export type ContractInfo = z.infer<typeof ContractInfoSchema>;

/**
 * A tracked contract as it appears on the wire.
 *
 * snake_case per ADR-017, and the identifier is `id` rather than `contract_id`
 * because that is the column name in the indexer's `contracts` table. The SDK
 * maps this to {@link ContractInfo} at its boundary.
 */
export const ContractInfoPayloadSchema = z.object({
  id: ContractIdSchema,
  added_at: z.iso.datetime({ offset: true }),
  first_indexed_ledger: z.number().int().nonnegative().nullable().optional(),
  last_indexed_ledger: z.number().int().nonnegative(),
  status: ContractStatusSchema,
});

/**
 * The payload `GET /contracts` returns.
 *
 * Note the extra nesting: the envelope's `data` holds `{ items: [...] }`
 * rather than the array directly, per section 7.2. Single-contract routes put
 * the record straight in `data`, so the two are not interchangeable.
 */
export const ContractListPayloadSchema = z.object({
  items: z.array(ContractInfoPayloadSchema),
});

/** Converts a wire payload into the camelCase shape callers see. */
export function toContractInfo(payload: z.infer<typeof ContractInfoPayloadSchema>): ContractInfo {
  return {
    contractId: payload.id,
    addedAt: payload.added_at,
    firstIndexedLedger: payload.first_indexed_ledger ?? null,
    lastIndexedLedger: payload.last_indexed_ledger,
    status: payload.status,
  };
}

/** The largest page the indexer will return. */
export const EVENT_QUERY_MAX_LIMIT = 500;

/** The page size used when a query does not ask for one. */
export const EVENT_QUERY_DEFAULT_LIMIT = 50;

/**
 * Filters for a query against one contract's event history.
 *
 * The contract is not part of this shape. It is a path parameter on the route,
 * so it is a separate argument to `events`, which keeps the type aligned with
 * the wire: path params in the path, filters in the query string.
 *
 * Strict: an unrecognized key is an error rather than a silently ignored
 * field, because a typo in a filter would otherwise return a confidently wrong
 * result set.
 *
 * `cursor` is opaque and comes from a previous response. Unlike Soroban RPC's
 * `getEvents`, which forbids combining a cursor with a ledger range, the
 * indexer accepts both: it owns its history and does not page through a
 * retention window. See ADR-013.
 */
export const EventQuerySchema = z.strictObject({
  /** Event name to match exactly, such as `transfer`. */
  name: z.string().min(1).optional(),
  fromLedger: z.number().int().nonnegative().optional(),
  toLedger: z.number().int().nonnegative().optional(),
  /** Substring match against decoded topic values. */
  topicContains: z.string().min(1).optional(),
  limit: z.number().int().min(1).max(EVENT_QUERY_MAX_LIMIT).default(EVENT_QUERY_DEFAULT_LIMIT),
  cursor: z.string().min(1).optional(),
  order: z.enum(['asc', 'desc']).default('desc'),
});

/**
 * An event query as a caller writes it, with `limit` and `order` optional.
 */
export type EventQuery = z.input<typeof EventQuerySchema>;

/**
 * An event query after parsing, with every default filled in. This is what
 * request-building code works with.
 */
export type ResolvedEventQuery = z.output<typeof EventQuerySchema>;

/**
 * One page of events, with the cursor that fetches the next one.
 *
 * `nextCursor` is null on the last page. Null and an absent wire field both
 * arrive here as null, per ADR-021, so a caller pages until it sees null and
 * never has to check two falsy values.
 */
export interface EventsPage {
  readonly items: DecodedEvent[];
  readonly nextCursor: string | null;
}

/**
 * A decoded event as it appears on the wire.
 *
 * snake_case per ADR-017, matching the indexer's `events` table columns. The
 * identifier is a string because that table uses `BIGSERIAL`, whose range
 * exceeds what a JSON number carries safely; see ADR-021.
 */
export const DecodedEventPayloadSchema = z.object({
  id: z.string().min(1),
  contract_id: ContractIdSchema,
  ledger: z.number().int().nonnegative(),
  tx_hash: z.string().min(1),
  event_index: z.number().int().nonnegative(),
  name: z.string().min(1),
  topics_json: z.array(DecodedValueSchema),
  data_json: DecodedValueSchema,
  raw_topics: z.array(z.string()),
  raw_data: z.string(),
  emitted_at: z.iso.datetime({ offset: true }),
});

/**
 * A decoded event's identifier as the SDK accepts it.
 *
 * A string of digits, because the indexer's `events` table uses `BIGSERIAL`
 * and ids past 2^53 cannot survive a JSON number; see ADR-021. Validated as
 * digits rather than any string so an obviously wrong value, such as a
 * transaction hash or an empty string, is caught before a request goes out.
 * The SDK still treats the value as opaque and never parses it into a number.
 */
export const EventIdSchema = z
  .string()
  .regex(/^\d+$/, { error: 'Event ID must be a string of digits' });

/** The payload `GET /contracts/:id/events` returns. */
export const EventListPayloadSchema = z.object({
  items: z.array(DecodedEventPayloadSchema),
});

/** Converts a wire event into the camelCase shape callers see. */
export function toDecodedEvent(payload: z.infer<typeof DecodedEventPayloadSchema>): DecodedEvent {
  return {
    id: payload.id,
    contractId: payload.contract_id,
    ledger: payload.ledger,
    txHash: payload.tx_hash,
    eventIndex: payload.event_index,
    name: payload.name,
    topics: payload.topics_json,
    data: payload.data_json,
    rawTopics: payload.raw_topics,
    rawData: payload.raw_data,
    emittedAt: payload.emitted_at,
  };
}

/**
 * The only schemes the SDK will send a request to.
 *
 * `z.url()` alone accepts anything the URL parser accepts, which is broader
 * than it looks: `localhost:8080` parses as a URL whose protocol is
 * `localhost:`, and `ftp://` and `javascript:` parse cleanly too. Dropping the
 * scheme off an indexer URL is a common mistake, and without this constraint it
 * would build a client that validates and then fails at its first request.
 */
const HTTP_PROTOCOL = /^https?$/;

/**
 * The envelope every indexer JSON response arrives in, per ADR-017.
 *
 * `data` carries the route's payload, `next_cursor` appears only on paginated
 * responses, and `meta.took_ms` is the indexer's own measurement of how long it
 * spent, which is not the round-trip latency the SDK measures.
 *
 * `data` is left unknown here and validated separately against the schema for
 * the route. Splitting the two keeps this schema non-generic, and it separates
 * "this is not an indexer response" from "this route returned the wrong shape",
 * which are different failures worth different messages.
 */
export const EnvelopeSchema = z.object({
  data: z.unknown(),
  // Null and absent both mean the same thing, per ADR-021: there is no next
  // page. An empty string is not a valid cursor and is rejected.
  next_cursor: z.string().min(1).nullish(),
  meta: z.object({ took_ms: z.number().nonnegative() }).optional(),
});

/** A response envelope with its payload still unvalidated. */
export type Envelope = z.infer<typeof EnvelopeSchema>;

/** The error envelope the indexer returns for a failed request, per ADR-017. */
export const ErrorEnvelopeSchema = z.object({
  error: z.object({
    code: z.enum(['not_found', 'validation', 'internal', 'rate_limited']),
    message: z.string(),
  }),
});

/** A failure reported by the indexer in its error envelope. */
export type ErrorEnvelope = z.infer<typeof ErrorEnvelopeSchema>;

/**
 * The indexer's health payload, as it appears on the wire.
 *
 * snake_case here and camelCase in {@link PingResult}: the boundary between the
 * two is this schema, so neither the Go side nor a TypeScript caller has to
 * work in the other's conventions.
 */
export const HealthPayloadSchema = z.object({
  ok: z.boolean(),
  version: z.string().min(1),
  latest_ledger: z.number().int().nonnegative().nullable().optional(),
  tracked_contracts: z.number().int().nonnegative().nullable().optional(),
});

/** What `PulsarClient.ping` returns. */
export interface PingResult {
  /** Whether the indexer reports itself healthy. */
  readonly ok: boolean;
  /** The indexer's version string. */
  readonly version: string;
  /** Highest ledger the indexer has processed, or null if it has none yet. */
  readonly latestLedger: number | null;
  /** How many contracts it is following, or null if it did not say. */
  readonly trackedContracts: number | null;
  /** Round-trip time measured by the SDK, in milliseconds. */
  readonly latencyMs: number;
  /** The indexer's own timing, in milliseconds, or null if it did not report one. */
  readonly serverTookMs: number | null;
}

/** The default request timeout, in milliseconds. */
export const DEFAULT_TIMEOUT_MS = 15_000;

/**
 * Configuration for a `PulsarClient`.
 *
 * `rpcUrl` is needed only for reads that bypass the indexer and go straight to
 * Soroban RPC. A client that only queries the indexer does not need one.
 *
 * `fetchImpl` exists for runtimes that do not expose a global `fetch`, and for
 * tests that want to supply their own. It is validated as a function and no
 * further: its signature is checked by the type system, not at runtime.
 */
export const PulsarConfigSchema = z.strictObject({
  indexerUrl: z.url({ protocol: HTTP_PROTOCOL, error: 'indexerUrl must be an http or https URL' }),
  rpcUrl: z
    .url({ protocol: HTTP_PROTOCOL, error: 'rpcUrl must be an http or https URL' })
    .optional(),
  network: PulsarNetworkSchema.optional(),
  fetchImpl: z.custom<typeof fetch>((value) => typeof value === 'function', {
    error: 'fetchImpl must be a function',
  }).optional(),
  timeoutMs: z.number().int().positive().default(DEFAULT_TIMEOUT_MS),
});

/** Configuration as a caller supplies it. */
export type PulsarConfig = z.input<typeof PulsarConfigSchema>;

/** Configuration after parsing, with defaults filled in. */
export type ResolvedPulsarConfig = z.output<typeof PulsarConfigSchema>;
