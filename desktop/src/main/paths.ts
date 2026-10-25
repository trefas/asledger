import { join, resolve } from 'node:path'

/** Порты по умолчанию, совпадающие с asledgerd. */
export const DEFAULT_WEB_PORT = 8777
export const DEFAULT_API_PORT = 8778

/** Пути, которые нужны desktop-клиенту для запуска сервера. */
export interface DesktopPaths {
  dataDir: string
  logDir: string
  logFile: string
  dbPath: string
  assetsDir: string
  chartsDir: string
  backendBinary: string
  ctlBinary: string
}

export interface PathOptions {
  isPackaged: boolean
  appPath: string
  resourcesPath: string
  userDataPath: string
  dataDir?: string
  logDir?: string
  env?: NodeJS.ProcessEnv
  platform?: NodeJS.Platform
}

function envPath(env: NodeJS.ProcessEnv, key: string): string | null {
  const value = env[key]?.trim()
  return value ? resolve(value) : null
}

/** Возвращает имя бинаря asledgerd для платформы. */
export function backendBinaryName(platform: NodeJS.Platform): string {
  return platform === 'win32' ? 'asledgerd.exe' : 'asledgerd'
}

/** Возвращает имя утилиты asledgerctl для платформы. */
export function ctlBinaryName(platform: NodeJS.Platform): string {
  return platform === 'win32' ? 'asledgerctl.exe' : 'asledgerctl'
}

/**
 * Собирает пути desktop-клиента.
 *
 * В dev-режиме бинарь и ресурсы берутся из корня репозитория, в упакованном —
 * из resources. Переменные ASLEDGER_DESKTOP_DATA_DIR, ASLEDGER_DESKTOP_LOG_DIR
 * и ASLEDGERD_BIN позволяют изолировать данные и подменить бинарь в тестах.
 */
export function resolveDesktopPaths(options: PathOptions): DesktopPaths {
  const env = options.env ?? process.env
  const platform = options.platform ?? process.platform
  const dataDir =
    envPath(env, 'ASLEDGER_DESKTOP_DATA_DIR') ??
    options.dataDir ??
    join(options.userDataPath, 'data')
  const logDir =
    envPath(env, 'ASLEDGER_DESKTOP_LOG_DIR') ??
    options.logDir ??
    join(options.userDataPath, 'logs')
  const backendBinary =
    envPath(env, 'ASLEDGERD_BIN') ??
    (options.isPackaged
      ? join(options.resourcesPath, 'bin', backendBinaryName(platform))
      : join(options.appPath, '..', 'bin', backendBinaryName(platform)))
  const ctlBinary =
    envPath(env, 'ASLEDGERCTL_BIN') ??
    (options.isPackaged
      ? join(options.resourcesPath, 'bin', ctlBinaryName(platform))
      : join(options.appPath, '..', 'bin', ctlBinaryName(platform)))
  const assetsDir = options.isPackaged
    ? join(options.resourcesPath, 'assets', 'logo')
    : join(options.appPath, '..', 'assets', 'logo')
  const chartsDir = options.isPackaged
    ? join(options.resourcesPath, 'templates', 'charts')
    : join(options.appPath, '..', 'templates', 'charts')

  return {
    dataDir,
    logDir,
    logFile: join(logDir, 'asledgerd.log'),
    dbPath: join(dataDir, 'asledger.db'),
    assetsDir,
    chartsDir,
    backendBinary,
    ctlBinary,
  }
}

/** Аргументы запуска asledgerd. Сервер слушает все интерфейсы. */
export function buildBackendArgs(
  paths: DesktopPaths,
  webPort = DEFAULT_WEB_PORT,
  apiPort = DEFAULT_API_PORT,
): string[] {
  return [
    '-db',
    paths.dbPath,
    '-assets',
    paths.assetsDir,
    '-charts-dir',
    paths.chartsDir,
    '-web-addr',
    `:${webPort}`,
    '-api-addr',
    `:${apiPort}`,
    '-log-level',
    'info',
    '-log-format',
    'text',
  ]
}

/** URL web-интерфейса для окна книги. */
export function webUrl(webPort = DEFAULT_WEB_PORT): string {
  return `http://127.0.0.1:${webPort}`
}

/** URL REST API. */
export function apiUrl(apiPort = DEFAULT_API_PORT): string {
  return `http://127.0.0.1:${apiPort}`
}

/** URL healthcheck REST API. */
export function healthUrl(apiPort = DEFAULT_API_PORT): string {
  return `${apiUrl(apiPort)}/api/v1/health`
}
