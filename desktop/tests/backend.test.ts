import { spawn } from 'node:child_process'
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
  mode: 'ready' | 'exit' | 'sleep' | 'port',
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
      expect(log).toContain('password=***')
      expect(log).not.toContain('hunter2')
      expect(log).not.toContain('abc123')
      expect(logs.some((entry) => entry.line.includes('готов'))).toBe(true)
      expect(logs.every((entry) => !entry.line.includes('hunter2'))).toBe(true)
    } finally {
      await rm(dir, { recursive: true, force: true })
    }
  })

  it('передаёт пароль через stdin и не светит его в argv/журнале', async () => {
    const dir = await tempDir()
    try {
      const port = await freePort()
      let seenArgs: string[] = []
      const server = new BackendProcess({
        command: process.execPath,
        args: [fakeBinary, '--port', String(port), '--mode', 'ready'],
        cwd: join(dir, 'data'),
        logFile: join(dir, 'logs', 'asledgerd.log'),
        healthUrl: healthUrl(port),
        webUrl: webUrl(port),
        apiUrl: apiUrl(port),
        startupTimeoutMs: 2000,
        stopTimeoutMs: 1000,
        spawnImpl: ((command: string, args: string[], options: unknown) => {
          seenArgs = args
          return spawn(command, args, options as never)
        }) as unknown as typeof spawn,
      })

      const state = await server.start({ kind: 'password', value: 'hunter2' })
      expect(state.status).toBe('ready')
      expect(seenArgs).toContain('-password-fd')
      expect(seenArgs).not.toContain('hunter2')
      const log = await readFile(join(dir, 'logs', 'asledgerd.log'), 'utf8')
      expect(log).toContain('secret received')
      expect(log).not.toContain('hunter2')
      await server.stop()
    } finally {
      await rm(dir, { recursive: true, force: true })
    }
  })

  it('останавливает запуск, если stop пришёл до появления child', async () => {
    const dir = await tempDir()
    try {
      const port = await freePort()
      const server = makeProcess(port, dir, 'sleep', 5000)
      const starting = server.start()
      const stopped = await server.stop()
      expect(stopped.status).toBe('stopped')
      const state = await starting
      expect(state.status).toBe('stopped')
      expect(state.pid).toBeNull()
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
      expect(state.detail).toMatch(/ошибка|завершился|не ответил/)
    } finally {
      await rm(dir, { recursive: true, force: true })
    }
  })

  it('показывает ошибку занятого порта и путь к журналу', async () => {
    const dir = await tempDir()
    try {
      const port = await freePort()
      const server = makeProcess(port, dir, 'port')
      const state = await server.start()
      expect(state.status).toBe('error')
      expect(state.detail).toContain('address already in use')
      expect(state.logPath).toContain('asledgerd.log')
    } finally {
      await rm(dir, { recursive: true, force: true })
    }
  })

  it('объясняет отсутствие бинаря', async () => {
    const dir = await tempDir()
    try {
      const port = await freePort()
      const server = new BackendProcess({
        command: join(dir, 'no-such-asledgerd'),
        args: [],
        cwd: join(dir, 'data'),
        logFile: join(dir, 'logs', 'asledgerd.log'),
        healthUrl: healthUrl(port),
        webUrl: webUrl(port),
        apiUrl: apiUrl(port),
        startupTimeoutMs: 1000,
        stopTimeoutMs: 100,
      })
      const state = await server.start()
      expect(state.status).toBe('error')
      expect(state.detail).toContain('Не найден asledgerd')
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
