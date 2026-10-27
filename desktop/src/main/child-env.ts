/**
 * Убирает из окружения переменные ASLEDGER_*, которые могли бы подсунуть
 * другой путь, пароль или ключ дочернему asledgerd/asledgerctl.
 */
export function safeChildEnv(env: NodeJS.ProcessEnv): NodeJS.ProcessEnv {
  const copy = { ...env }
  for (const key of Object.keys(copy)) {
    // Windows сравнивает имена переменных без учёта регистра.
    if (key.toUpperCase().startsWith('ASLEDGER_')) {
      delete copy[key]
    }
  }
  return copy
}
