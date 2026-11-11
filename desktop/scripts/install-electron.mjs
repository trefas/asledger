import { existsSync } from 'node:fs'
import { spawnSync } from 'node:child_process'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

// Скрипт ставит бинарник Electron после `npm ci` с
// ELECTRON_SKIP_BINARY_DOWNLOAD=1. Сначала пробуем зеркало npmmirror,
// затем — официальный GitHub: GitHub отдаёт 504 на release-assets, и без
// запасного источника smoke-тест и упаковка падают.
const desktopDir = resolve(dirname(fileURLToPath(import.meta.url)), '..')
const electronDir = join(desktopDir, 'node_modules', 'electron')
const installScript = join(electronDir, 'install.js')
const marker = join(electronDir, 'path.txt')

if (existsSync(marker)) {
  console.log('Electron уже установлен')
  process.exit(0)
}

if (!existsSync(installScript)) {
  console.error(`Не найден установщик Electron: ${installScript}`)
  process.exit(1)
}

// В CI ELECTRON_MIRROR задан на уровне джобы. Для запасной попытки с GitHub
// его нужно убрать, иначе она снова пойдёт на зеркало.
const githubEnv = { ...process.env }
delete githubEnv.ELECTRON_MIRROR

const attempts = [
  {
    name: 'зеркало npmmirror',
    env: { ELECTRON_MIRROR: 'https://registry.npmmirror.com/-/binary/electron/' },
  },
  { name: 'GitHub', env: githubEnv },
  { name: 'GitHub (повтор)', env: githubEnv },
]

for (const attempt of attempts) {
  console.log(`Установка Electron: ${attempt.name}`)
  const result = spawnSync(process.execPath, [installScript], {
    cwd: desktopDir,
    env: { ...process.env, ...attempt.env },
    stdio: 'inherit',
  })
  if (result.status === 0) {
    process.exit(0)
  }
}

console.error('Не удалось установить Electron ни с GitHub, ни с зеркала')
process.exit(1)
