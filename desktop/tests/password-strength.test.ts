import { describe, expect, it } from 'vitest'
import { passwordStrength } from '../src/renderer/src/password-strength'

describe('оценка сложности пароля', () => {
  it('считает короткий пароль низким', () => {
    expect(passwordStrength('').level).toBe('low')
    expect(passwordStrength('1234567').level).toBe('low')
  })

  it('считает длинный пароль без разных классов символов средним', () => {
    expect(passwordStrength('длинныйпарольбезцифр').level).toBe('medium')
  })

  it('считает длинный пароль с разными классами символов высоким', () => {
    const result = passwordStrength('Asledger-2026-Надёжный!')
    expect(result.level).toBe('high')
    expect(result.label).toBe('высокая')
  })

  it('распознаёт кириллицу и спецсимволы', () => {
    expect(passwordStrength('Пароль-2026-Длинный').level).toBe('high')
  })
})
