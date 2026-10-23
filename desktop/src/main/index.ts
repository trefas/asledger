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
  type BackendStartRequest,
  type BackendState,
  type DesktopInfo,
  type KeyStorageStatus,
  type NetworkAddress,
} from '../shared/ipc'
import { deriveBookKey } from './book-key'
import { BackendProcess, type BackendSecret } from './backend'
import { loadOrCreateConfig, type DesktopConfig } from './config'
import { readLogTail } from './log'
import { NetworkMonitor, currentLocalIPv4 } from './network'
import {
  forgetRememberedKey,
  keyStorageStatus,
  loadRememberedKey,
  saveRememberedKey,
} from './remembered-key'
import {
  apiUrl,
  buildBackendArgs,
  healthUrl,
  resolveDesktopPaths,
  webUrl,
} from './paths'

const APP_ID = 'org.asledger.desktop'

let mainWindow: BrowserWindow | null = null
let bookWindow: BrowserWindow | null = null
let backend: BackendProcess | null = null
let networkMonitor: NetworkMonitor | null = null
let backendStopped = false
let backendDbPath = ''

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

  backendDbPath = paths.dbPath
  return server
}

function parseStartRequest(raw: unknown): BackendStartRequest {
  if (raw === undefined || raw === null) {
    return {}
  }
  if (typeof raw !== 'object' || Array.isArray(raw)) {
    throw new Error('backend: ожидался объект запроса')
  }
  const value = raw as Record<string, unknown>
  const password = value['password']
  if (password !== undefined && typeof password !== 'string') {
    throw new Error('backend: пароль должен быть строкой')
  }
  const remember = value['remember']
  if (remember !== undefined && typeof remember !== 'boolean') {
    throw new Error('backend: remember должен быть boolean')
  }
  const useRemembered = value['useRemembered']
  if (useRemembered !== undefined && typeof useRemembered !== 'boolean') {
    throw new Error('backend: useRemembered должен быть boolean')
  }
  if (password !== undefined && useRemembered === true) {
    throw new Error(
      'backend: нельзя одновременно ввести пароль и взять ключ из хранилища',
    )
  }
  return {
    password: password as string | undefined,
    remember: remember === true,
    useRemembered: useRemembered === true,
  }
}

/** Запускает/перезапускает сервер, не передавая секрет через argv/env. */
async function startBackend(
  server: BackendProcess,
  raw: unknown,
  restart: boolean,
): Promise<BackendState> {
  const request = parseStartRequest(raw)
  let secret: BackendSecret | undefined
  let rememberedKey: Buffer | null = null

  if (request.useRemembered) {
    rememberedKey = await loadRememberedKey()
    if (!rememberedKey) {
      throw new Error('Сохранённый ключ не найден; введите пароль книги')
    }
    secret = { kind: 'key', value: rememberedKey.toString('hex') }
  } else if (request.password !== undefined) {
    if (request.password.length === 0) {
      throw new Error('Введите пароль книги')
    }
    secret = { kind: 'password', value: request.password }
  }

  let state: BackendState
  try {
    state = restart
      ? await server.restart(secret)
      : await server.start(secret)
  } finally {
    rememberedKey?.fill(0)
  }

  if (
    rememberedKey &&
    state.status !== 'ready' &&
    /неверн|ключ|парол/i.test(state.detail)
  ) {
    await forgetRememberedKey()
  }

  if (state.status === 'ready' && request.remember && request.password) {
    try {
      const key = await deriveBookKey(request.password, backendDbPath)
      await saveRememberedKey(key)
      key.fill(0)
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error)
      // Секрет сюда не попадает: сохраняем только текст ошибки.
      console.warn('desktop: не удалось сохранить ключ:', message)
    }
  }

  return state
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

  const rendererUrl = process.env['ELECTRON_RENDERER_URL']
  if (!app.isPackaged && rendererUrl) {
    void window.loadURL(rendererUrl)
  } else {
    void window.loadFile(join(__dirname, '../renderer/index.html'))
  }
}

function createBookWindow(url: string): void {
  if (bookWindow && !bookWindow.isDestroyed()) {
    bookWindow.show()
    bookWindow.focus()
    return
  }

  const window = new BrowserWindow({
    width: 1200,
    height: 800,
    minWidth: 900,
    minHeight: 600,
    show: false,
    title: `${APP_NAME} — книга`,
    backgroundColor: '#ffffff',
    autoHideMenuBar: true,
    webPreferences: {
      contextIsolation: true,
      nodeIntegration: false,
      sandbox: true,
      spellcheck: false,
      devTools: !app.isPackaged,
    },
  })

  bookWindow = window
  window.on('closed', () => {
    if (bookWindow === window) {
      bookWindow = null
    }
  })
  window.once('ready-to-show', () => {
    window.show()
  })
  void window.loadURL(url).catch((error: unknown) => {
    const message = error instanceof Error ? error.message : String(error)
    dialog.showErrorBox('asledger', `Не удалось открыть книгу: ${message}`)
    window.close()
  })
}

function registerIpc(
  server: BackendProcess,
  monitor: NetworkMonitor,
  bookUrl: string,
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

  ipcMain.handle(IPC_CHANNELS.openBook, (): void => {
    if (server.getState().status !== 'ready') {
      throw new Error('Сервер ещё не готов')
    }
    createBookWindow(bookUrl)
  })
  ipcMain.handle(IPC_CHANNELS.backendGetState, (): BackendState => {
    return server.getState()
  })
  ipcMain.handle(
    IPC_CHANNELS.backendStart,
    async (_event, request: unknown): Promise<BackendState> => {
      return startBackend(server, request, false)
    },
  )
  ipcMain.handle(IPC_CHANNELS.backendStop, async (): Promise<BackendState> => {
    return server.stop()
  })
  ipcMain.handle(
    IPC_CHANNELS.backendRestart,
    async (_event, request: unknown): Promise<BackendState> => {
      return startBackend(server, request, true)
    },
  )
  ipcMain.handle(IPC_CHANNELS.authStatus, async (): Promise<KeyStorageStatus> => {
    return keyStorageStatus()
  })
  ipcMain.handle(IPC_CHANNELS.authForget, async (): Promise<void> => {
    await forgetRememberedKey()
  })
  ipcMain.handle(IPC_CHANNELS.backendReadLog, async (): Promise<string[]> => {
    return readLogTail(server.getState().logPath)
  })
  ipcMain.handle(IPC_CHANNELS.backendOpenLog, async (): Promise<void> => {
    const error = await shell.openPath(server.getState().logPath)
    if (error) {
      throw new Error(error)
    }
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
  registerIpc(backend, monitor, webUrl(config.webPort))
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
