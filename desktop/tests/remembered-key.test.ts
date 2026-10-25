import { mkdtemp, readFile, rm, stat, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const state = vi.hoisted(() => ({
  dir: '',
  available: true,
  backend: 'gnome_libsecret',
}))

vi.mock('electron', () => ({
  app: {
    getPath: () => state.dir,
  },
  safeStorage: {
    isAsyncEncryptionAvailable: async () => state.available,
    getSelectedStorageBackend: () => state.backend,
    encryptStringAsync: async (value: string) =>
      Buffer.from(`encrypted:${value}`, 'utf8'),
    decryptStringAsync: async (value: Buffer) => ({
      result: value.toString('utf8').replace(/^encrypted:/, ''),
      shouldReEncrypt: false,
    }),
  },
}))

import {
  forgetRememberedKey,
  keyStorageAvailable,
  keyStorageStatus,
  loadRememberedKey,
  saveRememberedKey,
} from '../src/main/remembered-key'

const key = Buffer.from('ab'.repeat(32), 'hex')
const expectedBackend = process.platform === 'linux' ? 'gnome_libsecret' : 'os'

describe('remembered-key', () => {
  beforeEach(async () => {
    state.dir = await mkdtemp(join(tmpdir(), 'asledger-remembered-'))
    state.available = true
    state.backend = expectedBackend
  })

  afterEach(async () => {
    await rm(state.dir, { recursive: true, force: true })
  })

  it('сохраняет и загружает raw-ключ', async () => {
    await expect(keyStorageAvailable()).resolves.toBe(true)
    await saveRememberedKey(key)
    await expect(loadRememberedKey()).resolves.toEqual(key)
    await expect(keyStorageStatus()).resolves.toEqual({
      available: true,
      remembered: true,
      backend: expectedBackend,
    })
    if (process.platform !== 'win32') {
      const info = await stat(join(state.dir, 'book-key.bin'))
      expect(info.mode & 0o077).toBe(0)
    }
  })

  it('перезаписывает сохранённый ключ', async () => {
    await saveRememberedKey(key)
    const next = Buffer.from('cd'.repeat(32), 'hex')
    await saveRememberedKey(next)
    await expect(loadRememberedKey()).resolves.toEqual(next)
  })

  it('удаляет сохранённый ключ', async () => {
    await saveRememberedKey(key)
    await forgetRememberedKey()
    await expect(loadRememberedKey()).resolves.toBeNull()
    await expect(keyStorageStatus()).resolves.toMatchObject({ remembered: false })
  })

  it.skipIf(process.platform !== 'linux')(
    'не сохраняет ключ при basic_text',
    async () => {
      state.backend = 'basic_text'
      await expect(keyStorageAvailable()).resolves.toBe(false)
      await expect(saveRememberedKey(key)).rejects.toThrow(/недоступно/)
    },
  )

  it('не сохраняет ключ без шифрования', async () => {
    state.available = false
    await expect(keyStorageAvailable()).resolves.toBe(false)
    await expect(saveRememberedKey(key)).rejects.toThrow(/недоступно/)
  })

  it('игнорирует повреждённый файл', async () => {
    await writeFile(join(state.dir, 'book-key.bin'), 'мусор', 'utf8')
    await expect(loadRememberedKey()).resolves.toBeNull()
  })

  it('не отдаёт ключ неверной длины', async () => {
    await writeFile(
      join(state.dir, 'book-key.bin'),
      'encrypted:abcd',
      'utf8',
    )
    await expect(loadRememberedKey()).resolves.toBeNull()
    // Файл не удаляется молча: пользователь может разобраться.
    await expect(readFile(join(state.dir, 'book-key.bin'), 'utf8')).resolves.toBe(
      'encrypted:abcd',
    )
  })
})
