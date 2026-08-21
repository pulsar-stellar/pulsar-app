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
 * A single decoded contract value.
 *
 * Mirrors the `ScVal` variants the decoder produces. Integers wider than 53
 * bits are carried as strings so that no precision is lost passing through
 * JSON, and `bytes` is hex rather than base64 so a value is readable next to
 * the raw XDR it came from.
 */
export type DecodedValue =
  | { type: 'address'; value: string }
  | { type: 'symbol'; value: string }
  | { type: 'i128'; value: string }
  | { type: 'u128'; value: string }
  | { type: 'bytes'; value: string }
  | { type: 'string'; value: string }
  | { type: 'bool'; value: boolean }
  | { type: 'vec'; value: DecodedValue[] }
  | { type: 'map'; value: Record<string, DecodedValue> }
  | { type: 'tuple'; value: DecodedValue[] }
  | { type: 'void' };

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
    z.object({ type: z.literal('i128'), value: z.string() }),
    z.object({ type: z.literal('u128'), value: z.string() }),
    z.object({ type: z.literal('bytes'), value: z.string() }),
    z.object({ type: z.literal('string'), value: z.string() }),
    z.object({ type: z.literal('bool'), value: z.boolean() }),
    z.object({ type: z.literal('vec'), value: z.array(DecodedValueSchema) }),
    z.object({ type: z.literal('map'), value: z.record(z.string(), DecodedValueSchema) }),
    z.object({ type: z.literal('tuple'), value: z.array(DecodedValueSchema) }),
    z.object({ type: z.literal('void') }),
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
  /** Position of the event within its transaction. */
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
 * A query against the indexer's event history.
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
  contractId: ContractIdSchema,
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

/** One page of events, with the cursor that fetches the next one. */
export const EventPageSchema = z.object({
  items: z.array(DecodedEventSchema),
  /** Absent when the page is the last one. */
  nextCursor: z.string().min(1).optional(),
});

/** A page of decoded events. */
export type EventPage = z.infer<typeof EventPageSchema>;

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
  next_cursor: z.string().min(1).optional(),
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
