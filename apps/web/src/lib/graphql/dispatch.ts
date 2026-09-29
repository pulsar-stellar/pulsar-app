import 'server-only';

import { z } from 'zod';

import { GraphQLRequestError } from './errors';
import {
  getContract,
  getContractWithEvents,
  getEvent,
  getEvents,
  getHealth,
  type QueryOptions,
} from './queries';

/**
 * The browser-facing dispatch boundary for interactive refetches (ADR-046).
 *
 * The explorer's Hybrid data layer loads initial data server-side, but a screen
 * that paginates or refilters calls this instead of a searchParams round-trip.
 * This is deliberately NOT an arbitrary-query proxy: the browser names one of a
 * fixed allowlist of operations and supplies variables, and dispatch maps that
 * to a typed wrapper whose query document is a server-side constant. An input
 * the allowlist does not name never reaches the indexer.
 *
 * Each operation validates its variables against the GraphQL transport types
 * (Int to number, String to string) before delegating. Business rules, the
 * ledger bounds, the limit ceiling, the order enum, stay with the indexer, the
 * single authority, which returns its own `VALIDATION_*` codes. Identifier
 * format is checked in the wrappers, so a malformed id is rejected here with
 * the indexer's own catalog code and no network round-trip.
 */

/** The request the browser posts: a named operation and its variables. */
const requestSchema = z.object({
  operation: z.string(),
  variables: z.unknown().optional(),
});

/** Transport-type shape shared by the events filters (business rules stay server-side). */
const eventFilterShape = {
  name: z.string().optional(),
  fromLedger: z.number().int().optional(),
  toLedger: z.number().int().optional(),
  topicContains: z.string().optional(),
  limit: z.number().int().optional(),
  cursor: z.string().optional(),
  order: z.string().optional(),
} as const;

const idVars = z.object({ id: z.string() }).strict();
const eventsVars = z.object({ contractId: z.string(), ...eventFilterShape }).strict();
const contractWithEventsVars = z.object({ id: z.string(), ...eventFilterShape }).strict();

function validationError(message: string): GraphQLRequestError {
  return new GraphQLRequestError(message, { category: 'validation', code: null });
}

function parseVars<T extends z.ZodType>(schema: T, variables: unknown): z.infer<T> {
  const parsed = schema.safeParse(variables ?? {});
  if (!parsed.success) throw validationError('Invalid variables for the requested operation');
  return parsed.data;
}

/**
 * Execute one allowlisted operation and return its data.
 *
 * @throws GraphQLRequestError with category `validation` for a malformed body,
 * an operation outside the allowlist, or variables of the wrong transport type;
 * and whatever the delegated wrapper throws for a downstream failure.
 */
export async function dispatch(
  body: unknown,
  options: QueryOptions = {},
): Promise<unknown> {
  const req = requestSchema.safeParse(body);
  if (!req.success) throw validationError('Malformed request body');

  const { operation, variables } = req.data;
  switch (operation) {
    case 'health':
      return getHealth(options);
    case 'contract':
      return getContract(parseVars(idVars, variables).id, options);
    case 'event':
      return getEvent(parseVars(idVars, variables).id, options);
    case 'events':
      return getEvents(parseVars(eventsVars, variables), options);
    case 'contractWithEvents': {
      const { id, ...filters } = parseVars(contractWithEventsVars, variables);
      return getContractWithEvents(id, filters, options);
    }
    default:
      throw validationError('Unknown or unsupported operation');
  }
}

/** The operations the dispatch boundary accepts, for callers that enumerate them. */
export const DISPATCH_OPERATIONS = [
  'health',
  'contract',
  'event',
  'events',
  'contractWithEvents',
] as const;

export type DispatchOperation = (typeof DISPATCH_OPERATIONS)[number];
