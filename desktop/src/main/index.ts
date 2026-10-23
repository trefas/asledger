import { app, BrowserWindow, ipcMain, shell } from 'electron'
import { join } from 'node:path'
import { APP_NAME } from '../shared/app'
import {
  IPC_CHANNELS,
  type BackendLogEntry,
  type BackendState,
  type DesktopInfo,
} from '../shared/ipc'
import { BackendProcess } from './backend'
import {
  DEFAULT_API_PORT,
  DEFAULT_WEB_PORT,
  apiUrl,
  buildBackendArgs,
  healthUrl,
  resolveDesktopPaths,
  webUrl,
} from './paths'

const APP_ID = 'org.asledger.desktop'

let mainWindow: BrowserWindow | null = null
let backend: BackendProcess | null = null
let backendStopped = false

function broadcast(channel: string, payload: unknown): void {
  for (const window of BrowserWindow.getAllWindows()) {
    if (!window.isDestroyed()) {
      window.webContents.send(channel, payload)
    }
  }
}

function createBackend(): BackendProcess {
  const paths = resolveDesktopPaths({
    isPackaged: app.isPackaged,
    appPath: app.getAppPath(),
    resourcesPath: process.resourcesPath,
    userDataPath: app.getPath('userData'),
  })

  const server = new BackendProcess({
    command: paths.backendBinary,
    args: buildBackendArgs(paths, DEFAULT_WEB_PORT, DEFAULT_API_PORT),
    cwd: paths.dataDir,
    logFile: paths.logFile,
    healthUrl: healthUrl(DEFAULT_API_PORT),
    webUrl: webUrl(DEFAULT_WEB_PORT),
    apiUrl: apiUrl(DEFAULT_API_PORT),
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

function registerIpc(server: BackendProcess): void {
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
}

app.setAppUserModelId(APP_ID)

app.whenReady().then(() => {
  backend = createBackend()
  registerIpc(backend)
  createWindow()

  app.on('activate', () => {
    if (BrowserWindow.getAllWindows().length === 0) {
      createWindow()
    }
  })
})

app.on('web-contents-created', (_event, contents) => {
  contents.setWindowOpenHandler(({ url }) => {
    void shell.openExternal(url)
    return { action: 'deny' }
  })
})

app.on('before-quit', (event) => {
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
