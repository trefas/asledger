/** Каналы IPC между main и renderer. */
export const IPC_CHANNELS = {
  getInfo: 'desktop:get-info',
  openBook: 'desktop:open-book',
  backendGetState: 'backend:get-state',
  backendStart: 'backend:start',
  backendStop: 'backend:stop',
  backendRestart: 'backend:restart',
  backendState: 'backend:state',
  backendLog: 'backend:log',
  backendReadLog: 'backend:read-log',
  backendOpenLog: 'backend:open-log',
  authStatus: 'auth:status',
  authForget: 'auth:forget',
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

/** Запрос на запуск с паролем или сохранённым ключом. */
export interface BackendStartRequest {
  password?: string
  remember?: boolean
  useRemembered?: boolean
}

/** Состояние системного хранилища ключа. */
export interface KeyStorageStatus {
  available: boolean
  remembered: boolean
  backend: string
}

/** API управления сервером, доступное renderer через preload. */
export interface BackendApi {
  getState(): Promise<BackendState>
  start(request?: BackendStartRequest): Promise<BackendState>
  stop(): Promise<BackendState>
  restart(request?: BackendStartRequest): Promise<BackendState>
  readLog(): Promise<string[]>
  openLog(): Promise<void>
  onState(listener: (state: BackendState) => void): () => void
  onLog(listener: (entry: BackendLogEntry) => void): () => void
}

/** API системного хранилища ключа. */
export interface AuthApi {
  status(): Promise<KeyStorageStatus>
  forget(): Promise<void>
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
  openBook(): Promise<void>
  backend: BackendApi
  auth: AuthApi
  network: NetworkApi
  copyText(text: string): Promise<void>
}
