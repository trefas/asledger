import { _electron as electron, expect, test } from '@playwright/test'
import { existsSync } from 'node:fs'
import { mkdtemp, rm, writeFile } from 'node:fs/promises'
import { createServer } from 'node:net'
import { tmpdir } from 'node:os'
import { join } from 'node:path'

function freePort(): Promise<number> {
  return new Promise((resolve, reject) => {
    const server = createServer()
    server.once('error', reject)
    server.listen(0, '127.0.0.1', () => {
      const address = server.address()
      if (!address || typeof address === 'string') {
        server.close()
        reject(new Error('не удалось получить свободный порт'))
        return
      }
      const port = address.port
      server.close(() => resolve(port))
    })
  })
}

function isAlive(pid: number): boolean {
  try {
    process.kill(pid, 0)
    return true
  } catch {
    return false
  }
}

function backendBinary(): string {
  const goos =
    process.platform === 'win32'
      ? 'windows'
      : process.platform === 'darwin'
        ? 'darwin'
        : 'linux'
  const goarch =
    process.arch === 'x64'
      ? 'amd64'
      : process.arch === 'arm64'
        ? 'arm64'
        : process.arch
  const binary = process.platform === 'win32' ? 'asledgerd.exe' : 'asledgerd'
  return (
    process.env.ASLEDGERD_BIN?.trim() ||
    join(process.cwd(), 'resources', 'backend', `${goos}-${goarch}`, binary)
  )
}

test('запускает asledgerd, открывает книгу и корректно останавливает', async () => {
  const dir = await mkdtemp(join(tmpdir(), 'asledger-desktop-e2e-'))
  const binary = backendBinary()
  if (!existsSync(binary)) {
    throw new Error(
      `Не найден ${binary}. Соберите бинарь: npm run build:backend`,
    )
  }

  const webPort = await freePort()
  const apiPort = await freePort()
  const configPath = join(dir, 'config.json')
  await writeFile(
    configPath,
    `${JSON.stringify(
      {
        web_port: webPort,
        api_port: apiPort,
        data_path: 'data',
        log_path: 'logs',
      },
      null,
      2,
    )}\n`,
  )

  const args = [
    '.',
    '--no-sandbox',
    `--user-data-dir=${join(dir, 'userdata')}`,
  ]
  if (process.platform === 'linux') {
    args.push('--ozone-platform=x11')
  }

  const app = await electron.launch({
    args,
    cwd: process.cwd(),
    env: {
      ...process.env,
      ASLEDGER_DESKTOP_CONFIG: configPath,
      ASLEDGERD_BIN: binary,
      ELECTRON_DISABLE_SECURITY_WARNINGS: 'true',
    },
  })

  try {
    const window = await app.firstWindow()
    await window.waitForLoadState('domcontentloaded')
    await expect(window).toHaveTitle(/asledger/)
    await expect(window.locator('#security')).toContainText('Безопасность')
    await expect(window.locator('#cloud-warning')).toContainText('облачной')

    // Первый запуск: sidecar ещё нет, asledgerd ждёт пароль через stdin.
    await expect(window.locator('#password-dialog')).toBeVisible({
      timeout: 10_000,
    })
    await window.locator('#password').fill('e2e-password')
    await window.locator('#password-submit').click()

    await expect(window.locator('#status')).toHaveText('Готово', {
      timeout: 20_000,
    })
    await expect(window.locator('#detail')).toHaveText('Сервер готов')
    await expect(window.locator('#log-path')).toContainText('asledgerd.log')
    await expect(window.locator('#log')).toContainText('asledgerd', {
      timeout: 10_000,
    })

    const pidText = (await window.locator('#pid').innerText()).trim()
    expect(pidText).toMatch(/^\d+$/)
    const pid = Number(pidText)
    expect(isAlive(pid)).toBe(true)

    await window.locator('#copy-log').click()
    await expect(window.locator('#copy-log')).toHaveText('Скопировано')
    const clipboard = await app.evaluate(({ clipboard }) =>
      clipboard.readText(),
    )
    expect(clipboard).toContain('asledgerd')

    const bookPromise = app.waitForEvent('window')
    await window.locator('#open-book').click()
    const book = await bookPromise
    await book.waitForLoadState('domcontentloaded')
    expect(book.url()).toContain(`:${webPort}`)

    // Останавливаем сервер до закрытия окна книги: Playwright/Electron на
    // Linux зависает на page.close() второго окна, а app.close() закроет всё.
    await window.locator('#stop').click()
    await expect(window.locator('#status')).toHaveText('Остановлен', {
      timeout: 15_000,
    })
    await expect(window.locator('#pid')).toHaveText('—')
    await expect.poll(() => isAlive(pid), { timeout: 10_000 }).toBe(false)

    await expect
      .poll(
        async () => {
          try {
            const response = await fetch(
              `http://127.0.0.1:${apiPort}/api/v1/health`,
            )
            return response.status
          } catch {
            return 'closed'
          }
        },
        { timeout: 10_000 },
      )
      .toBe('closed')
  } finally {
    await app.close()
    await rm(dir, { recursive: true, force: true })
  }
})
