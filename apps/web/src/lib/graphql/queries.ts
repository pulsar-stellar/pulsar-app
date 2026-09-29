import 'server-only';

import {
  ContractIdSchema,
  EventIdSchema,
} from '@pulsar-stellar/sdk';
import { z } from 'zod';

import { executeGraphQL, type GraphQLRequestInput } from './client';
import { GraphQLRequestError } from './errors';
import {
  contractNodeSchema,
  eventConnectionSchema,
  eventNodeSchema,
  healthSchema,
  type ContractNode,
  type EventConnection,
  type EventNode,
  type Health,
} from './schemas';

/**
 * Typed wrappers over the indexer's GraphQL surface (ADR-043).
 *
 * Each wrapper owns one operation: it holds the query document, validates the
 * caller's identifiers before a request leaves the process, and returns the
 * single field the document selects, already parsed by {@link executeGraphQL}
 * against its boundary schema.
 *
 * Identifier validation reuses the SDK's `ContractIdSchema` and `EventIdSchema`
 * so a malformed id, the direct-user-input path, is rejected here with the same
 * catalog code the indexer would return, before any network round-trip. Filter
 * arguments (`name`, ledger bounds, `limit`, `order`, ...) are forwarded as
 * given: the indexer is the single authority on those and returns its own
 * `VALIDATION_*` codes, which the client surfaces unchanged (ADR-046).
 */

/** Per-call transport controls, forwarded to {@link executeGraphQL}. */
export interface QueryOptions {
  signal?: AbortSignal;
  /** Injectable for tests; defaults to the global fetch. */
  fetchImpl?: typeof fetch;
}

/** The selection set shared by every Event the explorer reads. */
const EVENT_FIELDS = `
  id
  contractId
  ledger
  txHash
  eventIndex
  name
  topics
  data
  rawTopics
  rawData
  emittedAt
  inSuccessfulContractCall
`;

/** The scalar Contract fields, without the optional nested event connection. */
const CONTRACT_FIELDS = `
  id
  addedAt
  firstIndexedLedger
  lastIndexedLedger
  status
`;

/** The filter arguments an events connection accepts, as GraphQL variables. */
const EVENT_ARG_DECLS =
  '$name: String, $fromLedger: Int, $toLedger: Int, ' +
  '$topicContains: String, $limit: Int, $cursor: String, $order: String';

/** The same filter arguments, threaded into an `events(...)` call. */
const EVENT_ARG_USES =
  'name: $name, fromLedger: $fromLedger, toLedger: $toLedger, ' +
  'topicContains: $topicContains, limit: $limit, cursor: $cursor, order: $order';

const HEALTH_QUERY = `
  query Health {
    health { ok version latestLedger trackedContracts }
  }
`;

const CONTRACT_QUERY = `
  query Contract($id: ID!) {
    contract(id: $id) { ${CONTRACT_FIELDS} }
  }
`;

const EVENT_QUERY = `
  query Event($id: ID!) {
    event(id: $id) { ${EVENT_FIELDS} }
  }
`;

const EVENTS_QUERY = `
  query Events($contractId: ID!, ${EVENT_ARG_DECLS}) {
    events(contractId: $contractId, ${EVENT_ARG_USES}) {
      items { ${EVENT_FIELDS} }
      nextCursor
    }
  }
`;

const CONTRACT_WITH_EVENTS_QUERY = `
  query ContractWithEvents($id: ID!, ${EVENT_ARG_DECLS}) {
    contract(id: $id) {
      ${CONTRACT_FIELDS}
      events(${EVENT_ARG_USES}) {
        items { ${EVENT_FIELDS} }
        nextCursor
      }
    }
  }
`;

/**
 * Filters for an events query, mirroring the SDK's `EventQuery` shape.
 *
 * The optional fields admit an explicit `undefined` (they are forwarded as
 * GraphQL variables, where an absent and a null variable are equivalent), and
 * `order` is a plain string: the indexer validates the enum and the ledger
 * bounds, returning its own `VALIDATION_*` codes (ADR-046).
 */
export interface EventQueryArgs {
  contractId: string;
  name?: string | undefined;
  fromLedger?: number | undefined;
  toLedger?: number | undefined;
  topicContains?: string | undefined;
  limit?: number | undefined;
  cursor?: string | undefined;
  order?: string | undefined;
}

/** The nested-events filters, without the contract the query is scoped to. */
export type NestedEventArgs = Omit<EventQueryArgs, 'contractId'>;

/**
 * Reject a malformed identifier before a request goes out.
 *
 * The message stays generic (client-safe) and carries the catalog code the
 * indexer uses for the same failure, so a caller branches on `code` the same
 * way whether the rejection came from here or from the wire.
 */
function assertValid(
  schema: z.ZodType<string>,
  value: string,
  code: 'VALIDATION_CONTRACT_ID' | 'VALIDATION_EVENT_ID',
  message: string,
): void {
  if (!schema.safeParse(value).success) {
    throw new GraphQLRequestError(message, { category: 'validation', code });
  }
}

function assertContractId(id: string): void {
  assertValid(ContractIdSchema, id, 'VALIDATION_CONTRACT_ID', 'Invalid contract ID');
}

function assertEventId(id: string): void {
  assertValid(EventIdSchema, id, 'VALIDATION_EVENT_ID', 'Invalid event ID');
}

/** Build the filter variables shared by the flat and nested events queries. */
function eventVariables(args: NestedEventArgs): Record<string, unknown> {
  return {
    name: args.name,
    fromLedger: args.fromLedger,
    toLedger: args.toLedger,
    topicContains: args.topicContains,
    limit: args.limit,
    cursor: args.cursor,
    order: args.order,
  };
}

function run<T extends z.ZodType>(
  dataSchema: T,
  request: GraphQLRequestInput,
  options: QueryOptions,
): Promise<z.infer<T>> {
  return executeGraphQL(dataSchema, request, options);
}

/** Fetch the indexer's health payload. */
export async function getHealth(options: QueryOptions = {}): Promise<Health> {
  const data = await run(
    z.object({ health: healthSchema }),
    { query: HEALTH_QUERY, operationName: 'Health' },
    options,
  );
  return data.health;
}

/** Fetch one contract by id, or `null` when it is not tracked. */
export async function getContract(
  id: string,
  options: QueryOptions = {},
): Promise<ContractNode | null> {
  assertContractId(id);
  const data = await run(
    z.object({ contract: contractNodeSchema.nullable() }),
    { query: CONTRACT_QUERY, variables: { id }, operationName: 'Contract' },
    options,
  );
  return data.contract;
}

/** Fetch one event by id, or `null` when it does not exist. */
export async function getEvent(
  id: string,
  options: QueryOptions = {},
): Promise<EventNode | null> {
  assertEventId(id);
  const data = await run(
    z.object({ event: eventNodeSchema.nullable() }),
    { query: EVENT_QUERY, variables: { id }, operationName: 'Event' },
    options,
  );
  return data.event;
}

/** Fetch a page of one contract's events, applying the given filters. */
export async function getEvents(
  args: EventQueryArgs,
  options: QueryOptions = {},
): Promise<EventConnection> {
  assertContractId(args.contractId);
  const data = await run(
    z.object({ events: eventConnectionSchema }),
    {
      query: EVENTS_QUERY,
      variables: { contractId: args.contractId, ...eventVariables(args) },
      operationName: 'Events',
    },
    options,
  );
  return data.events;
}

/**
 * Fetch a contract together with its first page of events in one round-trip.
 *
 * This is the "richer traversal" the GraphQL surface exists for: the REST
 * surface cannot nest the connection under the contract (ADR-043).
 */
export async function getContractWithEvents(
  id: string,
  args: NestedEventArgs = {},
  options: QueryOptions = {},
): Promise<ContractNode | null> {
  assertContractId(id);
  const data = await run(
    z.object({ contract: contractNodeSchema.nullable() }),
    {
      query: CONTRACT_WITH_EVENTS_QUERY,
      variables: { id, ...eventVariables(args) },
      operationName: 'ContractWithEvents',
    },
    options,
  );
  return data.contract;
}
