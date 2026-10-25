import { defineConfig } from '@playwright/test'

// Smoke-тест Electron. Запускается отдельно от Vitest: см. vitest.config.mts.
export default defineConfig({
  testDir: './tests/e2e',
  testMatch: '**/*.spec.ts',
  timeout: 60_000,
  workers: 1,
  fullyParallel: false,
  reporter: [['list']],
  use: {
    trace: 'retain-on-failure',
  },
})
