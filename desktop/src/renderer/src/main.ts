import {
  statusLabel,
  statusTone,
  type AppStatus,
} from '../../shared/status'
import type {
  BackendLogEntry,
  BackendStartRequest,
  BackendState,
  KeyStorageStatus,
  NetworkAddress,
} from '../../shared/ipc'
import './style.css'

function requireElement<T extends Element>(selector: string): T {
  const element = document.querySelector<T>(selector)
  if (!element) {
    throw new Error(`Не найден элемент интерфейса: ${selector}`)
  }
  return element
}

const statusElement = requireElement<HTMLSpanElement>('#status')
const detailElement = requireElement<HTMLElement>('#detail')
const webPortElement = requireElement<HTMLElement>('#web-port')
const apiPortElement = requireElement<HTMLElement>('#api-port')
const pidElement = requireElement<HTMLElement>('#pid')
const logPathElement = requireElement<HTMLElement>('#log-path')
const logElement = requireElement<HTMLPreElement>('#log')
const versionElement = requireElement<HTMLElement>('#version')
const platformElement = requireElement<HTMLElement>('#platform')
const electronElement = requireElement<HTMLElement>('#electron')
const addressesElement = requireElement<HTMLUListElement>('#addresses')
const addressesEmptyElement = requireElement<HTMLElement>('#addresses-empty')

const startButton = requireElement<HTMLButtonElement>('#start')
const openBookButton = requireElement<HTMLButtonElement>('#open-book')
const stopButton = requireElement<HTMLButtonElement>('#stop')
const restartButton = requireElement<HTMLButtonElement>('#restart')
const clearLogButton = requireElement<HTMLButtonElement>('#clear-log')
const copyLogButton = requireElement<HTMLButtonElement>('#copy-log')
const openLogButton = requireElement<HTMLButtonElement>('#open-log')
const refreshNetworkButton =
  requireElement<HTMLButtonElement>('#refresh-network')

const passwordDialog =
  requireElement<HTMLDialogElement>('#password-dialog')
const passwordForm = requireElement<HTMLFormElement>('#password-form')
const passwordTitle = requireElement<HTMLElement>('#password-title')
const passwordHint = requireElement<HTMLElement>('#password-hint')
const passwordInput = requireElement<HTMLInputElement>('#password')
const rememberCheckbox = requireElement<HTMLInputElement>('#remember')
const rememberHint = requireElement<HTMLElement>('#remember-hint')
const passwordError = requireElement<HTMLElement>('#password-error')
const passwordSubmitButton =
  requireElement<HTMLButtonElement>('#password-submit')
const passwordCancelButton =
  requireElement<HTMLButtonElement>('#password-cancel')
const forgetKeyButton = requireElement<HTMLButtonElement>('#forget-key')

const logLines: string[] = []
const MAX_LOG_LINES = 200

function setStatus(status: AppStatus): void {
  statusElement.textContent = statusLabel(status)
  statusElement.dataset['tone'] = statusTone(status)
}

function setText(element: HTMLElement, value: string): void {
  element.textContent = value
}

function portFromUrl(url: string): string {
  try {
    return new URL(url).port || '—'
  } catch {
    return '—'
  }
}

function applyState(state: BackendState): void {
  if (state.status === 'ready' && passwordDialog.open) {
    passwordDialog.close()
    setAuthError(null)
  }
  setStatus(state.status)
  setText(detailElement, state.detail)
  setText(webPortElement, portFromUrl(state.webUrl))
  setText(apiPortElement, portFromUrl(state.apiUrl))
  setText(pidElement, state.pid === null ? '—' : String(state.pid))
  setText(logPathElement, state.logPath)

  const busy = state.status === 'starting' || state.status === 'stopping'
  startButton.disabled = busy || state.status === 'ready'
  openBookButton.disabled = state.status !== 'ready'
  stopButton.disabled =
    state.status !== 'ready' && state.status !== 'starting'
  restartButton.disabled = busy
}

function logLine(entry: BackendLogEntry): string {
  const time = new Date(entry.at).toLocaleTimeString('ru-RU', {
    hour12: false,
  })
  return `[${time}] ${entry.stream === 'stderr' ? 'err' : 'out'}  ${entry.line}`
}

function appendLog(entry: BackendLogEntry): void {
  logLines.push(logLine(entry))
  if (logLines.length > MAX_LOG_LINES) {
    logLines.splice(0, logLines.length - MAX_LOG_LINES)
  }
  logElement.textContent = logLines.join('\n')
  logElement.scrollTop = logElement.scrollHeight
}

function addressRow(label: string, url: string): HTMLDivElement {
  const row = document.createElement('div')
  row.className = 'address-row'

  const name = document.createElement('span')
  name.className = 'address-label'
  name.textContent = label

  const code = document.createElement('code')
  code.textContent = url

  const copy = document.createElement('button')
  copy.type = 'button'
  copy.className = 'btn btn-secondary btn-small'
  copy.textContent = 'Копировать'
  copy.addEventListener('click', () => {
    void copyWithFeedback(url, copy)
  })

  row.append(name, code, copy)
  return row
}

async function copyWithFeedback(
  text: string,
  button: HTMLButtonElement,
): Promise<void> {
  const original = button.textContent
  try {
    await window.asledger.copyText(text)
    button.textContent = 'Скопировано'
  } catch {
    button.textContent = 'Ошибка'
  }
  window.setTimeout(() => {
    button.textContent = original
  }, 1500)
}

function renderAddresses(addresses: NetworkAddress[]): void {
  addressesElement.replaceChildren()
  addressesEmptyElement.hidden = addresses.length > 0

  for (const item of addresses) {
    const row = document.createElement('li')
    row.className = 'address'

    const head = document.createElement('div')
    head.className = 'address-head'
    const ip = document.createElement('strong')
    ip.textContent = item.address
    const iface = document.createElement('span')
    iface.className = 'address-iface'
    iface.textContent = item.interfaceName
    head.append(ip, iface)

    row.append(
      head,
      addressRow('API', item.apiUrl),
      addressRow('Web', item.webUrl),
    )
    addressesElement.append(row)
  }
}

async function refreshNetwork(): Promise<void> {
  try {
    renderAddresses(await window.asledger.network.get())
  } catch (error) {
    addressesElement.replaceChildren()
    addressesEmptyElement.hidden = false
    addressesEmptyElement.textContent =
      error instanceof Error ? error.message : 'не удалось получить адреса'
  }
}

async function loadLogTail(): Promise<void> {
  try {
    const tail = await window.asledger.backend.readLog()
    const merged = [...new Set([...tail, ...logLines])]
    logLines.length = 0
    logLines.push(...merged.slice(-MAX_LOG_LINES))
    logElement.textContent = logLines.join('\n')
    logElement.scrollTop = logElement.scrollHeight
  } catch {
    // Журнала ещё может не быть: это не ошибка запуска.
  }
}

async function openLogFile(): Promise<void> {
  const original = openLogButton.textContent
  try {
    await window.asledger.backend.openLog()
  } catch {
    openLogButton.textContent = 'Ошибка'
    window.setTimeout(() => {
      openLogButton.textContent = original
    }, 1500)
  }
}

async function openBook(): Promise<void> {
  const original = openBookButton.textContent
  try {
    await window.asledger.openBook()
  } catch {
    openBookButton.textContent = 'Ошибка'
    window.setTimeout(() => {
      openBookButton.textContent = original
    }, 1500)
  }
}

async function loadInfo(): Promise<void> {
  try {
    const info = await window.asledger.getInfo()
    setText(versionElement, info.version)
    setText(platformElement, info.platform)
    setText(electronElement, info.electron)
  } catch (error) {
    setText(
      versionElement,
      error instanceof Error ? error.message : 'неизвестная ошибка',
    )
  }
}

let authAction: 'start' | 'restart' = 'start'

function setAuthError(message: string | null): void {
  if (!message) {
    passwordError.hidden = true
    passwordError.textContent = ''
    return
  }
  passwordError.textContent = message
  passwordError.hidden = false
}

function renderAuthStatus(status: KeyStorageStatus): void {
  rememberCheckbox.disabled = !status.available
  forgetKeyButton.hidden = !status.remembered
  if (!status.available) {
    rememberHint.textContent =
      status.backend === 'basic_text'
        ? 'Хранилище Linux использует basic_text и не шифрует ключ; сохранение отключено.'
        : 'Системное хранилище ключей недоступно; сохранение отключено.'
    return
  }
  if (status.remembered) {
    rememberHint.textContent =
      'Сохранённый ключ есть: книгу можно открыть без пароля. После смены пароля ключ нужно ввести заново.'
    return
  }
  rememberHint.textContent =
    'В safeStorage сохраняется ключ, а не пароль; на Linux без keyring сохранение отключено.'
}

async function refreshAuthStatus(): Promise<KeyStorageStatus | null> {
  try {
    const status = await window.asledger.auth.status()
    renderAuthStatus(status)
    return status
  } catch (error) {
    rememberHint.textContent =
      error instanceof Error
        ? `Не удалось проверить хранилище ключа: ${error.message}`
        : 'Не удалось проверить хранилище ключа'
    return null
  }
}

function openPasswordDialog(
  action: 'start' | 'restart',
  message?: string,
): void {
  authAction = action
  passwordInput.value = ''
  rememberCheckbox.checked = false
  passwordSubmitButton.disabled = false
  passwordSubmitButton.textContent =
    action === 'restart' ? 'Перезапустить' : 'Открыть книгу'
  passwordTitle.textContent =
    action === 'restart' ? 'Перезапуск книги' : 'Пароль книги'
  passwordHint.textContent =
    action === 'restart'
      ? 'Введите пароль книги, чтобы перезапустить asledgerd.'
      : 'Введите пароль книги. Он нужен для вывода ключа SQLCipher и не сохраняется в конфигурации.'
  setAuthError(message ?? null)
  if (!passwordDialog.open) {
    passwordDialog.showModal()
  }
  window.setTimeout(() => passwordInput.focus(), 0)
  void refreshAuthStatus()
}

async function runAction(
  action: () => Promise<BackendState>,
  onError?: (message: string) => void,
): Promise<BackendState | null> {
  try {
    const state = await action()
    applyState(state)
    return state
  } catch (error) {
    const message =
      error instanceof Error ? error.message : 'неизвестная ошибка'
    setStatus('error')
    setText(detailElement, message)
    onError?.(message)
    return null
  }
}

async function startWithRemembered(
  action: 'start' | 'restart',
): Promise<boolean> {
  const status = await refreshAuthStatus()
  if (!status?.remembered) {
    return false
  }
  const request: BackendStartRequest = { useRemembered: true }
  const state = await runAction(() =>
    action === 'restart'
      ? window.asledger.backend.restart(request)
      : window.asledger.backend.start(request),
  )
  if (state?.status === 'ready') {
    return true
  }
  await refreshAuthStatus()
  openPasswordDialog(
    action,
    state?.status === 'error' ? state.detail : undefined,
  )
  return false
}

async function requestStart(action: 'start' | 'restart'): Promise<void> {
  if (await startWithRemembered(action)) {
    return
  }
  const status = await window.asledger.auth.status().catch(() => null)
  if (status) {
    renderAuthStatus(status)
  }
  if (!passwordDialog.open) {
    openPasswordDialog(action)
  }
}

startButton.addEventListener('click', () => {
  void requestStart('start')
})
stopButton.addEventListener('click', () => {
  void runAction(() => window.asledger.backend.stop())
})
restartButton.addEventListener('click', () => {
  void requestStart('restart')
})
openBookButton.addEventListener('click', () => {
  void openBook()
})
clearLogButton.addEventListener('click', () => {
  logLines.length = 0
  logElement.textContent = ''
})
copyLogButton.addEventListener('click', () => {
  void copyWithFeedback(logLines.join('\n'), copyLogButton)
})
openLogButton.addEventListener('click', () => {
  void openLogFile()
})
refreshNetworkButton.addEventListener('click', () => {
  void refreshNetwork()
})

passwordForm.addEventListener('submit', (event) => {
  event.preventDefault()
  const password = passwordInput.value
  if (password.length === 0) {
    setAuthError('Введите пароль книги')
    passwordInput.focus()
    return
  }
  const request: BackendStartRequest = {
    password,
    remember: rememberCheckbox.checked,
  }
  passwordSubmitButton.disabled = true
  void (async () => {
    const state = await runAction(
      () =>
        authAction === 'restart'
          ? window.asledger.backend.restart(request)
          : window.asledger.backend.start(request),
      setAuthError,
    )
    passwordSubmitButton.disabled = false
    passwordInput.value = ''
    if (state?.status === 'ready') {
      passwordDialog.close()
      setAuthError(null)
      await refreshAuthStatus()
      return
    }
    if (state) {
      setAuthError(state.detail || 'Не удалось открыть книгу')
    }
  })()
})

passwordCancelButton.addEventListener('click', () => {
  passwordDialog.close()
})

forgetKeyButton.addEventListener('click', () => {
  void (async () => {
    try {
      await window.asledger.auth.forget()
      await refreshAuthStatus()
      setAuthError(null)
      rememberHint.textContent =
        'Сохранённый ключ удалён из системного хранилища.'
    } catch (error) {
      setAuthError(
        error instanceof Error ? error.message : 'Не удалось удалить ключ',
      )
    }
  })()
})

window.asledger.backend.onState(applyState)
window.asledger.backend.onLog(appendLog)
window.asledger.network.onAddresses(renderAddresses)

void loadInfo()
void (async () => {
  const state = await window.asledger.backend.getState().catch(() => null)
  if (state) {
    applyState(state)
  }
  if (state?.status === 'ready' || state?.status === 'starting') {
    await refreshAuthStatus()
    return
  }
  const status = await refreshAuthStatus()
  if (status?.remembered) {
    const started = await startWithRemembered('start')
    if (started) {
      return
    }
  }
  if (!passwordDialog.open) {
    openPasswordDialog(
      'start',
      state?.status === 'error' ? state.detail : undefined,
    )
  }
})()
void loadLogTail()
void refreshNetwork()
