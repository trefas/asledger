import {
  app,
  BrowserWindow,
  clipboard,
  dialog,
  ipcMain,
  shell,
} from 'electron'
import { dirname, join } from 'node:path'
import { APP_NAME } from '../shared/app'
import {
  IPC_CHANNELS,
  type BackendLogEntry,
  type BackendState,
  type DesktopInfo,
  type NetworkAddress,
} from '../shared/ipc'
import { BackendProcess } from './backend'
import { loadOrCreateConfig, type DesktopConfig } from './config'
import { NetworkMonitor, currentLocalIPv4 } from './network'
import {
  apiUrl,
  buildBackendArgs,
  healthUrl,
  resolveDesktopPaths,
  webUrl,
} from './paths'

const APP_ID = 'org.asledger.desktop'

let mainWindow: BrowserWindow | null = null
let backend: BackendProcess | null = null
let networkMonitor: NetworkMonitor | null = null
let backendStopped = false

function broadcast(channel: string, payload: unknown): void {
  for (const window of BrowserWindow.getAllWindows()) {
    if (!window.isDestroyed()) {
      window.webContents.send(channel, payload)
    }
  }
}

function createBackend(config: DesktopConfig): BackendProcess {
  const paths = resolveDesktopPaths({
    isPackaged: app.isPackaged,
    appPath: app.getAppPath(),
    resourcesPath: process.resourcesPath,
    userDataPath: app.getPath('userData'),
    dataDir: config.dataDir,
    logDir: config.logDir,
  })

  const server = new BackendProcess({
    command: paths.backendBinary,
    args: buildBackendArgs(paths, config.webPort, config.apiPort),
    cwd: paths.dataDir,
    logFile: paths.logFile,
    healthUrl: healthUrl(config.apiPort),
    webUrl: webUrl(config.webPort),
    apiUrl: apiUrl(config.apiPort),
  })

  server.onState((state: BackendState) => {
    broadcast(IPC_CHANNELS.backendState, state)
  })
  server.onLog((entry: BackendLogEntry) => {
    broadcast(IPC_CHANNELS.backendLog, entry)
  })

  return server
}

function createWindow(): void {
  const window = new BrowserWindow({
    width: 960,
    height: 720,
    minWidth: 720,
    minHeight: 560,
    show: false,
    title: APP_NAME,
    backgroundColor: '#f7f8f5',
    autoHideMenuBar: true,
    webPreferences: {
      preload: join(__dirname, '../preload/index.js'),
      contextIsolation: true,
      nodeIntegration: false,
      sandbox: true,
      spellcheck: false,
      devTools: !app.isPackaged,
    },
  })

  mainWindow = window
  window.on('closed', () => {
    if (mainWindow === window) {
      mainWindow = null
    }
  })

  window.on('ready-to-show', () => {
    window.show()
  })

  window.webContents.once('did-finish-load', () => {
    if (backend) {
      void backend.start()
    }
  })

  const rendererUrl = process.env['ELECTRON_RENDERER_URL']
  if (!app.isPackaged && rendererUrl) {
    void window.loadURL(rendererUrl)
  } else {
    void window.loadFile(join(__dirname, '../renderer/index.html'))
  }
}

function registerIpc(
  server: BackendProcess,
  monitor: NetworkMonitor,
): void {
  ipcMain.handle(IPC_CHANNELS.getInfo, (): DesktopInfo => {
    return {
      appName: APP_NAME,
      version: app.getVersion(),
      platform: process.platform,
      electron: process.versions.electron ?? 'unknown',
      chrome: process.versions.chrome ?? 'unknown',
    }
  })

  ipcMain.handle(IPC_CHANNELS.backendGetState, (): BackendState => {
    return server.getState()
  })
  ipcMain.handle(IPC_CHANNELS.backendStart, async (): Promise<BackendState> => {
    return server.start()
  })
  ipcMain.handle(IPC_CHANNELS.backendStop, async (): Promise<BackendState> => {
    return server.stop()
  })
  ipcMain.handle(IPC_CHANNELS.backendRestart, async (): Promise<BackendState> => {
    return server.restart()
  })
  ipcMain.handle(IPC_CHANNELS.networkGet, (): NetworkAddress[] => {
    return monitor.refresh()
  })
  ipcMain.handle(
    IPC_CHANNELS.clipboardWrite,
    (_event, text: unknown): void => {
      if (typeof text !== 'string') {
        throw new Error('clipboard: ожидалась строка')
      }
      clipboard.writeText(text)
    },
  )
}

async function bootstrap(): Promise<void> {
  let config: DesktopConfig
  try {
    config = await loadOrCreateConfig({
      appDir: app.isPackaged
        ? dirname(app.getPath('exe'))
        : app.getAppPath(),
      userDataPath: app.getPath('userData'),
    })
  } catch (error) {
    const message = error instanceof Error ? error.message : String(error)
    dialog.showErrorBox('asledger Desktop', message)
    app.quit()
    return
  }

  const monitor = new NetworkMonitor(
    () => currentLocalIPv4(config.webPort, config.apiPort),
    (addresses) => broadcast(IPC_CHANNELS.networkAddresses, addresses),
  )
  networkMonitor = monitor
  backend = createBackend(config)
  registerIpc(backend, monitor)
  monitor.start()
  createWindow()

  app.on('activate', () => {
    if (BrowserWindow.getAllWindows().length === 0) {
      createWindow()
    }
  })
}

app.setAppUserModelId(APP_ID)

app.whenReady().then(() => {
  void bootstrap()
})

app.on('web-contents-created', (_event, contents) => {
  contents.setWindowOpenHandler(({ url }) => {
    void shell.openExternal(url)
    return { action: 'deny' }
  })
})

app.on('before-quit', (event) => {
  networkMonitor?.stop()
  if (!backend || backendStopped) {
    return
  }
  event.preventDefault()
  void backend.stop().finally(() => {
    backendStopped = true
    app.quit()
  })
})

app.on('window-all-closed', () => {
  if (process.platform !== 'darwin') {
    app.quit()
  }
})
