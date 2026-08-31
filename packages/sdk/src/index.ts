/**
 * Public surface of `@pulsar-stellar/sdk`.
 *
 * Exports are added here as each piece lands. Anything not exported from this
 * file is internal and may change without a major version.
 */

export { PulsarClient } from './client.js';

export { decodeScVal, decodeTopics, eventNameFromTopics } from './decode.js';

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
  DecodedMapEntrySchema,
  DecodedValueSchema,
  DEFAULT_TIMEOUT_MS,
  EVENT_QUERY_DEFAULT_LIMIT,
  EVENT_QUERY_MAX_LIMIT,
  EnvelopeSchema,
  ErrorEnvelopeSchema,
  DecodedEventPayloadSchema,
  EventIdSchema,
  EventListPayloadSchema,
  EventQuerySchema,
  HealthPayloadSchema,
  PulsarConfigSchema,
  PulsarNetworkSchema,
  toContractInfo,
  toDecodedEvent,
  type ContractInfo,
  type ContractStatus,
  type DecodedEvent,
  type DecodedMapEntry,
  type DecodedValue,
  type EventsPage,
  type Envelope,
  type ErrorEnvelope,
  type EventQuery,
  type PingResult,
  type PulsarConfig,
  type PulsarNetwork,
  type ResolvedEventQuery,
  type ResolvedPulsarConfig,
} from './types.js';

export {
  DEFAULT_POLL_INTERVAL_MS,
  LiveEventFilterSchema,
  LiveEventQuerySchema,
  RPC_ID_PREFIX,
  fetchLiveEvents,
  liveEventStream,
  toLiveDecodedEvent,
  type LiveEventFilter,
  type LiveEventQuery,
  type LiveEventStreamOptions,
  type LiveEventsPage,
  type RawRpcEvent,
} from './rpc.js';

export {
  DEFAULT_CALL_TIMEOUT_SECONDS,
  asAdminChangeEvent,
  asDepositEvent,
  asEmitCustomEvent,
  asInitializeEvent,
  asTransferEvent,
  asWithdrawEvent,
  buildContractCall,
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

