import { defineConfig } from 'vitest/config'

// Unit-тесты живут в tests/*.test.ts. E2E запускается Playwright отдельно.
export default defineConfig({
  test: {
    include: ['tests/**/*.test.ts'],
    exclude: ['node_modules/**', 'tests/e2e/**'],
    environment: 'node',
  },
})
