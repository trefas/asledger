/** Каналы IPC между main и renderer. */
export const IPC_CHANNELS = {
  getInfo: 'desktop:get-info',
  backendGetState: 'backend:get-state',
  backendStart: 'backend:start',
  backendStop: 'backend:stop',
  backendRestart: 'backend:restart',
  backendState: 'backend:state',
  backendLog: 'backend:log',
} as const

/** Состояние дочернего процесса asledgerd. */
export type BackendStatus = 'stopped' | 'starting' | 'ready' | 'stopping' | 'error'

/** Снимок состояния сервера для renderer. */
export interface BackendState {
  status: BackendStatus
  detail: string
  pid: number | null
  webUrl: string
  apiUrl: string
  logPath: string
}

/** Одна строка журнала asledgerd. */
export interface BackendLogEntry {
  stream: 'stdout' | 'stderr'
  line: string
  at: string
}

/** API управления сервером, доступное renderer через preload. */
export interface BackendApi {
  getState(): Promise<BackendState>
  start(): Promise<BackendState>
  stop(): Promise<BackendState>
  restart(): Promise<BackendState>
  onState(listener: (state: BackendState) => void): () => void
  onLog(listener: (entry: BackendLogEntry) => void): () => void
}

/** Сведения о приложении, безопасно передаваемые в renderer. */
export interface DesktopInfo {
  appName: string
  version: string
  platform: string
  electron: string
  chrome: string
}

/** Узкий API, который preload выставляет в window.asledger. */
export interface DesktopApi {
  getInfo(): Promise<DesktopInfo>
  backend: BackendApi
}
