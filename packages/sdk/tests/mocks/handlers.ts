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

export const handlers = [...healthHandlers];
