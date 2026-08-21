/**
 * The msw server every network test shares.
 *
 * `onUnhandledRequest: 'error'` is deliberate: a request no handler matched is
 * a test reaching the real network, which would be slow, flaky, and dependent
 * on somebody else's uptime. Failing loudly is the point.
 */

import { setupServer } from 'msw/node';

import { handlers } from './handlers.js';

export const server = setupServer(...handlers);

/** Base URL every mocked handler is registered against. */
export const INDEXER_URL = 'http://indexer.test';
