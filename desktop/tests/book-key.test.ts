import { mkdtemp, rm, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { describe, expect, it } from 'vitest'
import {
  deriveKey,
  kdfPath,
  loadKdfParams,
  parseKdfParams,
  parseKeyHex,
  type KdfParams,
} from '../src/main/book-key'

const salt = Buffer.alloc(16, 1).toString('base64url')

function params(overrides: Partial<KdfParams> = {}): KdfParams {
  return {
    version: 1,
    kdf: 'argon2id',
    salt,
    memory_kib: 65536,
    iterations: 3,
    parallelism: 4,
    key_len: 32,
    cipher: 'sqlcipher',
    cipher_compatibility: 4,
    page_size: 4096,
    ...overrides,
  }
}

describe('book-key', () => {
  it('разбирает корректный sidecar', () => {
    expect(parseKdfParams(params())).toEqual(params())
  })

  it('отклоняет опасные параметры KDF', () => {
    expect(() => parseKdfParams(params({ memory_kib: 1024 * 1024 + 1 }))).toThrow(
      /memory_kib/,
    )
    expect(() => parseKdfParams(params({ iterations: 0 }))).toThrow(
      /iterations/,
    )
    expect(() => parseKdfParams(params({ salt: 'AAAA' }))).toThrow(/соли/)
  })

  it('выводит тот же ключ, что Go Argon2id', async () => {
    const key = await deriveKey('p', params())
    expect(key.toString('hex')).toBe(
      '1f94197f0ff03988769d41e48503d34c3471a9dd4010185073eef01487ac66cc',
    )
  })

  it('читает sidecar рядом с книгой', async () => {
    const dir = await mkdtemp(join(tmpdir(), 'asledger-book-key-'))
    try {
      const dbPath = join(dir, 'book.db')
      await writeFile(kdfPath(dbPath), JSON.stringify(params()))
      await expect(loadKdfParams(dbPath)).resolves.toEqual(params())
    } finally {
      await rm(dir, { recursive: true, force: true })
    }
  })

  it('проверяет hex-ключ', () => {
    const hex = 'ab'.repeat(32)
    expect(parseKeyHex(hex).toString('hex')).toBe(hex)
    expect(() => parseKeyHex('ab')).toThrow(/64 hex/)
  })
})
