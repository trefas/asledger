import { mkdtemp, readFile, rm } from 'node:fs/promises'
import { createServer } from 'node:net'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { describe, expect, it } from 'vitest'
import { BackendProcess } from '../src/main/backend'
import { apiUrl, healthUrl, webUrl } from '../src/main/paths'
import type { BackendLogEntry, BackendState } from '../src/shared/ipc'

const fakeBinary = join(
  process.cwd(),
  'tests',
  'fixtures',
  'fake-asledgerd.mjs',
)

async function freePort(): Promise<number> {
  return new Promise((resolve, reject) => {
    const server = createServer()
    server.once('error', reject)
    server.listen(0, '127.0.0.1', () => {
      const address = server.address()
      if (!address || typeof address === 'string') {
        server.close()
        reject(new Error('не удалось получить свободный порт'))
        return
      }
      const port = address.port
      server.close(() => resolve(port))
    })
  })
}

async function tempDir(): Promise<string> {
  return mkdtemp(join(tmpdir(), 'asledger-desktop-test-'))
}

function makeProcess(
  port: number,
  dir: string,
  mode: 'ready' | 'exit' | 'sleep',
  startupTimeoutMs = 2000,
): BackendProcess {
  return new BackendProcess({
    command: process.execPath,
    args: [fakeBinary, '--port', String(port), '--mode', mode],
    cwd: join(dir, 'data'),
    logFile: join(dir, 'logs', 'asledgerd.log'),
    healthUrl: healthUrl(port),
    webUrl: webUrl(port),
    apiUrl: apiUrl(port),
    startupTimeoutMs,
    stopTimeoutMs: 1000,
  })
}

describe('BackendProcess', () => {
  it('запускает сервер, ждёт healthcheck и пишет журнал', async () => {
    const dir = await tempDir()
    try {
      const port = await freePort()
      const server = makeProcess(port, dir, 'ready')
      const states: BackendState[] = []
      const logs: BackendLogEntry[] = []
      server.onState((state) => states.push(state))
      server.onLog((entry) => logs.push(entry))

      const started = await server.start()
      expect(started.status).toBe('ready')
      expect(started.pid).not.toBeNull()
      expect(states.map((state) => state.status)).toContain('starting')
      expect(states.at(-1)?.status).toBe('ready')

      const stopped = await server.stop()
      expect(stopped.status).toBe('stopped')
      expect(stopped.pid).toBeNull()

      const log = await readFile(join(dir, 'logs', 'asledgerd.log'), 'utf8')
      expect(log).toContain('fake-asledgerd: готов')
      expect(log).toContain('тестовая строка stderr')
      expect(logs.some((entry) => entry.line.includes('готов'))).toBe(true)
    } finally {
      await rm(dir, { recursive: true, force: true })
    }
  })

  it('возвращает ошибку, если процесс завершился до готовности', async () => {
    const dir = await tempDir()
    try {
      const port = await freePort()
      const server = makeProcess(port, dir, 'exit')
      const state = await server.start()
      expect(state.status).toBe('error')
      expect(state.detail).toMatch(/завершился|не ответил/)
    } finally {
      await rm(dir, { recursive: true, force: true })
    }
  })

  it('возвращает ошибку по таймауту и не оставляет процесс', async () => {
    const dir = await tempDir()
    try {
      const port = await freePort()
      const server = makeProcess(port, dir, 'sleep', 120)
      const state = await server.start()
      expect(state.status).toBe('error')
      expect(state.detail).toMatch(/не ответил|завершился/)
      expect(server.getState().pid).toBeNull()
    } finally {
      await rm(dir, { recursive: true, force: true })
    }
  })
})
