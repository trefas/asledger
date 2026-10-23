import {
  statusLabel,
  statusTone,
  type AppStatus,
} from '../../shared/status'
import type {
  BackendLogEntry,
  BackendState,
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
const stopButton = requireElement<HTMLButtonElement>('#stop')
const restartButton = requireElement<HTMLButtonElement>('#restart')
const clearLogButton = requireElement<HTMLButtonElement>('#clear-log')
const copyLogButton = requireElement<HTMLButtonElement>('#copy-log')
const openLogButton = requireElement<HTMLButtonElement>('#open-log')
const refreshNetworkButton =
  requireElement<HTMLButtonElement>('#refresh-network')

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
  setStatus(state.status)
  setText(detailElement, state.detail)
  setText(webPortElement, portFromUrl(state.webUrl))
  setText(apiPortElement, portFromUrl(state.apiUrl))
  setText(pidElement, state.pid === null ? '—' : String(state.pid))
  setText(logPathElement, state.logPath)

  const busy = state.status === 'starting' || state.status === 'stopping'
  startButton.disabled = busy || state.status === 'ready'
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

async function runAction(action: () => Promise<BackendState>): Promise<void> {
  try {
    applyState(await action())
  } catch (error) {
    setStatus('error')
    setText(
      detailElement,
      error instanceof Error ? error.message : 'неизвестная ошибка',
    )
  }
}

startButton.addEventListener('click', () => {
  void runAction(() => window.asledger.backend.start())
})
stopButton.addEventListener('click', () => {
  void runAction(() => window.asledger.backend.stop())
})
restartButton.addEventListener('click', () => {
  void runAction(() => window.asledger.backend.restart())
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

window.asledger.backend.onState(applyState)
window.asledger.backend.onLog(appendLog)
window.asledger.network.onAddresses(renderAddresses)

void loadInfo()
void window.asledger.backend.getState().then(applyState)
void loadLogTail()
void refreshNetwork()
