import { mkdtemp, readFile, rm } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { describe, expect, it } from 'vitest'
import { migratePlaintextBook } from '../src/main/book-migration'

const fakeCtl = join(
  process.cwd(),
  'tests',
  'fixtures',
  'fake-asledgerctl-migrate.mjs',
)

async function tempDir(): Promise<string> {
  return mkdtemp(join(tmpdir(), 'asledger-book-migration-'))
}

describe('шифрование старой книги через asledgerctl', () => {
  it('передаёт пароль в stdin и не светит его в argv/env', async () => {
    const dir = await tempDir()
    try {
      const output = join(dir, 'result.json')
      const result = await migratePlaintextBook({
        command: process.execPath,
        commandArgs: [fakeCtl],
        dbPath: join(dir, 'book.db'),
        password: 'старый-секрет',
        env: {
          ...process.env,
          FAKE_ASLEDGERCTL_OUT: output,
          ASLEDGER_PASSWORD_FILE: '/tmp/другой-секрет',
          asledger_password_file: '/tmp/ещё-секрет',
        },
      })

      expect(result.backupPath).toBe('/tmp/fake-encrypted-backup.db')
      const payload = JSON.parse(await readFile(output, 'utf8')) as {
        args: string[]
        env: Record<string, string>
        password: string
      }
      expect(payload.password).toBe('старый-секрет')
      expect(payload.args).toContain('migrate-encrypt')
      expect(payload.args).toContain('-password-fd')
      expect(payload.args).toContain('0')
      expect(payload.args.join(' ')).not.toContain('старый-секрет')
      expect(payload.env['ASLEDGER_PASSWORD_FILE']).toBeUndefined()
      expect(payload.env['asledger_password_file']).toBeUndefined()
    } finally {
      await rm(dir, { recursive: true, force: true })
    }
  })

  it('возвращает понятную ошибку и вычищает пароль из вывода', async () => {
    const dir = await tempDir()
    try {
      let failure: Error | null = null
      try {
        await migratePlaintextBook({
          command: process.execPath,
          commandArgs: [fakeCtl],
          dbPath: join(dir, 'book.db'),
          password: 'старый-секрет',
          env: {
            ...process.env,
            FAKE_ASLEDGERCTL_FAIL: '1',
          },
        })
      } catch (error) {
        failure = error instanceof Error ? error : new Error(String(error))
      }
      expect(failure?.message).toMatch(/Не удалось зашифровать книгу/)
      expect(failure?.message).not.toContain('старый-секрет')
    } finally {
      await rm(dir, { recursive: true, force: true })
    }
  })

  it('объясняет отсутствие asledgerctl', async () => {
    const dir = await tempDir()
    try {
      await expect(
        migratePlaintextBook({
          command: join(dir, 'no-such-asledgerctl'),
          dbPath: join(dir, 'book.db'),
          password: 'секрет',
        }),
      ).rejects.toThrow(/Не найден asledgerctl/)
    } finally {
      await rm(dir, { recursive: true, force: true })
    }
  })

  it('не запускает команду с пустым паролем', async () => {
    await expect(
      migratePlaintextBook({
        command: process.execPath,
        commandArgs: [fakeCtl],
        dbPath: '/tmp/book.db',
        password: '',
      }),
    ).rejects.toThrow(/Введите пароль книги/)
  })
})
