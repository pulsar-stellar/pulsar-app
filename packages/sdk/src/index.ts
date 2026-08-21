/**
 * Public surface of `@pulsar-stellar/sdk`.
 *
 * Exports are added here as each piece lands. Anything not exported from this
 * file is internal and may change without a major version.
 */

export { PulsarClient } from './client.js';

export {
  findPulsarError,
  PulsarError,
  PulsarNetworkError,
  PulsarValidationError,
  type PulsarErrorOptions,
  type PulsarNetworkErrorOptions,
} from './errors.js';

export {
  ContractIdSchema,
  ContractInfoSchema,
  ContractStatusSchema,
  DecodedEventSchema,
  DecodedValueSchema,
  DEFAULT_TIMEOUT_MS,
  EVENT_QUERY_DEFAULT_LIMIT,
  EVENT_QUERY_MAX_LIMIT,
  EventPageSchema,
  EventQuerySchema,
  PulsarConfigSchema,
  PulsarNetworkSchema,
  type ContractInfo,
  type ContractStatus,
  type DecodedEvent,
  type DecodedValue,
  type EventPage,
  type EventQuery,
  type PulsarConfig,
  type PulsarNetwork,
  type ResolvedEventQuery,
  type ResolvedPulsarConfig,
} from './types.js';
