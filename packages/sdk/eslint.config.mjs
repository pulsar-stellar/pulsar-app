// @ts-check
/**
 * SDK lint configuration. Extends the workspace config and points the
 * type-aware rules at this package's tsconfig.
 */

import base from '../../eslint.config.mjs';

export default [
  // Contract bindings generated at test time by the ADR-016 composition
  // check. Not ours to lint.
  { ignores: ['tmp/**'] },
  ...base,
  {
    files: ['src/**/*.ts', 'tests/**/*.ts'],
    languageOptions: {
      parserOptions: {
        projectService: true,
        tsconfigRootDir: import.meta.dirname,
      },
    },
  },
];
