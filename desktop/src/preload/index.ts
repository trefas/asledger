import {
  contextBridge,
  ipcRenderer,
  type IpcRendererEvent,
} from 'electron'
import {
  IPC_CHANNELS,
  type BackendApi,
  type BackendLogEntry,
  type BackendState,
  type DesktopApi,
  type DesktopInfo,
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
  start: () =>
    ipcRenderer.invoke(IPC_CHANNELS.backendStart) as Promise<BackendState>,
  stop: () =>
    ipcRenderer.invoke(IPC_CHANNELS.backendStop) as Promise<BackendState>,
  restart: () =>
    ipcRenderer.invoke(IPC_CHANNELS.backendRestart) as Promise<BackendState>,
  onState: (listener) =>
    subscribe<BackendState>(IPC_CHANNELS.backendState, listener),
  onLog: (listener) =>
    subscribe<BackendLogEntry>(IPC_CHANNELS.backendLog, listener),
}

const network: NetworkApi = {
  get: () =>
    ipcRenderer.invoke(IPC_CHANNELS.networkGet) as Promise<NetworkAddress[]>,
  onAddresses: (listener) =>
    subscribe<NetworkAddress[]>(IPC_CHANNELS.networkAddresses, listener),
}

const api: DesktopApi = {
  getInfo: () => ipcRenderer.invoke(IPC_CHANNELS.getInfo) as Promise<DesktopInfo>,
  backend,
  network,
  copyText: (text) =>
    ipcRenderer.invoke(IPC_CHANNELS.clipboardWrite, text) as Promise<void>,
}

contextBridge.exposeInMainWorld('asledger', api)
