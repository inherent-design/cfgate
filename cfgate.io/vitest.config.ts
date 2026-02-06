import { defineWorkersConfig } from '@cloudflare/vitest-pool-workers/config'

export default defineWorkersConfig({
  esbuild: {
    exclude: ['node_modules', 'docs'],
  },
  test: {
    include: ['tests\/{**,.}\/*.test.ts'],
    poolOptions: {
      workers: {
        wrangler: { configPath: './wrangler.toml' },
      },
    },
  },
})
