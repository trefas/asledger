/** Оценка сложности пароля для индикатора первого запуска. */
export type PasswordStrengthLevel = 'low' | 'medium' | 'high'

export interface PasswordStrength {
  level: PasswordStrengthLevel
  label: string
}

/**
 * Грубая эвристика для подсказки пользователю.
 *
 * Это не проверка стойкости: окончательное решение остаётся за пользователем.
 * Пароль не покидает renderer до отправки в main.
 */
export function passwordStrength(password: string): PasswordStrength {
  if (password.length < 8) {
    return { level: 'low', label: 'низкая' }
  }

  let score = 0
  if (password.length >= 12) {
    score += 1
  }
  if (password.length >= 16) {
    score += 1
  }
  if (/[a-zа-яё]/.test(password)) {
    score += 1
  }
  if (/[A-ZА-ЯЁ]/.test(password)) {
    score += 1
  }
  if (/\d/.test(password)) {
    score += 1
  }
  if (/[^A-Za-zА-Яа-яЁё0-9]/.test(password)) {
    score += 1
  }

  if (score >= 5) {
    return { level: 'high', label: 'высокая' }
  }
  if (score >= 3) {
    return { level: 'medium', label: 'средняя' }
  }
  return { level: 'low', label: 'низкая' }
}
