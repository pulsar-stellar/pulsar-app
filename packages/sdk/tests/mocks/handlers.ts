/**
 * Default request handlers, one group per indexer endpoint.
 *
 * These describe the healthy case. A test that needs a failure overrides the
 * specific route with `server.use(...)`, which is reset between tests.
 *
 * Payload shapes follow ADR-017: snake_case on the wire, wrapped in the
 * response envelope.
 */

import { http, HttpResponse } from 'msw';

/** Base URL every handler here is registered against. */
const BASE = 'http://indexer.test';

/** The healthy `/health` response the SDK is written against. */
export const healthyHealthPayload = {
  ok: true,
  version: '0.1.0',
  latest_ledger: 1_234_567,
  tracked_contracts: 3,
} as const;

export const healthHandlers = [
  http.get(`${BASE}/health`, () =>
    HttpResponse.json({ data: healthyHealthPayload, meta: { took_ms: 4 } }),
  ),
];

/** A tracked contract as the indexer serves it, snake_case per ADR-017. */
export const trackedContractPayload = {
  id: 'CDNWTVUDKCCGW7GOC6SBLUFXXUCD2YDHWRDUSXZ6CYBQKQWLCUYYWI5L',
  added_at: '2026-08-21T09:00:00Z',
  first_indexed_ledger: 1_000_000,
  last_indexed_ledger: 1_234_567,
  status: 'active',
} as const;

export const contractHandlers = [
  http.post(`${BASE}/contracts`, () =>
    HttpResponse.json({ data: trackedContractPayload, meta: { took_ms: 7 } }),
  ),
];

export const contractReadHandlers = [
  http.get(`${BASE}/contracts`, () =>
    HttpResponse.json({ data: { items: [trackedContractPayload] }, meta: { took_ms: 3 } }),
  ),
  http.get(`${BASE}/contracts/:id`, () => HttpResponse.json({ data: trackedContractPayload })),
];

/** A decoded event as the indexer serves it, snake_case per ADR-017. */
export const eventPayload = {
  id: '9007199254740993',
  contract_id: trackedContractPayload.id,
  ledger: 1_234_567,
  tx_hash: 'ab'.repeat(32),
  event_index: 0,
  name: 'deposit',
  topics_json: [
    { type: 'symbol', value: 'deposit' },
    { type: 'address', value: trackedContractPayload.id },
  ],
  data_json: { type: 'i128', value: '1000000000' },
  raw_topics: ['AAAADwAAAAdkZXBvc2l0AA=='],
  raw_data: 'AAAACgAAAAAAAAAAAAAAADuaygA=',
  emitted_at: '2026-08-21T09:15:00Z',
} as const;

export const eventHandlers = [
  http.get(`${BASE}/contracts/:id/events`, () =>
    HttpResponse.json({ data: { items: [eventPayload] }, next_cursor: null }),
  ),
];

export const singleEventHandlers = [
  http.get(`${BASE}/events/:eventId`, () => HttpResponse.json({ data: eventPayload })),
];

export const handlers = [
  ...healthHandlers,
  ...contractHandlers,
  ...contractReadHandlers,
  ...eventHandlers,
  ...singleEventHandlers,
];
