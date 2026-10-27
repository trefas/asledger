import { mkdtemp, rm, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { describe, expect, it } from 'vitest'
import { inspectBookState } from '../src/main/book-state'

async function tempDir(): Promise<string> {
  return mkdtemp(join(tmpdir(), 'asledger-book-state-'))
}

describe('состояние книги', () => {
  it('отличает отсутствующую, пустую, plaintext и зашифрованную книгу', async () => {
    const dir = await tempDir()
    try {
      const missing = join(dir, 'missing.db')
      expect(await inspectBookState(missing)).toBe('missing')

      const empty = join(dir, 'empty.db')
      await writeFile(empty, '')
      expect(await inspectBookState(empty)).toBe('missing')

      const plaintext = join(dir, 'plaintext.db')
      await writeFile(plaintext, 'SQLite format 3\0остальное')
      expect(await inspectBookState(plaintext)).toBe('plaintext')

      const encrypted = join(dir, 'encrypted.db')
      await writeFile(encrypted, Buffer.from('0123456789abcdef'))
      expect(await inspectBookState(encrypted)).toBe('encrypted')

      const short = join(dir, 'short.db')
      await writeFile(short, 'SQLite')
      expect(await inspectBookState(short)).toBe('unknown')
    } finally {
      await rm(dir, { recursive: true, force: true })
    }
  })

  it('возвращает unknown для пустого пути и каталога', async () => {
    const dir = await tempDir()
    try {
      expect(await inspectBookState('')).toBe('unknown')
      expect(await inspectBookState(dir)).toBe('unknown')
    } finally {
      await rm(dir, { recursive: true, force: true })
    }
  })
})
