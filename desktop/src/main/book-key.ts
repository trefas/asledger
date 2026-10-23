import { argon2idAsync } from '@noble/hashes/argon2.js'
import { readFile } from 'node:fs/promises'

// Параметры sidecar должны совпадать с internal/key.Params. Sidecar не
// содержит ни пароля, ни результата KDF — только соль и параметры Argon2id.
const SALT_LEN = 16
const KEY_LEN = 32

/** Параметры KDF из sidecar `<db>.kdf.json`. */
export interface KdfParams {
  version: number
  kdf: string
  salt: string
  memory_kib: number
  iterations: number
  parallelism: number
  key_len: number
  cipher: string
  cipher_compatibility: number
  page_size: number
}

/** Путь к sidecar рядом с книгой. */
export function kdfPath(dbPath: string): string {
  return `${dbPath}.kdf.json`
}

function integer(
  value: unknown,
  name: string,
  min: number,
  max: number,
): number {
  if (typeof value !== 'number' || !Number.isInteger(value)) {
    throw new Error(`sidecar: ${name} должен быть целым числом`)
  }
  if (value < min || value > max) {
    throw new Error(`sidecar: ${name} вне диапазона ${min}..${max}`)
  }
  return value
}

/**
 * Проверяет JSON sidecar. Границы совпадают с Go-валидацией, чтобы
 * повреждённый файл не приводил к неконтролируемому расходу памяти.
 */
export function parseKdfParams(raw: unknown): KdfParams {
  if (typeof raw !== 'object' || raw === null || Array.isArray(raw)) {
    throw new Error('sidecar: корень должен быть JSON-объектом')
  }
  const value = raw as Record<string, unknown>
  if (value['version'] !== 1) {
    throw new Error('sidecar: неподдерживаемая версия')
  }
  if (value['kdf'] !== 'argon2id') {
    throw new Error('sidecar: неподдерживаемый KDF')
  }
  if (value['cipher'] !== 'sqlcipher') {
    throw new Error('sidecar: неподдерживаемый шифр')
  }
  if (value['cipher_compatibility'] !== 4) {
    throw new Error('sidecar: неподдерживаемая совместимость SQLCipher')
  }
  if (value['page_size'] !== 4096) {
    throw new Error('sidecar: неподдерживаемый размер страницы')
  }
  if (typeof value['salt'] !== 'string') {
    throw new Error('sidecar: соль должна быть строкой')
  }
  const salt = Buffer.from(value['salt'], 'base64url')
  if (salt.length !== SALT_LEN) {
    throw new Error(`sidecar: длина соли ${salt.length}, ожидалось ${SALT_LEN}`)
  }
  return {
    version: 1,
    kdf: 'argon2id',
    salt: value['salt'],
    memory_kib: integer(value['memory_kib'], 'memory_kib', 8 * 1024, 1024 * 1024),
    iterations: integer(value['iterations'], 'iterations', 1, 10),
    parallelism: integer(value['parallelism'], 'parallelism', 1, 16),
    key_len: integer(value['key_len'], 'key_len', KEY_LEN, KEY_LEN),
    cipher: 'sqlcipher',
    cipher_compatibility: 4,
    page_size: 4096,
  }
}

/** Читает и проверяет sidecar рядом с книгой. */
export async function loadKdfParams(dbPath: string): Promise<KdfParams> {
  const path = kdfPath(dbPath)
  let data: string
  try {
    data = await readFile(path, 'utf8')
  } catch (error) {
    const message = error instanceof Error ? error.message : String(error)
    throw new Error(`sidecar: не удалось прочитать ${path}: ${message}`)
  }
  let parsed: unknown
  try {
    parsed = JSON.parse(data)
  } catch (error) {
    const message = error instanceof Error ? error.message : String(error)
    throw new Error(`sidecar: не удалось разобрать ${path}: ${message}`)
  }
  return parseKdfParams(parsed)
}

/**
 * Выводит 32-байтовый raw-ключ SQLCipher. Используется только для
 * «запомнить ключ»; в safeStorage попадает ключ, а не пароль.
 */
export async function deriveKey(
  password: string,
  params: KdfParams,
): Promise<Buffer> {
  if (password.length === 0) {
    throw new Error('пароль не должен быть пустым')
  }
  const salt = Buffer.from(params.salt, 'base64url')
  // Node/Electron `crypto.argon2` в Electron 45 недоступен (BoringSSL без
  // Argon2), поэтому используем чистую реализацию @noble/hashes. Параметры
  // m/t/p/dkLen совпадают с Go argon2.IDKey.
  const key = await argon2idAsync(
    new Uint8Array(Buffer.from(password, 'utf8')),
    new Uint8Array(salt),
    {
      t: params.iterations,
      m: params.memory_kib,
      p: params.parallelism,
      dkLen: params.key_len,
      asyncTick: 10,
    },
  )
  return Buffer.from(key)
}

/** Выводит ключ из пароля и sidecar книги. */
export async function deriveBookKey(
  password: string,
  dbPath: string,
): Promise<Buffer> {
  return deriveKey(password, await loadKdfParams(dbPath))
}

/** Проверяет, что строка — 32-байтовый ключ в hex. */
export function parseKeyHex(value: string): Buffer {
  const key = Buffer.from(value.trim(), 'hex')
  if (
    key.length !== KEY_LEN ||
    key.toString('hex') !== value.trim().toLowerCase()
  ) {
    throw new Error('ключ: ожидались 64 hex-символа')
  }
  return key
}
