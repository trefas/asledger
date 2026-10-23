import {
  contextBridge,
  ipcRenderer,
  type IpcRendererEvent,
} from 'electron'
import {
  IPC_CHANNELS,
  type AuthApi,
  type BackendApi,
  type BackendLogEntry,
  type BackendStartRequest,
  type BackendState,
  type DesktopApi,
  type DesktopInfo,
  type KeyStorageStatus,
  type NetworkAddress,
  type NetworkApi,
} from '../shared/ipc'

function subscribe<T>(
  channel: string,
  listener: (payload: T) => void,
): () => void {
  const wrapped = (_event: IpcRendererEvent, payload: T): void => {
    listener(payload)
  }
  ipcRenderer.on(channel, wrapped)
  return () => {
    ipcRenderer.removeListener(channel, wrapped)
  }
}

const backend: BackendApi = {
  getState: () =>
    ipcRenderer.invoke(IPC_CHANNELS.backendGetState) as Promise<BackendState>,
  start: (request?: BackendStartRequest) =>
    ipcRenderer.invoke(
      IPC_CHANNELS.backendStart,
      request,
    ) as Promise<BackendState>,
  stop: () =>
    ipcRenderer.invoke(IPC_CHANNELS.backendStop) as Promise<BackendState>,
  restart: (request?: BackendStartRequest) =>
    ipcRenderer.invoke(
      IPC_CHANNELS.backendRestart,
      request,
    ) as Promise<BackendState>,
  readLog: () =>
    ipcRenderer.invoke(IPC_CHANNELS.backendReadLog) as Promise<string[]>,
  openLog: () =>
    ipcRenderer.invoke(IPC_CHANNELS.backendOpenLog) as Promise<void>,
  onState: (listener) =>
    subscribe<BackendState>(IPC_CHANNELS.backendState, listener),
  onLog: (listener) =>
    subscribe<BackendLogEntry>(IPC_CHANNELS.backendLog, listener),
}

const auth: AuthApi = {
  status: () =>
    ipcRenderer.invoke(IPC_CHANNELS.authStatus) as Promise<KeyStorageStatus>,
  forget: () => ipcRenderer.invoke(IPC_CHANNELS.authForget) as Promise<void>,
}

const network: NetworkApi = {
  get: () =>
    ipcRenderer.invoke(IPC_CHANNELS.networkGet) as Promise<NetworkAddress[]>,
  onAddresses: (listener) =>
    subscribe<NetworkAddress[]>(IPC_CHANNELS.networkAddresses, listener),
}

const api: DesktopApi = {
  getInfo: () => ipcRenderer.invoke(IPC_CHANNELS.getInfo) as Promise<DesktopInfo>,
  openBook: () =>
    ipcRenderer.invoke(IPC_CHANNELS.openBook) as Promise<void>,
  backend,
  auth,
  network,
  copyText: (text) =>
    ipcRenderer.invoke(IPC_CHANNELS.clipboardWrite, text) as Promise<void>,
}

contextBridge.exposeInMainWorld('asledger', api)
