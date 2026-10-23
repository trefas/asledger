import { spawn, type ChildProcess } from 'node:child_process'
import { createWriteStream, type WriteStream } from 'node:fs'
import { mkdir } from 'node:fs/promises'
import { dirname } from 'node:path'
import type { Readable } from 'node:stream'
import type {
  BackendLogEntry,
  BackendState,
  BackendStatus,
} from '../shared/ipc'
import { waitForHealth } from './health'
import { maskSecrets } from './log'

export interface BackendOptions {
  command: string
  args: string[]
  cwd: string
  logFile: string
  healthUrl: string
  webUrl: string
  apiUrl: string
  startupTimeoutMs?: number
  stopTimeoutMs?: number
  spawnImpl?: typeof spawn
  fetchImpl?: typeof fetch
}

function errorText(error: unknown): string {
  return error instanceof Error ? error.message : String(error)
}

function spawnErrorText(command: string, error: Error): string {
  const code = (error as NodeJS.ErrnoException).code
  if (code === 'ENOENT') {
    return `Не найден asledgerd: ${command}. Соберите его или задайте ASLEDGERD_BIN.`
  }
  return `Не удалось запустить asledgerd: ${error.message}`
}

/**
 * Управляет дочерним процессом asledgerd.
 *
 * Класс не зависит от Electron: пути и команда передаются снаружи, поэтому
 * логику запуска, healthcheck и остановки можно тестировать отдельно.
 */
export class BackendProcess {
  private readonly options: Required<
    Pick<BackendOptions, 'startupTimeoutMs' | 'stopTimeoutMs'>
  > &
    BackendOptions
  private child: ChildProcess | null = null
  private logStream: WriteStream | null = null
  private lastStderr = ''
  private spawnError: Error | null = null
  private stopping = false
  private startPromise: Promise<BackendState> | null = null
  private state: BackendState
  private readonly stateListeners = new Set<(state: BackendState) => void>()
  private readonly logListeners = new Set<(entry: BackendLogEntry) => void>()

  constructor(options: BackendOptions) {
    this.options = {
      ...options,
      startupTimeoutMs: options.startupTimeoutMs ?? 15_000,
      stopTimeoutMs: options.stopTimeoutMs ?? 10_000,
    }
    this.state = {
      status: 'stopped',
      detail: 'Сервер не запущен',
      pid: null,
      webUrl: options.webUrl,
      apiUrl: options.apiUrl,
      logPath: options.logFile,
    }
  }

  getState(): BackendState {
    return { ...this.state }
  }

  onState(listener: (state: BackendState) => void): () => void {
    this.stateListeners.add(listener)
    listener(this.getState())
    return () => {
      this.stateListeners.delete(listener)
    }
  }

  onLog(listener: (entry: BackendLogEntry) => void): () => void {
    this.logListeners.add(listener)
    return () => {
      this.logListeners.delete(listener)
    }
  }

  /** Запускает сервер и ждёт healthcheck. Повторный вызов безопасен. */
  async start(): Promise<BackendState> {
    if (this.state.status === 'ready' || this.state.status === 'starting') {
      return this.startPromise ?? this.getState()
    }
    if (this.state.status === 'stopping') {
      return this.getState()
    }

    this.stopping = false
    this.lastStderr = ''
    this.spawnError = null
    this.setState('starting', 'Запуск asledgerd')
    this.startPromise = this.doStart()
    try {
      return await this.startPromise
    } finally {
      this.startPromise = null
    }
  }

  /** Останавливает сервер: SIGTERM, затем принудительное завершение. */
  async stop(): Promise<BackendState> {
    this.stopping = true
    if (!this.child) {
      this.setState('stopped', 'Сервер не запущен', null)
      return this.getState()
    }

    this.setState('stopping', 'Остановка сервера')
    await this.stopChild()
    this.child = null
    this.closeLog()
    this.setState('stopped', 'Сервер остановлен', null)
    return this.getState()
  }

  /** Перезапускает сервер. */
  async restart(): Promise<BackendState> {
    await this.stop()
    return this.start()
  }

  private async doStart(): Promise<BackendState> {
    let child: ChildProcess | null = null
    try {
      await mkdir(this.options.cwd, { recursive: true })
      await this.openLog()

      const spawnImpl = this.options.spawnImpl ?? spawn
      const spawned = spawnImpl(this.options.command, this.options.args, {
        cwd: this.options.cwd,
        env: { ...process.env },
        stdio: ['ignore', 'pipe', 'pipe'],
        windowsHide: true,
      })
      child = spawned
      this.child = spawned
      this.attachOutput(spawned)
      spawned.once('error', (error) => {
        this.handleSpawnError(spawned, error)
      })
      spawned.once('exit', (code, signal) => {
        this.handleExit(spawned, code, signal)
      })

      this.setState('starting', 'Ожидание healthcheck', spawned.pid ?? null)
      await waitForHealth({
        url: this.options.healthUrl,
        timeoutMs: this.options.startupTimeoutMs,
        fetchImpl: this.options.fetchImpl,
        isProcessAlive: () =>
          this.spawnError === null &&
          spawned.exitCode === null &&
          spawned.signalCode === null,
      })

      if (this.stopping || this.child !== spawned) {
        if (this.stopping) {
          this.setState('stopped', 'Запуск отменён', null)
        } else {
          this.setState(
            'error',
            this.lastStderr || 'asledgerd завершился до готовности',
            null,
          )
        }
        return this.getState()
      }

      this.setState('ready', 'Сервер готов', spawned.pid ?? null)
      return this.getState()
    } catch (error) {
      const message = this.spawnError
        ? spawnErrorText(this.options.command, this.spawnError)
        : this.lastStderr || errorText(error)
      await this.stopChild()
      this.closeLog()
      if (this.stopping) {
        this.setState('stopped', 'Запуск отменён', null)
      } else {
        this.setState('error', message, null)
      }
      return this.getState()
    } finally {
      if (child && child.exitCode !== null && this.child === child) {
        this.child = null
      }
    }
  }

  private attachOutput(child: ChildProcess): void {
    this.pipeOutput(child.stdout, 'stdout')
    this.pipeOutput(child.stderr, 'stderr')
  }

  private pipeOutput(stream: Readable | null, name: 'stdout' | 'stderr'): void {
    if (!stream) {
      return
    }

    stream.setEncoding('utf8')
    let buffer = ''
    stream.on('data', (chunk: string) => {
      buffer += chunk
      const lines = buffer.split(/\r?\n/)
      buffer = lines.pop() ?? ''
      for (const line of lines) {
        this.appendLog(name, line)
      }
      if (buffer.length > 64 * 1024) {
        this.appendLog(name, buffer)
        buffer = ''
      }
    })
    stream.on('end', () => {
      if (buffer) {
        this.appendLog(name, buffer)
        buffer = ''
      }
    })
  }

  private async openLog(): Promise<void> {
    await mkdir(dirname(this.options.logFile), { recursive: true })
    this.logStream = createWriteStream(this.options.logFile, { flags: 'a' })
    this.logStream.on('error', (error) => {
      this.logStream = null
      this.emitLog({
        stream: 'stderr',
        line: `не удалось записать журнал: ${errorText(error)}`,
        at: new Date().toISOString(),
      })
    })
  }

  private closeLog(): void {
    if (!this.logStream) {
      return
    }
    this.logStream.end()
    this.logStream = null
  }

  private appendLog(stream: 'stdout' | 'stderr', line: string): void {
    const safeLine = maskSecrets(line)
    if (
      stream === 'stderr' &&
      /(level=ERROR|\berror\b|fatal|asledgerd:)/i.test(safeLine)
    ) {
      this.lastStderr = safeLine
    }
    const entry: BackendLogEntry = {
      stream,
      line: safeLine,
      at: new Date().toISOString(),
    }
    try {
      this.logStream?.write(`[${entry.at}] [${stream}] ${safeLine}\n`)
    } catch {
      this.logStream = null
    }
    this.emitLog(entry)
  }

  private emitLog(entry: BackendLogEntry): void {
    for (const listener of [...this.logListeners]) {
      listener(entry)
    }
  }

  private setState(
    status: BackendStatus,
    detail: string,
    pid: number | null = this.child?.pid ?? null,
  ): void {
    this.state = {
      status,
      detail,
      pid,
      webUrl: this.options.webUrl,
      apiUrl: this.options.apiUrl,
      logPath: this.options.logFile,
    }
    for (const listener of [...this.stateListeners]) {
      listener(this.getState())
    }
  }

  private handleSpawnError(child: ChildProcess, error: Error): void {
    this.spawnError = error
    if (this.child === child) {
      this.child = null
    }
    this.closeLog()
    this.setState(
      'error',
      spawnErrorText(this.options.command, error),
      null,
    )
  }

  private handleExit(
    child: ChildProcess,
    code: number | null,
    signal: NodeJS.Signals | null,
  ): void {
    if (this.child === child) {
      this.child = null
    }
    this.closeLog()

    if (this.stopping) {
      this.setState('stopped', 'Сервер остановлен', null)
      return
    }

    const reason =
      signal !== null
        ? `сигнал ${signal}`
        : code !== null
          ? `код ${code}`
          : 'неизвестная причина'
    this.setState(
      'error',
      this.lastStderr || `asledgerd завершился: ${reason}`,
      null,
    )
  }

  private async stopChild(): Promise<void> {
    const child = this.child
    if (!child) {
      return
    }

    if (child.exitCode !== null || child.signalCode !== null) {
      this.child = null
      return
    }

    await new Promise<void>((resolve) => {
      let finished = false
      let timer: ReturnType<typeof setTimeout> | undefined
      const done = (): void => {
        if (finished) {
          return
        }
        finished = true
        if (timer) {
          clearTimeout(timer)
        }
        resolve()
      }

      child.once('exit', done)
      this.killChild(child, false)
      timer = setTimeout(() => {
        this.killChild(child, true)
        done()
      }, this.options.stopTimeoutMs)
    })

    if (this.child === child) {
      this.child = null
    }
  }

  private killChild(child: ChildProcess, force: boolean): void {
    if (process.platform === 'win32' && child.pid) {
      const args = ['/PID', String(child.pid), '/T']
      if (force) {
        args.push('/F')
      }
      const spawnImpl = this.options.spawnImpl ?? spawn
      const killer = spawnImpl('taskkill', args, {
        stdio: 'ignore',
        windowsHide: true,
      })
      killer.on('error', () => {
        child.kill()
      })
      return
    }

    child.kill(force ? 'SIGKILL' : 'SIGTERM')
  }
}
