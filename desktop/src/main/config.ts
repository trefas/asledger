import {
  access,
  mkdir,
  readFile,
  rename,
  stat,
  writeFile,
} from 'node:fs/promises'
import { constants } from 'node:fs'
import { dirname, isAbsolute, join, resolve } from 'node:path'
import { DEFAULT_API_PORT, DEFAULT_WEB_PORT } from './paths'

/** Содержимое desktop-конфига. Секретов здесь быть не должно. */
export interface DesktopConfigFile {
  web_port: number
  api_port: number
  data_path: string
  log_path: string
}

/** Конфиг с разрешёнными абсолютными путями. */
export interface DesktopConfig {
  configPath: string
  configDir: string
  portable: boolean
  webPort: number
  apiPort: number
  dataDir: string
  logDir: string
}

export interface ConfigOptions {
  appDir: string
  userDataPath: string
  env?: NodeJS.ProcessEnv
  isWritableDir?: (dir: string) => Promise<boolean>
}

function errorText(error: unknown): string {
  return error instanceof Error ? error.message : String(error)
}

function defaultConfig(): DesktopConfigFile {
  return {
    web_port: DEFAULT_WEB_PORT,
    api_port: DEFAULT_API_PORT,
    data_path: 'data',
    log_path: 'logs',
  }
}

function parsePort(value: unknown, name: string, fallback: number): number {
  if (value === undefined) {
    return fallback
  }
  if (typeof value !== 'number' || !Number.isInteger(value)) {
    throw new Error(`config: ${name} должен быть целым числом`)
  }
  if (value < 1 || value > 65535) {
    throw new Error(`config: ${name} должен быть в диапазоне 1..65535`)
  }
  return value
}

function parsePath(value: unknown, name: string, fallback: string): string {
  if (value === undefined) {
    return fallback
  }
  if (typeof value !== 'string' || value.trim() === '') {
    throw new Error(`config: ${name} должен быть непустой строкой`)
  }
  return value.trim()
}

/** Проверяет JSON-объект конфига и подставляет значения по умолчанию. */
export function validateConfig(raw: unknown): DesktopConfigFile {
  if (typeof raw !== 'object' || raw === null || Array.isArray(raw)) {
    throw new Error('config: корень файла должен быть JSON-объектом')
  }
  const record = raw as Record<string, unknown>
  const webPort = parsePort(record['web_port'], 'web_port', DEFAULT_WEB_PORT)
  const apiPort = parsePort(record['api_port'], 'api_port', DEFAULT_API_PORT)
  if (webPort === apiPort) {
    throw new Error('config: web_port и api_port должны различаться')
  }
  return {
    web_port: webPort,
    api_port: apiPort,
    data_path: parsePath(record['data_path'], 'data_path', 'data'),
    log_path: parsePath(record['log_path'], 'log_path', 'logs'),
  }
}

/** Превращает файл конфига в конфиг с абсолютными путями. */
export function resolveConfig(
  configPath: string,
  raw: unknown,
  portable: boolean,
): DesktopConfig {
  const file = validateConfig(raw)
  const configDir = dirname(configPath)
  return {
    configPath,
    configDir,
    portable,
    webPort: file.web_port,
    apiPort: file.api_port,
    dataDir: isAbsolute(file.data_path)
      ? file.data_path
      : resolve(configDir, file.data_path),
    logDir: isAbsolute(file.log_path)
      ? file.log_path
      : resolve(configDir, file.log_path),
  }
}

async function isWritableDir(dir: string): Promise<boolean> {
  try {
    await mkdir(dir, { recursive: true })
    await access(dir, constants.W_OK)
    return true
  } catch {
    return false
  }
}

async function isWritableFile(path: string): Promise<boolean> {
  try {
    await access(path, constants.W_OK)
    return true
  } catch {
    return false
  }
}

async function fileExists(path: string): Promise<boolean> {
  try {
    await stat(path)
    return true
  } catch {
    return false
  }
}

/**
 * Выбирает место конфига.
 *
 * Приоритет: ASLEDGER_DESKTOP_CONFIG → portable `config.json` рядом с
 * приложением (если каталог и файл доступны для записи) → userData.
 */
export async function chooseConfigPath(options: ConfigOptions): Promise<{
  configPath: string
  portable: boolean
}> {
  const env = options.env ?? process.env
  const override = env['ASLEDGER_DESKTOP_CONFIG']?.trim()
  if (override) {
    return { configPath: resolve(override), portable: false }
  }

  const portablePath = join(options.appDir, 'config.json')
  const writableDir = await (options.isWritableDir ?? isWritableDir)(
    options.appDir,
  )
  if (writableDir) {
    const exists = await fileExists(portablePath)
    if (!exists || (await isWritableFile(portablePath))) {
      return { configPath: portablePath, portable: true }
    }
  }

  return {
    configPath: join(options.userDataPath, 'config.json'),
    portable: false,
  }
}

async function writeConfigAtomic(
  configPath: string,
  config: DesktopConfigFile,
): Promise<void> {
  await mkdir(dirname(configPath), { recursive: true })
  const tempPath = `${configPath}.${process.pid}.${Date.now()}.tmp`
  await writeFile(tempPath, `${JSON.stringify(config, null, 2)}\n`, {
    mode: 0o600,
  })
  await rename(tempPath, configPath)
}

/**
 * Читает конфиг или создаёт его при первом запуске.
 *
 * Ошибки разбора и валидации не заменяются молча значениями по умолчанию:
 * иначе пользователь не узнает, что приложение слушает не те порты.
 */
export async function loadOrCreateConfig(
  options: ConfigOptions,
): Promise<DesktopConfig> {
  const { configPath, portable } = await chooseConfigPath(options)
  let raw: string
  try {
    raw = await readFile(configPath, 'utf8')
  } catch (error) {
    const code = (error as NodeJS.ErrnoException).code
    if (code !== 'ENOENT') {
      throw new Error(`config: не удалось прочитать ${configPath}: ${errorText(error)}`)
    }
    const created = defaultConfig()
    await writeConfigAtomic(configPath, created)
    return resolveConfig(configPath, created, portable)
  }

  let parsed: unknown
  try {
    parsed = JSON.parse(raw)
  } catch (error) {
    throw new Error(`config: не удалось разобрать ${configPath}: ${errorText(error)}`)
  }
  return resolveConfig(configPath, parsed, portable)
}
