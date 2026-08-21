// @ts-check
/**
 * Shared ESLint configuration for every TypeScript workspace.
 *
 * Each package extends this with its own `eslint.config.mjs`, which supplies
 * the `tsconfigRootDir` that type-aware rules need. Rules here encode the
 * discipline in CONTRIBUTING.md rather than taste: anything a reviewer would
 * send a PR back for, and that a linter can see, belongs here.
 */

import js from '@eslint/js';
import importX from 'eslint-plugin-import-x';
import unusedImports from 'eslint-plugin-unused-imports';
import tseslint from 'typescript-eslint';

export default tseslint.config(
  {
    ignores: ['**/dist/**', '**/coverage/**', '**/node_modules/**', '**/.next/**'],
  },

  js.configs.recommended,
  tseslint.configs.recommendedTypeChecked,

  {
    // Only import-x's ordering rules are enabled. Its resolution rules
    // duplicate what `tsc --noEmit` already checks in CI, and enabling them
    // means maintaining a second module resolver that has to agree with
    // TypeScript's.
    plugins: { 'import-x': importX, 'unused-imports': unusedImports },
    rules: {
      // CONTRIBUTING: no any, no @ts-ignore without an ADR.
      '@typescript-eslint/no-explicit-any': 'error',
      '@typescript-eslint/ban-ts-comment': [
        'error',
        { 'ts-expect-error': 'allow-with-description', 'ts-ignore': true },
      ],

      // CONTRIBUTING: no floating promises, every promise awaited or handled.
      '@typescript-eslint/no-floating-promises': 'error',
      '@typescript-eslint/no-misused-promises': 'error',
      '@typescript-eslint/await-thenable': 'error',

      // CONTRIBUTING: no non-null assertions in shipped code.
      '@typescript-eslint/no-non-null-assertion': 'error',

      // Unused imports are removable; unused bindings escape with a leading _.
      '@typescript-eslint/no-unused-vars': 'off',
      'unused-imports/no-unused-imports': 'error',
      'unused-imports/no-unused-vars': [
        'error',
        {
          args: 'after-used',
          argsIgnorePattern: '^_',
          varsIgnorePattern: '^_',
          caughtErrors: 'all',
          caughtErrorsIgnorePattern: '^_',
        },
      ],

      // Import hygiene, per section 13.1.
      'import-x/order': [
        'error',
        {
          groups: ['builtin', 'external', 'internal', 'parent', 'sibling', 'index'],
          'newlines-between': 'always',
          alphabetize: { order: 'asc', caseInsensitive: true },
        },
      ],
      'import-x/no-duplicates': 'error',

      // A type-only import that is not marked as one is a runtime import that
      // survives bundling for no reason.
      '@typescript-eslint/consistent-type-imports': [
        'error',
        { prefer: 'type-imports', fixStyle: 'inline-type-imports' },
      ],
    },
  },

  {
    // Tests deliberately construct invalid values to prove they are rejected,
    // so the assertions that check that cannot themselves typecheck cleanly.
    files: ['**/tests/**'],
    rules: {
      '@typescript-eslint/no-unsafe-assignment': 'off',
      '@typescript-eslint/no-unsafe-argument': 'off',
    },
  },

  {
    // Config files are not part of any package's tsconfig program.
    files: ['**/*.config.{ts,mts,js,mjs}', 'eslint.config.mjs'],
    extends: [tseslint.configs.disableTypeChecked],
  },
);
