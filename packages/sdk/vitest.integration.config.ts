import { defineConfig } from 'vitest/config';

/**
 * Integration tests. These reach the network and are never part of the default
 * run, which `vitest.config.ts` keeps offline.
 *
 * Every test here skips rather than fails when the service it needs is
 * unreachable, so an outage does not turn CI red over something outside this
 * repository.
 */
export default defineConfig({
  test: {
    environment: 'node',
    include: ['tests/**/*.integration.test.ts'],
    testTimeout: 240_000,
  },
});
