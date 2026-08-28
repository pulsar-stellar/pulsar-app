/**
 * Type-level assertions for the direct-RPC path.
 *
 * ADR-025 enforces the query constraint at two layers. `rpc.test.ts` covers
 * the runtime layer; this file covers the one a runtime test cannot reach.
 * The `@ts-expect-error` assertions fail the typecheck run if a shape that
 * should be rejected ever starts compiling.
 *
 * The mode-switch checks use `assertType` rather than `expectTypeOf`. A union
 * whose members carry `never` fields does not survive `toEqualTypeOf` under
 * `exactOptionalPropertyTypes`, and the claim being made is about assignment
 * anyway: the reassignment on the line above has to compile.
 */

import { assertType, describe, expectTypeOf, it } from 'vitest';

import {
  fetchLiveEvents,
  liveEventStream,
  type LiveEventQuery,
  type LiveEventsPage,
} from '../src/rpc.js';
import type { DecodedEvent, ResolvedPulsarConfig } from '../src/types.js';

declare const config: ResolvedPulsarConfig;

describe('LiveEventQuery rejects the shapes the protocol rejects', () => {
  it('accepts either mode on its own', () => {
    assertType<LiveEventQuery>({ startLedger: 4_378_751 });
    assertType<LiveEventQuery>({ cursor: '0018806592342327296-0000000000' });
    assertType<LiveEventQuery>({ startLedger: 4_378_751, limit: 25 });
    assertType<LiveEventQuery>({ cursor: 'c', filter: { type: 'system' } });
  });

  it('rejects both modes at once', () => {
    // @ts-expect-error startLedger and cursor are mutually exclusive
    const query: LiveEventQuery = { startLedger: 4_378_751, cursor: 'c' };
    void query;
  });

  it('rejects neither mode', () => {
    // @ts-expect-error one of startLedger or cursor is required
    const query: LiveEventQuery = { limit: 25 };
    void query;
  });

  it('rejects a spread continuation, which would carry startLedger forward', () => {
    const first: LiveEventQuery = { startLedger: 4_378_751 };
    // @ts-expect-error the spread keeps startLedger, so both fields are set
    const next: LiveEventQuery = { ...first, cursor: 'c' };
    void next;
  });
});

describe('the pagination mode switch a consumer writes', () => {
  it('reassigns from ledger mode to cursor mode without a cast', async () => {
    let query: LiveEventQuery = { startLedger: 4_378_751 };
    const page = await fetchLiveEvents(config, query);
    query = { cursor: page.cursor };
    assertType<LiveEventQuery>(query);
  });

  it('carries a limit across the mode switch', async () => {
    let query: LiveEventQuery = { startLedger: 4_378_751, limit: 25 };
    const page = await fetchLiveEvents(config, query);
    query = { cursor: page.cursor, limit: 25 };
    assertType<LiveEventQuery>(query);
  });
});

describe('return types', () => {
  it('resolves fetchLiveEvents to a page that is never null', () => {
    expectTypeOf(fetchLiveEvents).returns.resolves.toEqualTypeOf<LiveEventsPage>();
    expectTypeOf<LiveEventsPage>().not.toBeNullable();
  });

  it('makes the page cursor a plain string, since RPC always sends one', () => {
    expectTypeOf<LiveEventsPage['cursor']>().toEqualTypeOf<string>();
    expectTypeOf<LiveEventsPage['latestLedger']>().toEqualTypeOf<number>();
  });

  it('returns an async iterable of decoded events from liveEventStream', () => {
    expectTypeOf(liveEventStream).returns.toEqualTypeOf<AsyncIterable<DecodedEvent>>();
  });
});

describe('the DecodedEvent fields ADR-026 added', () => {
  it('requires the success flag rather than leaving it optional', () => {
    expectTypeOf<DecodedEvent>().toHaveProperty('inSuccessfulContractCall');
    expectTypeOf<DecodedEvent['inSuccessfulContractCall']>().toEqualTypeOf<boolean>();
    expectTypeOf<DecodedEvent['inSuccessfulContractCall']>().not.toBeNullable();
  });

  it('keeps name a plain string, so an off-convention event can be nameless', () => {
    expectTypeOf<DecodedEvent['name']>().toEqualTypeOf<string>();
  });
});
