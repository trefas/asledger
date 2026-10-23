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

const api: DesktopApi = {
  getInfo: () => ipcRenderer.invoke(IPC_CHANNELS.getInfo) as Promise<DesktopInfo>,
  backend,
}

contextBridge.exposeInMainWorld('asledger', api)
