// @ts-check
/**
 * Web app lint configuration. Extends the workspace config and points the
 * type-aware rules at this package's tsconfig, exactly as the SDK does.
 */

import base from '../../eslint.config.mjs';

export default [
  // Next build output and generated route types are not ours to lint.
  { ignores: ['.next/**', 'next-env.d.ts'] },
  ...base,
  {
    files: ['src/**/*.{ts,tsx}', 'tests/**/*.{ts,tsx}', 'vitest.setup.ts'],
    languageOptions: {
      parserOptions: {
        projectService: true,
        tsconfigRootDir: import.meta.dirname,
      },
    },
  },
];
