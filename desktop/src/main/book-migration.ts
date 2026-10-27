import { spawn } from 'node:child_process'
import { safeChildEnv } from './child-env'

/** Параметры шифрования старой plaintext-книги через asledgerctl. */
export interface BookMigrationOptions {
  command: string
  dbPath: string
  password: string
  timeoutMs?: number
  /** Аргументы перед подкомандой; нужны для тестового скрипта. */
  commandArgs?: string[]
  env?: NodeJS.ProcessEnv
}

/** Результат миграции: путь к зашифрованной резервной копии. */
export interface BookMigrationResult {
  backupPath: string | null
}

function redact(text: string, secret: string): string {
  return secret.length > 0 ? text.split(secret).join('***').trim() : text.trim()
}

function backupPathFromOutput(stdout: string): string | null {
  const match = /^резервная копия:\s*(.+)$/m.exec(stdout)
  return match?.[1]?.trim() ?? null
}

/**
 * Зашифровывает plaintext-книгу v1.2.0 перед первым запуском desktop.
 *
 * Пароль передаётся только через stdin (`-password-fd 0`) и не попадает в
 * argv/env. asledgerctl сам создаёт зашифрованную резервную копию, sidecar и
 * атомарно заменяет книгу; при ошибке исходная plaintext-книга остаётся.
 */
export function migratePlaintextBook(
  options: BookMigrationOptions,
): Promise<BookMigrationResult> {
  if (options.password.length === 0) {
    return Promise.reject(new Error('Введите пароль книги'))
  }

  const args = [
    ...(options.commandArgs ?? []),
    'migrate-encrypt',
    '-db',
    options.dbPath,
    '-password-fd',
    '0',
  ]

  return new Promise((resolve, reject) => {
    let child
    try {
      child = spawn(options.command, args, {
        env: safeChildEnv(options.env ?? process.env),
        stdio: ['pipe', 'pipe', 'pipe'],
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
        reject(new Error('Шифрование книги не завершилось за отведённое время'))
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
        resolve({ backupPath: backupPathFromOutput(stdout) })
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
      const details = redact(stderr || stdout, options.password)
      const suffix = signal ? `, сигнал ${signal}` : `, код ${code ?? 'неизвестен'}`
      finish(
        new Error(
          details.length > 0
            ? `Не удалось зашифровать книгу${suffix}: ${details}`
            : `Не удалось зашифровать книгу${suffix}`,
        ),
      )
    })

    const stdin = child.stdin
    if (!stdin) {
      child.kill('SIGKILL')
      finish(new Error('Не удалось открыть канал для передачи пароля книги'))
      return
    }
    stdin.on('error', () => {
      // Ошибку канала заменит код выхода asledgerctl.
    })
    stdin.end(options.password, 'utf8')
  })
}
