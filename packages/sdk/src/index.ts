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
  request,
  requestMaybe,
  type HttpMethod,
  type RequestOptions,
  type RequestResult,
} from './http.js';

export {
  ContractIdSchema,
  ContractInfoPayloadSchema,
  ContractInfoSchema,
  ContractListPayloadSchema,
  ContractStatusSchema,
  DecodedEventSchema,
  DecodedValueSchema,
  DEFAULT_TIMEOUT_MS,
  EVENT_QUERY_DEFAULT_LIMIT,
  EVENT_QUERY_MAX_LIMIT,
  EnvelopeSchema,
  ErrorEnvelopeSchema,
  EventPageSchema,
  EventQuerySchema,
  HealthPayloadSchema,
  PulsarConfigSchema,
  PulsarNetworkSchema,
  toContractInfo,
  type ContractInfo,
  type ContractStatus,
  type DecodedEvent,
  type DecodedValue,
  type EventPage,
  type Envelope,
  type ErrorEnvelope,
  type EventQuery,
  type PingResult,
  type PulsarConfig,
  type PulsarNetwork,
  type ResolvedEventQuery,
  type ResolvedPulsarConfig,
} from './types.js';
