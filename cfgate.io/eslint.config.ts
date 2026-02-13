import js from '@eslint/js'
import { defineConfig } from 'eslint/config'
import eslintPluginAstro from 'eslint-plugin-astro'
import globals from 'globals'
import tseslint from 'typescript-eslint'

export default defineConfig([
  {
    rules: {
      'no-unused-vars': ['error', { argsIgnorePattern: '^_' }],
    },
    languageOptions: {
      globals: {
        ...globals.browser,
        ...globals.node,
        ...globals.serviceworker,
      },
      parser: tseslint.parser,
      parserOptions: {
        ecmaFeatures: {
          impliedStrict: true,
        },
        projectService: true,
        sourceType: 'module',
      },
    },
    plugins: {
      js: js,
      tseslint: tseslint,
    },
    extends: [js.configs.recommended, tseslint.configs.stylisticTypeChecked],
    files: ['src/**/*.ts'],
    ignores: ['docs/**', 'node_modules/**', 'dist/**'],
  },
  ...eslintPluginAstro.configs.recommended,
])
