import { spawnSync } from 'node:child_process'
import { createHash } from 'node:crypto'
import { createReadStream, existsSync } from 'node:fs'
import { mkdir, rm } from 'node:fs/promises'
import { homedir, tmpdir } from 'node:os'
import { join } from 'node:path'

// Скрипт заранее скачивает служебные архивы electron-builder в его кеш.
// GitHub Releases периодически отдаёт 502/504, а зеркала изредка обрывают
// соединение. Поэтому скачиваем через curl с повторами и докачкой, проверяем
// SHA-256 и кладём архив туда, откуда electron-builder берёт его без сети:
// <кеш>/<release>/<файл>. Контрольные суммы повторяют toolsets самого
// electron-builder, чтобы не принять повреждённый архив за готовый.
const tools = {
  linux: [
    {
      release: 'appimage-12.0.1',
      file: 'appimage-12.0.1.7z',
      sha256: 'd12ff7eb8f1d1ec4652ca5237a7fbdca33acc0c758045636feca62dc6ecb8ec4',
    },
    {
      release: '7zip@1.0.0',
      file: '7zip-linux-x64.tar.gz',
      sha256: 'd151bb44b2a9d9bfc52813ce4cac042916394a0ab8a56bd5d221a5dc9ef8d5d7',
    },
    {
      release: 'fpm@2.1.4',
      file: 'fpm-1.17.0-ruby-3.4.3-linux-amd64.7z',
      sha256: '44b0ec6025c14ec137f56180e62675c0eae36233cdce53d0953d9c73ced8989f',
    },
  ],
  win32: [
    {
      release: '7zip@1.0.0',
      file: '7zip-win-x64.tar.gz',
      sha256: 'be071f15bd6da2f78fe81c6ddef2009b0c4d8a51f36b780cb806c7e6df95e1b3',
    },
    {
      release: 'nsis-3.0.4.1',
      file: 'nsis-3.0.4.1.7z',
      sha256: '9877df902530f96357d13a7a31ae2b9df67f48b11ffc9a1700a7c961574ec5fa',
    },
    {
      release: 'nsis-resources-3.4.1',
      file: 'nsis-resources-3.4.1.7z',
      sha256: '593a9a92ef958321293ac6a2ee61e64bf1bd543142a5bd6b3d310709cc924103',
    },
  ],
}

// npmmirror отдаёт служебные архивы без задержек; GitHub оставляем запасным
// источником на случай, если зеркало недоступно.
const sources = [
  'https://registry.npmmirror.com/-/binary/electron-builder-binaries',
  'https://github.com/electron-userland/electron-builder-binaries/releases/download',
]

function cacheDirectory() {
  const configured = process.env.ELECTRON_BUILDER_CACHE?.trim()
  if (configured) {
    return configured
  }
  if (process.platform === 'win32') {
    const localAppData = process.env.LOCALAPPDATA?.trim()
    return localAppData
      ? join(localAppData, 'electron-builder', 'Cache')
      : join(tmpdir(), 'electron-builder-cache')
  }
  const xdgCache = process.env.XDG_CACHE_HOME
  return xdgCache
    ? join(xdgCache, 'electron-builder')
    : join(homedir(), '.cache', 'electron-builder')
}

async function sha256(path) {
  const hash = createHash('sha256')
  for await (const chunk of createReadStream(path)) {
    hash.update(chunk)
  }
  return hash.digest('hex')
}

function download(url, destination) {
  const result = spawnSync(
    'curl',
    [
      '--fail',
      '--location',
      '--silent',
      '--show-error',
      '--retry',
      '3',
      '--retry-delay',
      '2',
      '--retry-all-errors',
      '--connect-timeout',
      '30',
      '--max-time',
      '180',
      // Докачиваем недописанный файл после обрыва соединения.
      '--continue-at',
      '-',
      '--output',
      destination,
      url,
    ],
    { stdio: 'inherit' },
  )
  if (result.error) {
    throw result.error
  }
  if (result.status !== 0) {
    throw new Error(`curl завершился с кодом ${result.status}`)
  }
}

async function fetchTool(tool) {
  const directory = join(cacheDirectory(), tool.release)
  const destination = join(directory, tool.file)

  if (existsSync(destination)) {
    if ((await sha256(destination)) === tool.sha256) {
      console.log(`Уже в кеше: ${tool.file}`)
      return
    }
    // Повреждённый или недокачанный файл нельзя продолжать: curl с
    // --continue-at отправит Range от неверной длины.
    await rm(destination, { force: true })
  }

  await mkdir(directory, { recursive: true })
  for (const source of sources) {
    const url = `${source}/${tool.release}/${tool.file}`
    try {
      download(url, destination)
      const actual = await sha256(destination)
      if (actual !== tool.sha256) {
        await rm(destination, { force: true })
        throw new Error(`SHA-256 не совпал: ${actual}`)
      }
      console.log(`Скачано: ${tool.file} (${source})`)
      return
    } catch (error) {
      // Частично скачанный файл оставляем: следующий источник продолжит
      // докачку с того же места.
      console.warn(`Не удалось скачать (${source}): ${error.message}`)
    }
  }

  await rm(destination, { force: true })
  throw new Error(`Не удалось скачать ${tool.file} ни с одного источника`)
}

const required = tools[process.platform] ?? []
if (required.length === 0) {
  console.log(`Для платформы ${process.platform} служебные архивы не нужны`)
  process.exit(0)
}

for (const tool of required) {
  await fetchTool(tool)
}
