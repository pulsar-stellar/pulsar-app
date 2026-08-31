/**
 * Pins the types the package exports.
 *
 * A type export is invisible at runtime, so `index.test.ts` cannot see one
 * appear or disappear. These assertions fail the typecheck run if a public
 * type is dropped or renamed, and the `@ts-expect-error` cases fail if an
 * internal one becomes public.
 */

import { describe, expectTypeOf, it } from 'vitest';

import type * as sdk from '../src/index.js';
import type {
  AdminChangeEvent,
  BindingEvent,
  ContractCallOptions,
  ContractInfo,
  ContractStatus,
  DecodedEvent,
  DecodedMapEntry,
  DecodedValue,
  DepositEvent,
  EmitCustomEvent,
  EventQuery,
  EventsPage,
  InitializeEvent,
  LiveEventFilter,
  LiveEventQuery,
  LiveEventStreamOptions,
  LiveEventsPage,
  PingResult,
  PulsarConfig,
  PulsarErrorOptions,
  PulsarNetwork,
  PulsarNetworkErrorOptions,
  ResolvedEventQuery,
  ResolvedPulsarConfig,
  TransferEvent,
  WithdrawEvent,
} from '../src/index.js';

/** Whether `Name` is exported as a value from the package entry point. */
type InSurface<Name extends string> = Name extends keyof typeof sdk ? true : false;

describe('the exported types', () => {
  it('carries the event and value types a consumer reads', () => {
    expectTypeOf<DecodedEvent>().not.toBeNever();
    expectTypeOf<DecodedValue>().not.toBeNever();
    expectTypeOf<DecodedMapEntry>().not.toBeNever();
    expectTypeOf<EventsPage>().not.toBeNever();
    expectTypeOf<EventQuery>().not.toBeNever();
    expectTypeOf<ResolvedEventQuery>().not.toBeNever();
  });

  it('carries the configuration and client result types', () => {
    expectTypeOf<PulsarConfig>().not.toBeNever();
    expectTypeOf<ResolvedPulsarConfig>().not.toBeNever();
    expectTypeOf<PulsarNetwork>().not.toBeNever();
    expectTypeOf<PingResult>().not.toBeNever();
    expectTypeOf<ContractInfo>().not.toBeNever();
    expectTypeOf<ContractStatus>().not.toBeNever();
  });

  it('carries the error option types', () => {
    expectTypeOf<PulsarErrorOptions>().not.toBeNever();
    expectTypeOf<PulsarNetworkErrorOptions>().not.toBeNever();
  });

  it('carries the live-path types', () => {
    expectTypeOf<LiveEventQuery>().not.toBeNever();
    expectTypeOf<LiveEventFilter>().not.toBeNever();
    expectTypeOf<LiveEventsPage>().not.toBeNever();
    expectTypeOf<LiveEventStreamOptions>().not.toBeNever();
  });

  it('carries the contract-call and binding types', () => {
    expectTypeOf<ContractCallOptions>().not.toBeNever();
    expectTypeOf<BindingEvent<'X', { a: 1 }>>().not.toBeNever();
    expectTypeOf<InitializeEvent>().not.toBeNever();
    expectTypeOf<DepositEvent>().not.toBeNever();
    expectTypeOf<WithdrawEvent>().not.toBeNever();
    expectTypeOf<TransferEvent>().not.toBeNever();
    expectTypeOf<AdminChangeEvent>().not.toBeNever();
    expectTypeOf<EmitCustomEvent>().not.toBeNever();
  });
});

describe('the types that stay internal', () => {
  /**
   * Written as a membership test against the module's own key set, and paired
   * with a positive case. Without the positive case a broken assertion would
   * report every name as absent, including the public ones, and pass.
   */
  it('excludes the wire payload schemas from the module surface', () => {
    expectTypeOf<InSurface<'DecodedEventPayloadSchema'>>().toEqualTypeOf<false>();
    expectTypeOf<InSurface<'ContractInfoPayloadSchema'>>().toEqualTypeOf<false>();
    expectTypeOf<InSurface<'EnvelopeSchema'>>().toEqualTypeOf<false>();
  });

  it('excludes the transport and the wire mappers', () => {
    expectTypeOf<InSurface<'request'>>().toEqualTypeOf<false>();
    expectTypeOf<InSurface<'requestMaybe'>>().toEqualTypeOf<false>();
    expectTypeOf<InSurface<'toDecodedEvent'>>().toEqualTypeOf<false>();
    expectTypeOf<InSurface<'toLiveDecodedEvent'>>().toEqualTypeOf<false>();
  });

  it('reports a genuinely public name as present, which is what makes the above mean something', () => {
    expectTypeOf<InSurface<'PulsarClient'>>().toEqualTypeOf<true>();
    expectTypeOf<InSurface<'decodeScVal'>>().toEqualTypeOf<true>();
    expectTypeOf<InSurface<'buildContractCall'>>().toEqualTypeOf<true>();
  });
});
