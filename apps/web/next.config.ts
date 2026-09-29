import { fileURLToPath } from 'node:url';

import type { NextConfig } from 'next';

/**
 * Next.js configuration for the Pulsar explorer.
 *
 * Deliberately minimal. The explorer reads its indexer URL at request time from
 * a NEXT_PUBLIC_ variable (wired in a later unit), so there is nothing to inline
 * or rewrite here. `reactStrictMode` surfaces effect and lifecycle bugs in dev.
 */
const nextConfig: NextConfig = {
  reactStrictMode: true,

  // This app lives in a pnpm monorepo, and unrelated lockfiles elsewhere on the
  // machine make Next guess the wrong workspace root for output file tracing.
  // Pin it to the repo root (two levels up from apps/web) so tracing is stable.
  outputFileTracingRoot: fileURLToPath(new URL('../..', import.meta.url)),

  // Linting is owned by the repo's shared flat config, run through `pnpm lint`
  // (eslint.config.mjs), not by eslint-config-next. Skipping the build-time lint
  // pass keeps a single lint authority and avoids a redundant, differently
  // configured run during `next build`.
  eslint: {
    ignoreDuringBuilds: true,
  },
};

export default nextConfig;
