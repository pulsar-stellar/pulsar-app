/**
 * Server lifecycle for every network test.
 *
 * Listening starts once per file, handlers reset between tests so an override
 * cannot leak, and the server closes at the end so a hanging interceptor
 * cannot keep the process alive.
 */

import { afterAll, afterEach, beforeAll } from 'vitest';

import { server } from './mocks/server.js';

beforeAll(() => {
  server.listen({ onUnhandledRequest: 'error' });
});

afterEach(() => {
  server.resetHandlers();
});

afterAll(() => {
  server.close();
});
