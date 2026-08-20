import { defineConfig } from 'tsup';

/**
 * Bundles the SDK to dual ESM and CJS output.
 *
 * Type declarations are not emitted here. `tsc --emitDeclarationOnly` produces
 * them instead, so the published types come from the compiler rather than from
 * a bundler's reconstruction of them.
 *
 * `@stellar/stellar-sdk` is a peer dependency and stays external: bundling it
 * would duplicate it in any consumer that already depends on it.
 */
export default defineConfig({
  entry: ['src/index.ts'],
  format: ['esm', 'cjs'],
  outExtension: ({ format }) => ({ js: format === 'cjs' ? '.cjs' : '.js' }),
  target: 'node22',
  platform: 'neutral',
  dts: false,
  sourcemap: true,
  clean: true,
  treeshake: true,
  external: ['@stellar/stellar-sdk'],
});
