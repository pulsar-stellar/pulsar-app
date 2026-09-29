import { fileURLToPath } from 'node:url';

import react from '@vitejs/plugin-react';
import { defineConfig } from 'vitest/config';

/**
 * Component and unit tests for the explorer.
 *
 * The React plugin owns the JSX transform here, not the Next compiler: the
 * app's tsconfig sets `jsx: preserve` for Next, which esbuild alone cannot
 * consume, so the plugin transforms test and component sources instead. jsdom
 * gives the DOM that Testing Library renders into.
 *
 * Coverage is scoped to the logic modules. App Router entry points under
 * `src/app` are exercised by component and integration tests, not counted as
 * unit surface, matching CONTRIBUTING's split between behavior and scaffolding.
 */
export default defineConfig({
  plugins: [react()],
  resolve: {
    // Mirror the tsconfig `@/*` path alias; Vite does not read tsconfig paths.
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url)),
      // The data layer marks itself `server-only`; that package throws on import
      // under any condition but Next's `react-server`. Tests run the modules
      // directly in Node, so resolve the marker to its empty server stub, the
      // same file Next serves a Server Component.
      'server-only': fileURLToPath(
        new URL('./node_modules/server-only/empty.js', import.meta.url),
      ),
    },
  },
  test: {
    environment: 'jsdom',
    globals: true,
    setupFiles: ['vitest.setup.ts'],
    include: ['tests/**/*.test.{ts,tsx}'],
    coverage: {
      provider: 'v8',
      include: ['src/**/*.{ts,tsx}'],
      exclude: ['src/app/**'],
      thresholds: {
        lines: 80,
        functions: 80,
        branches: 80,
        statements: 80,
      },
    },
  },
});
