/** Каналы IPC между main и renderer. */
export const IPC_CHANNELS = {
  getInfo: 'desktop:get-info',
  backendGetState: 'backend:get-state',
  backendStart: 'backend:start',
  backendStop: 'backend:stop',
  backendRestart: 'backend:restart',
  backendState: 'backend:state',
  backendLog: 'backend:log',
  backendReadLog: 'backend:read-log',
  backendOpenLog: 'backend:open-log',
  networkGet: 'network:get',
  networkAddresses: 'network:addresses',
  clipboardWrite: 'clipboard:write',
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

/** Готовый адрес для Android-клиента. */
export interface NetworkAddress {
  interfaceName: string
  address: string
  apiUrl: string
  webUrl: string
}

/** API списка локальных адресов. */
export interface NetworkApi {
  get(): Promise<NetworkAddress[]>
  onAddresses(listener: (addresses: NetworkAddress[]) => void): () => void
}

/** API управления сервером, доступное renderer через preload. */
export interface BackendApi {
  getState(): Promise<BackendState>
  start(): Promise<BackendState>
  stop(): Promise<BackendState>
  restart(): Promise<BackendState>
  readLog(): Promise<string[]>
  openLog(): Promise<void>
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
  network: NetworkApi
  copyText(text: string): Promise<void>
}
