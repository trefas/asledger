import { mkdtemp, readFile, rm, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { describe, expect, it } from 'vitest'
import {
  chooseConfigPath,
  loadOrCreateConfig,
  validateConfig,
} from '../src/main/config'

async function tempDir(): Promise<string> {
  return mkdtemp(join(tmpdir(), 'asledger-config-test-'))
}

describe('конфигурация desktop-клиента', () => {
  it('создаёт portable config.json при первом запуске', async () => {
    const dir = await tempDir()
    try {
      const config = await loadOrCreateConfig({
        appDir: dir,
        userDataPath: join(dir, 'user-data'),
        env: {},
      })

      expect(config.portable).toBe(true)
      expect(config.configPath).toBe(join(dir, 'config.json'))
      expect(config.webPort).toBe(8777)
      expect(config.apiPort).toBe(8778)
      expect(config.dataDir).toBe(join(dir, 'data'))
      expect(config.logDir).toBe(join(dir, 'logs'))

      const raw = JSON.parse(
        await readFile(join(dir, 'config.json'), 'utf8'),
      ) as Record<string, unknown>
      expect(raw['web_port']).toBe(8777)
      expect(raw['api_port']).toBe(8778)
      expect(raw['data_path']).toBe('data')
      expect(raw['log_path']).toBe('logs')
    } finally {
      await rm(dir, { recursive: true, force: true })
    }
  })

  it('читает существующий конфиг и разрешает относительные пути', async () => {
    const dir = await tempDir()
    try {
      await writeFile(
        join(dir, 'config.json'),
        JSON.stringify({
          web_port: 9000,
          api_port: 9001,
          data_path: 'книга',
          log_path: 'журналы',
        }),
      )

      const config = await loadOrCreateConfig({
        appDir: dir,
        userDataPath: join(dir, 'user-data'),
        env: {},
      })

      expect(config.webPort).toBe(9000)
      expect(config.apiPort).toBe(9001)
      expect(config.dataDir).toBe(join(dir, 'книга'))
      expect(config.logDir).toBe(join(dir, 'журналы'))
    } finally {
      await rm(dir, { recursive: true, force: true })
    }
  })

  it('уходит в userData, если каталог приложения недоступен для записи', async () => {
    const dir = await tempDir()
    try {
      const userDataPath = join(dir, 'user-data')
      const location = await chooseConfigPath({
        appDir: dir,
        userDataPath,
        env: {},
        isWritableDir: async () => false,
      })

      expect(location.portable).toBe(false)
      expect(location.configPath).toBe(join(userDataPath, 'config.json'))
    } finally {
      await rm(dir, { recursive: true, force: true })
    }
  })

  it('поддерживает ASLEDGER_DESKTOP_CONFIG', async () => {
    const dir = await tempDir()
    try {
      const custom = join(dir, 'custom.json')
      const config = await loadOrCreateConfig({
        appDir: dir,
        userDataPath: join(dir, 'user-data'),
        env: { ASLEDGER_DESKTOP_CONFIG: custom },
      })

      expect(config.configPath).toBe(custom)
      expect(config.portable).toBe(false)
    } finally {
      await rm(dir, { recursive: true, force: true })
    }
  })

  it('не подменяет неверный конфиг значениями по умолчанию', async () => {
    const dir = await tempDir()
    try {
      await writeFile(
        join(dir, 'config.json'),
        JSON.stringify({ web_port: 0, api_port: 8778 }),
      )

      await expect(
        loadOrCreateConfig({
          appDir: dir,
          userDataPath: join(dir, 'user-data'),
          env: {},
        }),
      ).rejects.toThrow(/web_port/)
    } finally {
      await rm(dir, { recursive: true, force: true })
    }
  })

  it('проверяет диапазон портов и их различие', () => {
    expect(() => validateConfig({ web_port: 1, api_port: 1 })).toThrow(
      /различаться/,
    )
    expect(() => validateConfig({ web_port: 70000, api_port: 8778 })).toThrow(
      /диапазоне/,
    )
    expect(() => validateConfig({ data_path: '' })).toThrow(/data_path/)
  })

  it('работает с пробелами и кириллицей в пути', async () => {
    const base = await tempDir()
    const dir = join(base, 'папка с пробелами')
    try {
      const config = await loadOrCreateConfig({
        appDir: dir,
        userDataPath: join(dir, 'user-data'),
        env: {},
      })
      expect(config.configPath).toBe(join(dir, 'config.json'))
    } finally {
      await rm(base, { recursive: true, force: true })
    }
  })
})
