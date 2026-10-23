import {
  statusLabel,
  statusTone,
  type AppStatus,
} from '../../shared/status'
import type {
  BackendLogEntry,
  BackendState,
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
const webUrlElement = requireElement<HTMLElement>('#web-url')
const apiUrlElement = requireElement<HTMLElement>('#api-url')
const pidElement = requireElement<HTMLElement>('#pid')
const logPathElement = requireElement<HTMLElement>('#log-path')
const logElement = requireElement<HTMLPreElement>('#log')
const versionElement = requireElement<HTMLElement>('#version')
const platformElement = requireElement<HTMLElement>('#platform')
const electronElement = requireElement<HTMLElement>('#electron')

const startButton = requireElement<HTMLButtonElement>('#start')
const stopButton = requireElement<HTMLButtonElement>('#stop')
const restartButton = requireElement<HTMLButtonElement>('#restart')
const clearLogButton = requireElement<HTMLButtonElement>('#clear-log')

const logLines: string[] = []
const MAX_LOG_LINES = 200

function setStatus(status: AppStatus): void {
  statusElement.textContent = statusLabel(status)
  statusElement.dataset['tone'] = statusTone(status)
}

function setText(element: HTMLElement, value: string): void {
  element.textContent = value
}

function applyState(state: BackendState): void {
  setStatus(state.status)
  setText(detailElement, state.detail)
  setText(webUrlElement, state.webUrl)
  setText(apiUrlElement, state.apiUrl)
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

window.asledger.backend.onState(applyState)
window.asledger.backend.onLog(appendLog)

void loadInfo()
void window.asledger.backend.getState().then(applyState)
