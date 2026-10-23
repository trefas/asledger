import { open, stat } from 'node:fs/promises'

export const MAX_LOG_TAIL_LINES = 200
export const MAX_LOG_TAIL_BYTES = 64 * 1024

const SECRET_VALUE =
  /(\b(?:password|passwd|token|secret|api[_-]?key|private[_-]?key|cookie|session)\b\s*[=:]\s*)([^\s,;]+)/gi
const AUTHORIZATION =
  /(\bAuthorization\s*[:=]\s*(?:(?:Bearer|Basic)\s+)?)([^\s,;]+)/gi
const BEARER = /(\bBearer\s+)[A-Za-z0-9._~+/=-]+/gi
const JSON_SECRET =
  /("(?:password|passwd|token|secret|api[_-]?key|private[_-]?key|cookie|session|authorization)"\s*:\s*")([^"]*)(")/gi

/** Маскирует значения токенов, паролей и ключей в строке журнала. */
export function maskSecrets(line: string): string {
  return line
    .replace(SECRET_VALUE, '$1***')
    .replace(AUTHORIZATION, '$1***')
    .replace(BEARER, '$1***')
    .replace(JSON_SECRET, '$1***$3')
}

/**
 * Читает только хвост журнала: не больше maxBytes с конца файла и maxLines
 * строк. Первая строка обрезается, если чтение началось с середины.
 */
export async function readLogTail(
  path: string,
  maxLines = MAX_LOG_TAIL_LINES,
  maxBytes = MAX_LOG_TAIL_BYTES,
): Promise<string[]> {
  let size: number
  try {
    size = (await stat(path)).size
  } catch (error) {
    if ((error as NodeJS.ErrnoException).code === 'ENOENT') {
      return []
    }
    throw error
  }
  if (size === 0) {
    return []
  }

  const start = Math.max(0, size - maxBytes)
  const readStart = Math.max(0, start - 1)
  const length = size - readStart
  const handle = await open(path, 'r')
  try {
    const buffer = Buffer.alloc(length)
    await handle.read(buffer, 0, length, readStart)
    let text = buffer.toString('utf8')
    if (start > 0) {
      const firstNewline = text.indexOf('\n')
      if (firstNewline < 0) {
        return []
      }
      text = text.slice(firstNewline + 1)
    }
    return text
      .split(/\r?\n/)
      .filter((line) => line.length > 0)
      .slice(-maxLines)
      .map(maskSecrets)
  } finally {
    await handle.close()
  }
}
