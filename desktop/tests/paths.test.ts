import { describe, expect, it } from 'vitest'
import {
  apiUrl,
  backendBinaryName,
  buildBackendArgs,
  healthUrl,
  resolveDesktopPaths,
  webUrl,
} from '../src/main/paths'

describe('пути desktop-клиента', () => {
  it('в dev-режиме берёт бинарь и ресурсы из корня репозитория', () => {
    const paths = resolveDesktopPaths({
      isPackaged: false,
      appPath: '/repo/desktop',
      resourcesPath: '/unused',
      userDataPath: '/home/user/.config/asledger',
      env: {},
      platform: 'linux',
    })

    expect(paths.backendBinary).toBe('/repo/bin/asledgerd')
    expect(paths.assetsDir).toBe('/repo/assets/logo')
    expect(paths.chartsDir).toBe('/repo/templates/charts')
    expect(paths.dbPath).toBe(
      '/home/user/.config/asledger/data/asledger.db',
    )
    expect(paths.logFile).toBe(
      '/home/user/.config/asledger/logs/asledgerd.log',
    )
  })

  it('в упакованном приложении использует resources и .exe на Windows', () => {
    const paths = resolveDesktopPaths({
      isPackaged: true,
      appPath: '/opt/asledger/resources/app.asar',
      resourcesPath: '/opt/asledger/resources',
      userDataPath: '/home/user/.config/asledger',
      env: {},
      platform: 'win32',
    })

    expect(paths.backendBinary).toBe(
      '/opt/asledger/resources/bin/asledgerd.exe',
    )
    expect(paths.assetsDir).toBe('/opt/asledger/resources/assets/logo')
    expect(paths.chartsDir).toBe(
      '/opt/asledger/resources/templates/charts',
    )
  })

  it('учитывает переопределения путей из окружения', () => {
    const paths = resolveDesktopPaths({
      isPackaged: false,
      appPath: '/repo/desktop',
      resourcesPath: '/unused',
      userDataPath: '/home/user/.config/asledger',
      env: {
        ASLEDGERD_BIN: '/tmp/asledgerd-test',
        ASLEDGER_DESKTOP_DATA_DIR: '/tmp/asledger-data',
        ASLEDGER_DESKTOP_LOG_DIR: '/tmp/asledger-logs',
      },
      platform: 'linux',
    })

    expect(paths.backendBinary).toBe('/tmp/asledgerd-test')
    expect(paths.dataDir).toBe('/tmp/asledger-data')
    expect(paths.logDir).toBe('/tmp/asledger-logs')
  })

  it('собирает аргументы asledgerd с адресами на все интерфейсы', () => {
    const args = buildBackendArgs(
      {
        dataDir: '/data',
        logDir: '/logs',
        logFile: '/logs/asledgerd.log',
        dbPath: '/data/asledger.db',
        assetsDir: '/assets',
        chartsDir: '/charts',
        backendBinary: '/bin/asledgerd',
      },
      8777,
      8778,
    )

    expect(args).toContain('-web-addr')
    expect(args).toContain(':8777')
    expect(args).toContain('-api-addr')
    expect(args).toContain(':8778')
    expect(args).toContain('/data/asledger.db')
  })

  it('строит URL и имя бинаря', () => {
    expect(backendBinaryName('linux')).toBe('asledgerd')
    expect(backendBinaryName('win32')).toBe('asledgerd.exe')
    expect(webUrl(9000)).toBe('http://127.0.0.1:9000')
    expect(apiUrl(9001)).toBe('http://127.0.0.1:9001')
    expect(healthUrl(9001)).toBe('http://127.0.0.1:9001/api/v1/health')
  })
})
