/** Состояние desktop-приложения. */
export type AppStatus =
  | 'scaffold'
  | 'starting'
  | 'ready'
  | 'stopping'
  | 'error'
  | 'stopped'

/** Визуальный тон статуса: нейтральный, успех, ошибка или приглушённый. */
export type StatusTone = 'neutral' | 'ok' | 'danger' | 'muted'

const LABELS: Record<AppStatus, string> = {
  scaffold: 'Каркас готов',
  starting: 'Запуск…',
  ready: 'Готово',
  stopping: 'Остановка…',
  error: 'Ошибка',
  stopped: 'Остановлен',
}

const TONES: Record<AppStatus, StatusTone> = {
  scaffold: 'neutral',
  starting: 'muted',
  ready: 'ok',
  stopping: 'muted',
  error: 'danger',
  stopped: 'muted',
}

/** Возвращает русскую подпись состояния. */
export function statusLabel(status: AppStatus): string {
  return LABELS[status]
}

/** Возвращает тон оформления состояния. */
export function statusTone(status: AppStatus): StatusTone {
  return TONES[status]
}
