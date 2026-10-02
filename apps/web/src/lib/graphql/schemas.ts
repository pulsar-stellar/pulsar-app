import 'server-only';

import {
  ContractIdSchema,
  ContractStatusSchema,
  DecodedEventSchema,
} from '@pulsar-stellar/sdk';
import { z } from 'zod';

/**
 * Zod boundary schemas for the indexer's GraphQL responses.
 *
 * The explorer trusts nothing that crosses the wire: every response is parsed
 * before it is used. Where the indexer's GraphQL shape matches a type the SDK
 * already validates, this module reuses the SDK schema rather than restating
 * it, so the two cannot drift (ADR-046). The explorer does not re-decode XDR;
 * the indexer has already produced the decoded taxonomy, and these schemas
 * validate that taxonomy on arrival.
 *
 * The posture is lenient, matching the SDK: unknown keys are stripped (plain
 * `z.object` default) so an indexer that adds a field does not break a build
 * made before it existed.
 */

/**
 * A GraphQL Event node.
 *
 * ADR-043's Event maps onto the SDK's `DecodedEventSchema` field-for-field:
 * a decimal-string id, camelCase fields, `topics` as the decoded value array
 * and `data` as a single decoded value, RFC3339 `emittedAt`. Reused directly.
 */
export const eventNodeSchema = DecodedEventSchema;

/** A page of events: the items plus an opaque cursor to the next page. */
export const eventConnectionSchema = z.object({
  items: z.array(eventNodeSchema),
  nextCursor: z.string().nullable(),
});

/**
 * A GraphQL Contract node.
 *
 * The contract's `id` is its Soroban address (validated with the SDK's
 * `ContractIdSchema`) and `status` reuses the SDK enum. `events` is present
 * only when a query selects the nested connection.
 */
export const contractNodeSchema = z.object({
  id: ContractIdSchema,
  addedAt: z.iso.datetime({ offset: true }),
  firstIndexedLedger: z.number().int().nonnegative().nullable(),
  lastIndexedLedger: z.number().int().nonnegative(),
  status: ContractStatusSchema,
  events: eventConnectionSchema.optional(),
});

/** The indexer health payload. */
export const healthSchema = z.object({
  ok: z.boolean(),
  version: z.string(),
  latestLedger: z.number().int().nonnegative(),
  trackedContracts: z.number().int().nonnegative(),
});

/**
 * A single GraphQL error object.
 *
 * The indexer carries its catalog code and wire class in `extensions`
 * (ADR-043); both are optional so a spec-shaped error from any layer parses.
 */
export const graphqlErrorSchema = z.object({
  message: z.string(),
  path: z.array(z.union([z.string(), z.number()])).optional(),
  extensions: z
    .object({
      code: z.string().optional(),
      class: z.string().optional(),
    })
    .optional(),
});

/**
 * Wrap a data-payload schema in the GraphQL-spec `{ data, errors }` envelope.
 *
 * `data` is nullish because a request that errors carries `data: null` (or
 * omits it); `errors` is present only when something went wrong.
 */
export function graphqlResponseSchema<T extends z.ZodType>(dataSchema: T) {
  return z.object({
    data: dataSchema.nullish(),
    errors: z.array(graphqlErrorSchema).optional(),
  });
}

export type EventNode = z.infer<typeof eventNodeSchema>;
export type EventConnection = z.infer<typeof eventConnectionSchema>;
export type ContractNode = z.infer<typeof contractNodeSchema>;
export type Health = z.infer<typeof healthSchema>;
export type GraphQLErrorObject = z.infer<typeof graphqlErrorSchema>;

/**
 * The decoded-value taxonomy (ADR-023), re-exported from the SDK so the UI has a
 * single boundary module for its types. Type-only, so it adds no runtime import
 * and never pulls the SDK into a client bundle (ADR-046).
 */
export type { DecodedValue, DecodedMapEntry } from '@pulsar-stellar/sdk';
