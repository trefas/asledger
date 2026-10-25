import { app, safeStorage } from 'electron'
import { mkdir, readFile, rename, rm, writeFile } from 'node:fs/promises'
import { dirname, join } from 'node:path'
import { parseKeyHex } from './book-key'

/** Состояние системного хранилища ключа. */
export interface KeyStorageStatus {
  /** Доступно ли безопасное шифрование (не basic_text на Linux). */
  available: boolean
  /** Есть ли сохранённый ключ, который удалось расшифровать. */
  remembered: boolean
  /** Имя backend-а safeStorage для диагностики. */
  backend: string
}

function keyFilePath(): string {
  return join(app.getPath('userData'), 'book-key.bin')
}

function backendName(): string {
  if (process.platform !== 'linux') {
    return 'os'
  }
  try {
    return safeStorage.getSelectedStorageBackend()
  } catch {
    return 'unknown'
  }
}

/** Проверяет, можно ли хранить ключ в системном хранилище. */
export async function keyStorageAvailable(): Promise<boolean> {
  try {
    if (!(await safeStorage.isAsyncEncryptionAvailable())) {
      return false
    }
  } catch {
    return false
  }
  // basic_text — это не шифрование, а обратимое кодирование; такой ключ
  // нельзя считать защищённым.
  return backendName() !== 'basic_text'
}

/** Возвращает сохранённый raw-ключ или null. */
export async function loadRememberedKey(): Promise<Buffer | null> {
  if (!(await keyStorageAvailable())) {
    return null
  }
  let encrypted: Buffer
  try {
    encrypted = await readFile(keyFilePath())
  } catch {
    return null
  }
  try {
    const { result, shouldReEncrypt } =
      await safeStorage.decryptStringAsync(encrypted)
    const key = parseKeyHex(result)
    try {
      if (shouldReEncrypt) {
        // Ключ шифрования ОС сменился: перезаписываем сохранённый ключ.
        await saveRememberedKey(key)
      }
      return key
    } catch {
      key.fill(0)
      return null
    }
  } catch {
    return null
  }
}

/** Сохраняет raw-ключ в safeStorage. Пароль сюда не попадает. */
export async function saveRememberedKey(key: Buffer): Promise<void> {
  if (!(await keyStorageAvailable())) {
    throw new Error(
      'системное хранилище ключей недоступно; сохранение отключено',
    )
  }
  if (key.length !== 32) {
    throw new Error('ключ: ожидалось 32 байта')
  }
  const encrypted = await safeStorage.encryptStringAsync(key.toString('hex'))
  const path = keyFilePath()
  await mkdir(dirname(path), { recursive: true })
  const temp = `${path}.${process.pid}.${Date.now()}.tmp`
  try {
    await writeFile(temp, encrypted, { mode: 0o600 })
    try {
      await rename(temp, path)
    } catch (error) {
      // На Windows rename поверх существующего файла не работает.
      // Уводим старый blob в .old и восстанавливаем его при ошибке, чтобы
      // неудачная запись не уничтожила сохранённый ключ.
      const backup = `${path}.old`
      await rm(backup, { force: true })
      try {
        await rename(path, backup)
      } catch {
        throw error
      }
      try {
        await rename(temp, path)
      } catch (renameError) {
        await rename(backup, path)
        throw renameError
      }
      await rm(backup, { force: true })
    }
  } finally {
    await rm(temp, { force: true })
  }
}

/** Удаляет сохранённый ключ. */
export async function forgetRememberedKey(): Promise<void> {
  await rm(keyFilePath(), { force: true })
}

/** Собирает состояние хранилища для экрана пароля. */
export async function keyStorageStatus(): Promise<KeyStorageStatus> {
  const available = await keyStorageAvailable()
  const key = await loadRememberedKey()
  const remembered = key !== null
  key?.fill(0)
  return {
    available,
    remembered,
    backend: backendName(),
  }
}
