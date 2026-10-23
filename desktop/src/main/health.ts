export interface WaitForHealthOptions {
  url: string
  timeoutMs: number
  intervalMs?: number
  fetchImpl?: typeof fetch
  isProcessAlive?: () => boolean
}

function delay(ms: number): Promise<void> {
  return new Promise((resolve) => {
    setTimeout(resolve, ms)
  })
}

function errorText(error: unknown): string {
  return error instanceof Error ? error.message : String(error)
}

/**
 * Ждёт, пока healthcheck начнёт отвечать 2xx.
 *
 * Если процесс завершился до готовности, ожидание прекращается сразу, чтобы
 * Electron не показывал «Запуск…» до конца таймаута.
 */
export async function waitForHealth(options: WaitForHealthOptions): Promise<void> {
  const fetchImpl = options.fetchImpl ?? fetch
  const intervalMs = options.intervalMs ?? 250
  const deadline = Date.now() + options.timeoutMs
  let lastError = 'healthcheck ещё не отвечал'

  while (Date.now() < deadline) {
    if (options.isProcessAlive && !options.isProcessAlive()) {
      throw new Error('процесс asledgerd завершился до готовности')
    }

    const controller = new AbortController()
    const requestTimeout = setTimeout(() => controller.abort(), 1000)
    try {
      const response = await fetchImpl(options.url, {
        signal: controller.signal,
        cache: 'no-store',
      })
      if (response.ok) {
        return
      }
      lastError = `healthcheck вернул HTTP ${response.status}`
    } catch (error) {
      lastError = errorText(error)
    } finally {
      clearTimeout(requestTimeout)
    }

    await delay(intervalMs)
  }

  throw new Error(`asledgerd не ответил за отведённое время: ${lastError}`)
}
