import { describe, expect, it } from 'vitest'
import { waitForHealth } from '../src/main/health'

describe('waitForHealth', () => {
  it('завершается успешно, когда healthcheck отвечает 2xx', async () => {
    const fetchImpl: typeof fetch = async () =>
      new Response('{"status":"ok"}', { status: 200 })

    await expect(
      waitForHealth({
        url: 'http://127.0.0.1:1/api/v1/health',
        timeoutMs: 1000,
        intervalMs: 10,
        fetchImpl,
      }),
    ).resolves.toBeUndefined()
  })

  it('падает, если процесс завершился до готовности', async () => {
    const fetchImpl: typeof fetch = async () =>
      new Response('', { status: 200 })

    await expect(
      waitForHealth({
        url: 'http://127.0.0.1:1/api/v1/health',
        timeoutMs: 1000,
        intervalMs: 10,
        fetchImpl,
        isProcessAlive: () => false,
      }),
    ).rejects.toThrow(/завершился/)
  })

  it('падает по таймауту, если healthcheck не отвечает', async () => {
    const fetchImpl: typeof fetch = async () =>
      new Response('', { status: 503 })

    await expect(
      waitForHealth({
        url: 'http://127.0.0.1:1/api/v1/health',
        timeoutMs: 80,
        intervalMs: 10,
        fetchImpl,
      }),
    ).rejects.toThrow(/HTTP 503/)
  })
})
