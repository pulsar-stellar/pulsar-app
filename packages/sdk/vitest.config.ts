import { defineConfig } from 'vitest/config';

/**
 * Unit tests only. HTTP is mocked at the boundary with msw, so no test in this
 * config reaches the network.
 *
 * Integration tests live in `*.integration.test.ts` and are excluded here. They
 * run against live RPC and are opted into explicitly, never in the default run.
 */
export default defineConfig({
  test: {
    environment: 'node',
    include: ['tests/**/*.test.ts'],
    exclude: ['tests/**/*.integration.test.ts'],
    typecheck: {
      include: ['tests/**/*.test-d.ts'],
      tsconfig: './tsconfig.json',
    },
    coverage: {
      provider: 'v8',
      include: ['src/**/*.ts'],
      thresholds: {
        lines: 80,
        functions: 80,
        branches: 80,
        statements: 80,
      },
    },
  },
});
