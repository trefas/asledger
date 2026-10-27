import { describe, expect, it } from 'vitest'
import { safeChildEnv } from '../src/main/child-env'

describe('окружение дочерних процессов', () => {
  it('удаляет ASLEDGER_* без учёта регистра и сохраняет остальные', () => {
    const env = safeChildEnv({
      PATH: '/bin',
      ASLEDGER_PASSWORD_FILE: '/tmp/password',
      asledger_key_file: '/tmp/key',
      ASLEDGER_DB_PATH: '/tmp/book.db',
      ASLEDGERD_BIN: '/tmp/asledgerd',
    })

    expect(env).toEqual({
      PATH: '/bin',
      ASLEDGERD_BIN: '/tmp/asledgerd',
    })
  })
})
