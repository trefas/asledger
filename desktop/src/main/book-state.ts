import { open } from 'node:fs/promises'
import type { BookState } from '../shared/ipc'

/** Начало незашифрованной книги SQLite. */
const sqliteHeader = Buffer.from('SQLite format 3\0', 'latin1')

/**
 * Определяет состояние файла книги без его открытия драйвером.
 *
 * Пустой файл считается отсутствующей книгой: asledger создаст в нём
 * зашифрованную книгу. Нечитаемый или слишком короткий файл даёт unknown,
 * чтобы UI не предлагал миграцию вслепую.
 */
export async function inspectBookState(path: string): Promise<BookState> {
  if (path.trim() === '') {
    return 'unknown'
  }

  let file
  try {
    file = await open(path, 'r')
  } catch (error) {
    const code = (error as NodeJS.ErrnoException).code
    return code === 'ENOENT' ? 'missing' : 'unknown'
  }

  try {
    const info = await file.stat()
    if (!info.isFile()) {
      return 'unknown'
    }
    if (info.size === 0) {
      return 'missing'
    }
    if (info.size < sqliteHeader.length) {
      return 'unknown'
    }

    const header = Buffer.alloc(sqliteHeader.length)
    const { bytesRead } = await file.read(
      header,
      0,
      sqliteHeader.length,
      0,
    )
    if (bytesRead === sqliteHeader.length && header.equals(sqliteHeader)) {
      return 'plaintext'
    }
    return 'encrypted'
  } catch {
    return 'unknown'
  } finally {
    await file.close().catch(() => undefined)
  }
}
