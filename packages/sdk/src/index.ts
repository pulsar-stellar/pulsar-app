/**
 * Public surface of `@pulsar-stellar/sdk`.
 *
 * Everything a consumer is meant to use is exported here. Anything not
 * exported from this file is internal and may change without a major version,
 * which is the point of listing the surface in one place rather than letting
 * it accumulate.
 *
 * Three things are deliberately absent.
 *
 * The HTTP layer, `request` and `requestMaybe`, is how this SDK talks to the
 * indexer. A consumer calls a client method instead; exporting the transport
 * would freeze an internal contract and invite calls that bypass validation.
 *
 * The wire payload schemas and their mappers, `DecodedEventPayloadSchema`,
 * `toDecodedEvent` and their siblings, describe the snake_case shapes moving
 * between this SDK and the indexer. They are an implementation detail of that
 * conversation. Consumers get the camelCase types the mappers produce.
 *
 * The response envelope, `EnvelopeSchema` and `ErrorEnvelopeSchema`, is the
 * same: ADR-017 fixes it as the indexer's contract, and this SDK unwraps it so
 * nobody else has to know it exists.
 *
 * @packageDocumentation
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

export { decodeScVal, decodeTopics, eventNameFromTopics } from './decode.js';

export {
  ContractIdSchema,
  ContractInfoSchema,
  ContractStatusSchema,
  DecodedEventSchema,
  DecodedMapEntrySchema,
  DecodedValueSchema,
  DEFAULT_TIMEOUT_MS,
  EVENT_QUERY_DEFAULT_LIMIT,
  EVENT_QUERY_MAX_LIMIT,
  EventIdSchema,
  EventQuerySchema,
  PulsarConfigSchema,
  PulsarNetworkSchema,
  type ContractInfo,
  type ContractStatus,
  type DecodedEvent,
  type DecodedMapEntry,
  type DecodedValue,
  type EventQuery,
  type EventsPage,
  type PingResult,
  type PulsarConfig,
  type PulsarNetwork,
  type ResolvedEventQuery,
  type ResolvedPulsarConfig,
} from './types.js';

export {
  DEFAULT_POLL_INTERVAL_MS,
  fetchLiveEvents,
  liveEventStream,
  LiveEventFilterSchema,
  LiveEventQuerySchema,
  RPC_ID_PREFIX,
  type LiveEventFilter,
  type LiveEventQuery,
  type LiveEventsPage,
  type LiveEventStreamOptions,
} from './rpc.js';

export {
  asAdminChangeEvent,
  asDepositEvent,
  asEmitCustomEvent,
  asInitializeEvent,
  asTransferEvent,
  asWithdrawEvent,
  buildContractCall,
  DEFAULT_CALL_TIMEOUT_SECONDS,
  parseTopics,
  scValToNative,
  type AdminChangeEvent,
  type BindingEvent,
  type ContractCallOptions,
  type DepositEvent,
  type EmitCustomEvent,
  type InitializeEvent,
  type TransferEvent,
  type WithdrawEvent,
} from './contract.js';
