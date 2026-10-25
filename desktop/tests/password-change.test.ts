import { existsSync } from 'node:fs'
import { mkdtemp, readFile, rm } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { describe, expect, it } from 'vitest'
import { changeBookPassword } from '../src/main/password-change'

const fakeCtl = join(process.cwd(), 'tests', 'fixtures', 'fake-asledgerctl.mjs')

async function tempDir(): Promise<string> {
  return mkdtemp(join(tmpdir(), 'asledger-password-change-'))
}

describe('смена пароля через asledgerctl', () => {
  it.skipIf(process.platform === 'win32')(
    'передаёт старый пароль в stdin, новый в fd 3 и не светит их в argv/env',
    async () => {
      const dir = await tempDir()
      try {
        const output = join(dir, 'result.json')
        const result = await changeBookPassword({
          command: process.execPath,
          commandArgs: [fakeCtl],
          dbPath: join(dir, 'book.db'),
          currentPassword: 'старый-секрет',
          newPassword: 'новый-секрет',
          backupPath: join(dir, 'backup.db'),
          env: {
            ...process.env,
            FAKE_ASLEDGERCTL_OUT: output,
            ASLEDGER_PASSWORD_FILE: '/tmp/другой-секрет',
            asledger_password_file: '/tmp/ещё-секрет',
          },
        })

        expect(result.backupPath).toBe(join(dir, 'backup.db'))
        const payload = JSON.parse(await readFile(output, 'utf8')) as {
          args: string[]
          env: Record<string, string>
          currentPassword: string
          newPassword: string
        }
        expect(payload.currentPassword).toBe('старый-секрет')
        expect(payload.newPassword).toBe('новый-секрет')
        expect(payload.args).toContain('-password-fd')
        expect(payload.args).toContain('0')
        expect(payload.args).toContain('-new-password-fd')
        expect(payload.args).toContain('3')
        expect(payload.args.join(' ')).not.toContain('старый-секрет')
        expect(payload.args.join(' ')).not.toContain('новый-секрет')
        expect(payload.env['ASLEDGER_PASSWORD_FILE']).toBeUndefined()
        expect(payload.env['asledger_password_file']).toBeUndefined()
      } finally {
        await rm(dir, { recursive: true, force: true })
      }
    },
  )

  it('на Windows передаёт новый пароль через временный файл', async () => {
    const dir = await tempDir()
    try {
      const output = join(dir, 'result.json')
      await changeBookPassword({
        command: process.execPath,
        commandArgs: [fakeCtl],
        dbPath: join(dir, 'book.db'),
        currentPassword: 'старый-секрет',
        newPassword: 'новый-секрет',
        backupPath: join(dir, 'backup.db'),
        platform: 'win32',
        env: {
          ...process.env,
          FAKE_ASLEDGERCTL_OUT: output,
        },
      })

      const payload = JSON.parse(await readFile(output, 'utf8')) as {
        args: string[]
        currentPassword: string
        newPassword: string
      }
      const fileIndex = payload.args.indexOf('-new-password-file')
      expect(fileIndex).toBeGreaterThanOrEqual(0)
      const passwordFile = payload.args[fileIndex + 1] as string
      expect(payload.currentPassword).toBe('старый-секрет')
      expect(payload.newPassword).toBe('новый-секрет')
      expect(payload.args.join(' ')).not.toContain('старый-секрет')
      expect(payload.args.join(' ')).not.toContain('новый-секрет')
      // Временный каталог с новым паролем удаляется после завершения.
      expect(existsSync(passwordFile)).toBe(false)
    } finally {
      await rm(dir, { recursive: true, force: true })
    }
  })

  it('возвращает понятную ошибку и вычищает секреты из вывода', async () => {
    const dir = await tempDir()
    try {
      let failure: Error | null = null
      try {
        await changeBookPassword({
          command: process.execPath,
          commandArgs: [fakeCtl],
          dbPath: join(dir, 'book.db'),
          currentPassword: 'старый-секрет',
          newPassword: 'новый-секрет',
          backupPath: join(dir, 'backup.db'),
          env: {
            ...process.env,
            FAKE_ASLEDGERCTL_FAIL: '1',
          },
        })
      } catch (error) {
        failure = error instanceof Error ? error : new Error(String(error))
      }
      expect(failure?.message).toMatch(/Не удалось сменить пароль/)
      expect(failure?.message).not.toMatch(/старый-секрет|новый-секрет/)
    } finally {
      await rm(dir, { recursive: true, force: true })
    }
  })

  it('объясняет отсутствие asledgerctl', async () => {
    const dir = await tempDir()
    try {
      await expect(
        changeBookPassword({
          command: join(dir, 'no-such-asledgerctl'),
          dbPath: join(dir, 'book.db'),
          currentPassword: 'старый',
          newPassword: 'новый',
          backupPath: join(dir, 'backup.db'),
        }),
      ).rejects.toThrow(/Не найден asledgerctl/)
    } finally {
      await rm(dir, { recursive: true, force: true })
    }
  })
})
