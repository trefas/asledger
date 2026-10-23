import { mkdtemp, rm, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { afterEach, describe, expect, it } from 'vitest'
import { maskSecrets, readLogTail } from '../src/main/log'

const dirs: string[] = []

async function tempLog(content: string): Promise<string> {
  const dir = await mkdtemp(join(tmpdir(), 'asledger-log-'))
  dirs.push(dir)
  const path = join(dir, 'asledgerd.log')
  await writeFile(path, content)
  return path
}

afterEach(async () => {
  await Promise.all(
    dirs.splice(0).map((dir) => rm(dir, { recursive: true, force: true })),
  )
})

describe('maskSecrets', () => {
  it('маскирует пароли, токены и Bearer', () => {
    const masked = maskSecrets(
      'level=INFO password=hunter2 token=abc123 Authorization: Bearer secret.jwt',
    )

    expect(masked).not.toContain('hunter2')
    expect(masked).not.toContain('abc123')
    expect(masked).not.toContain('secret.jwt')
    expect(masked).toContain('password=***')
  })

  it('маскирует значения JSON-полей', () => {
    const masked = maskSecrets('{"token":"abc","password":"x"}')

    expect(masked).not.toContain('abc')
    expect(masked).not.toContain('"x"')
    expect(masked).toBe('{"token":"***","password":"***"}')
  })

  it('не меняет обычные строки', () => {
    const line = 'level=INFO msg="сервер запущен"'

    expect(maskSecrets(line)).toBe(line)
  })
})

describe('readLogTail', () => {
  it('возвращает только последние строки', async () => {
    const path = await tempLog('one\ntwo\nthree\nfour\n')

    expect(await readLogTail(path, 2)).toEqual(['three', 'four'])
  })

  it('читает хвост с конца файла', async () => {
    const path = await tempLog('aaaa\nbbbb\ncccc\n')

    expect(await readLogTail(path, 10, 5)).toEqual(['cccc'])
  })

  it('возвращает пустой массив для отсутствующего файла', async () => {
    expect(await readLogTail(join(tmpdir(), 'asledger-no-such.log'))).toEqual([])
  })

  it('маскирует старые записи при чтении', async () => {
    const path = await tempLog('password=old\nnormal\n')

    expect(await readLogTail(path)).toEqual(['password=***', 'normal'])
  })
})
