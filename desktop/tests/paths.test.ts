import { resolve, join } from 'node:path'
import { describe, expect, it } from 'vitest'
import {
  apiUrl,
  backendBinaryName,
  brandIconPath,
  buildBackendArgs,
  ctlBinaryName,
  healthUrl,
  resolveDesktopPaths,
  webUrl,
} from '../src/main/paths'

describe('пути desktop-клиента', () => {
  it('в dev-режиме берёт бинарь и ресурсы из корня репозитория', () => {
    const appPath = join('/repo', 'desktop')
    const userDataPath = join('/home', 'user', '.config', 'asledger')
    const paths = resolveDesktopPaths({
      isPackaged: false,
      appPath,
      resourcesPath: join('/unused'),
      userDataPath,
      env: {},
      platform: 'linux',
    })

    expect(paths.backendBinary).toBe(join(appPath, '..', 'bin', 'asledgerd'))
    expect(paths.ctlBinary).toBe(join(appPath, '..', 'bin', 'asledgerctl'))
    expect(paths.assetsDir).toBe(join(appPath, '..', 'assets', 'logo'))
    expect(
      brandIconPath({
        isPackaged: false,
        appPath,
        resourcesPath: join('/unused'),
      }),
    ).toBe(join(appPath, '..', 'assets', 'logo', 'icon-512.png'))
    expect(paths.chartsDir).toBe(join(appPath, '..', 'templates', 'charts'))
    expect(paths.dbPath).toBe(join(userDataPath, 'data', 'asledger.db'))
    expect(paths.logFile).toBe(join(userDataPath, 'logs', 'asledgerd.log'))
  })

  it('в упакованном приложении использует resources и .exe на Windows', () => {
    const resourcesPath = join('/opt', 'asledger', 'resources')
    const paths = resolveDesktopPaths({
      isPackaged: true,
      appPath: join(resourcesPath, 'app.asar'),
      resourcesPath,
      userDataPath: join('/home', 'user', '.config', 'asledger'),
      env: {},
      platform: 'win32',
    })

    expect(paths.backendBinary).toBe(
      join(resourcesPath, 'bin', 'asledgerd.exe'),
    )
    expect(paths.ctlBinary).toBe(
      join(resourcesPath, 'bin', 'asledgerctl.exe'),
    )
    expect(paths.assetsDir).toBe(join(resourcesPath, 'assets', 'logo'))
    expect(
      brandIconPath({
        isPackaged: true,
        appPath: join(resourcesPath, 'app.asar'),
        resourcesPath,
      }),
    ).toBe(join(resourcesPath, 'assets', 'logo', 'icon-512.png'))
    expect(paths.chartsDir).toBe(join(resourcesPath, 'templates', 'charts'))
  })

  it('учитывает переопределения путей из окружения', () => {
    const paths = resolveDesktopPaths({
      isPackaged: false,
      appPath: join('/repo', 'desktop'),
      resourcesPath: join('/unused'),
      userDataPath: join('/home', 'user', '.config', 'asledger'),
      env: {
        ASLEDGERD_BIN: '/tmp/asledgerd-test',
        ASLEDGERCTL_BIN: '/tmp/asledgerctl-test',
        ASLEDGER_DESKTOP_DATA_DIR: '/tmp/asledger-data',
        ASLEDGER_DESKTOP_LOG_DIR: '/tmp/asledger-logs',
      },
      platform: 'linux',
    })

    expect(paths.backendBinary).toBe(resolve('/tmp/asledgerd-test'))
    expect(paths.ctlBinary).toBe(resolve('/tmp/asledgerctl-test'))
    expect(paths.dataDir).toBe(resolve('/tmp/asledger-data'))
    expect(paths.logDir).toBe(resolve('/tmp/asledger-logs'))
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
        ctlBinary: '/bin/asledgerctl',
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
    expect(ctlBinaryName('linux')).toBe('asledgerctl')
    expect(ctlBinaryName('win32')).toBe('asledgerctl.exe')
    expect(webUrl(9000)).toBe('http://127.0.0.1:9000')
    expect(apiUrl(9001)).toBe('http://127.0.0.1:9001')
    expect(healthUrl(9001)).toBe('http://127.0.0.1:9001/api/v1/health')
  })
})
