import { describe, expect, it } from 'vitest'
import {
  statusLabel,
  statusTone,
  type AppStatus,
  type StatusTone,
} from '../src/shared/status'

const cases: ReadonlyArray<readonly [AppStatus, string, StatusTone]> = [
  ['scaffold', 'Каркас готов', 'neutral'],
  ['starting', 'Запуск…', 'muted'],
  ['ready', 'Готово', 'ok'],
  ['stopping', 'Остановка…', 'muted'],
  ['error', 'Ошибка', 'danger'],
  ['stopped', 'Остановлен', 'muted'],
]

describe('статусы приложения', () => {
  it.each(cases)('для %s возвращает подпись и тон', (status, label, tone) => {
    expect(statusLabel(status)).toBe(label)
    expect(statusTone(status)).toBe(tone)
  })
})
