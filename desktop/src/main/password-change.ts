import { spawn } from 'node:child_process'
import { existsSync } from 'node:fs'
import { mkdtemp, rm, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import type { Writable } from 'node:stream'

/** Параметры смены пароля книги через asledgerctl. */
export interface PasswordChangeOptions {
  command: string
  dbPath: string
  currentPassword: string
  newPassword: string
  backupPath?: string
  timeoutMs?: number
  /** Аргументы перед подкомандой; нужны для тестового скрипта. */
  commandArgs?: string[]
  env?: NodeJS.ProcessEnv
  /** Платформа нужна для выбора способа передачи нового пароля в тестах. */
  platform?: NodeJS.Platform
}

/** Результат успешной смены пароля. */
export interface PasswordChangeResult {
  backupPath: string
}

function redact(text: string, secrets: string[]): string {
  let result = text
  for (const secret of secrets) {
    if (secret.length > 0) {
      result = result.split(secret).join('***')
    }
  }
  return result.trim()
}

/** Убирает из окружения переменные, которые могут подсунуть другой ключ. */
function safeEnv(env: NodeJS.ProcessEnv): NodeJS.ProcessEnv {
  const copy = { ...env }
  for (const key of Object.keys(copy)) {
    // Windows сравнивает имена переменных без учёта регистра.
    if (key.toUpperCase().startsWith('ASLEDGER_')) {
      delete copy[key]
    }
  }
  return copy
}

function backupPathFor(dbPath: string): string {
  const stamp = new Date()
    .toISOString()
    .replace(/[-:]/g, '')
    .replace(/\.\d+Z$/, '')
    .replace('T', '-')
  const base = `${dbPath}.pre-rekey-${stamp}.db`
  if (!existsSync(base)) {
    return base
  }
  for (let index = 1; ; index += 1) {
    const candidate = `${base}.${index}`
    if (!existsSync(candidate)) {
      return candidate
    }
  }
}

function runCtl(
  options: PasswordChangeOptions,
  args: string[],
  useExtraFD: boolean,
  backupPath: string,
): Promise<PasswordChangeResult> {
  const secrets = [options.currentPassword, options.newPassword]
  const stdio: Array<'pipe'> = useExtraFD
    ? ['pipe', 'pipe', 'pipe', 'pipe']
    : ['pipe', 'pipe', 'pipe']

  return new Promise((resolve, reject) => {
    let child
    try {
      child = spawn(options.command, args, {
        env: safeEnv(options.env ?? process.env),
        stdio,
        windowsHide: true,
      })
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error)
      reject(new Error(`Не удалось запустить asledgerctl: ${message}`))
      return
    }

    let stdout = ''
    let stderr = ''
    let settled = false
    const timeoutMs = options.timeoutMs ?? 120_000
    const timer = setTimeout(() => {
      if (!settled) {
        settled = true
        child.kill('SIGKILL')
        reject(new Error('Смена пароля не завершилась за отведённое время'))
      }
    }, timeoutMs)

    const finish = (error: Error | null): void => {
      if (settled) {
        return
      }
      settled = true
      clearTimeout(timer)
      if (error) {
        reject(error)
      } else {
        resolve({ backupPath })
      }
    }

    child.stdout?.setEncoding('utf8')
    child.stderr?.setEncoding('utf8')
    child.stdout?.on('data', (chunk: string) => {
      stdout = (stdout + chunk).slice(-64 * 1024)
    })
    child.stderr?.on('data', (chunk: string) => {
      stderr = (stderr + chunk).slice(-64 * 1024)
    })
    child.once('error', (error) => {
      const code = (error as NodeJS.ErrnoException).code
      const message =
        code === 'ENOENT'
          ? `Не найден asledgerctl: ${options.command}`
          : `Не удалось запустить asledgerctl: ${error.message}`
      finish(new Error(message))
    })
    child.once('close', (code, signal) => {
      if (settled) {
        return
      }
      if (code === 0) {
        finish(null)
        return
      }
      const details = redact(stderr || stdout, secrets)
      const suffix = signal ? `, сигнал ${signal}` : `, код ${code ?? 'неизвестен'}`
      finish(
        new Error(
          details.length > 0
            ? `Не удалось сменить пароль${suffix}: ${details}`
            : `Не удалось сменить пароль${suffix}`,
        ),
      )
    })

    const stdin = child.stdin
    if (!stdin) {
      child.kill('SIGKILL')
      finish(new Error('Не удалось открыть канал для передачи текущего пароля'))
      return
    }
    stdin.on('error', () => {
      // Ошибку канала заменит код выхода asledgerctl.
    })
    stdin.end(options.currentPassword, 'utf8')

    if (useExtraFD) {
      const newPasswordFD = child.stdio[3] as Writable | null
      if (!newPasswordFD) {
        child.kill('SIGKILL')
        finish(new Error('Не удалось открыть канал для передачи нового пароля'))
        return
      }
      newPasswordFD.on('error', () => {
        // Ошибку канала заменит код выхода asledgerctl.
      })
      newPasswordFD.end(options.newPassword, 'utf8')
    }
  })
}

/**
 * Меняет пароль книги, не передавая секреты в argv/env.
 *
 * На Unix старый пароль идёт в stdin, новый — в fd 3. На Windows
 * дополнительный fd не гарантирован, поэтому новый пароль пишется во
 * временный файл 0600 в пользовательском temp и удаляется после запуска.
 * asledgerctl сам создаёт зашифрованную резервную копию, проверяет новую
 * книгу и атомарно заменяет файл и sidecar.
 */
export async function changeBookPassword(
  options: PasswordChangeOptions,
): Promise<PasswordChangeResult> {
  if (options.currentPassword.length === 0) {
    throw new Error('Введите текущий пароль книги')
  }
  if (options.newPassword.length === 0) {
    throw new Error('Введите новый пароль книги')
  }

  const platform = options.platform ?? process.platform
  const backupPath = options.backupPath ?? backupPathFor(options.dbPath)
  const useExtraFD = platform !== 'win32'
  let tempDir: string | null = null

  try {
    const args = [
      ...(options.commandArgs ?? []),
      'passwd',
      '-db',
      options.dbPath,
      '-password-fd',
      '0',
    ]
    if (useExtraFD) {
      args.push('-new-password-fd', '3')
    } else {
      tempDir = await mkdtemp(join(tmpdir(), 'asledger-passwd-'))
      const newPasswordFile = join(tempDir, 'new-password')
      await writeFile(newPasswordFile, options.newPassword, { mode: 0o600 })
      args.push('-new-password-file', newPasswordFile)
    }
    args.push('-backup', backupPath)
    return await runCtl(options, args, useExtraFD, backupPath)
  } finally {
    if (tempDir !== null) {
      await rm(tempDir, {
        recursive: true,
        force: true,
        maxRetries: 3,
        retryDelay: 100,
      }).catch(() => undefined)
    }
  }
}
