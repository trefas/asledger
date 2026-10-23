import { spawnSync } from 'node:child_process'
import { existsSync, readFileSync } from 'node:fs'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const scriptDir = dirname(fileURLToPath(import.meta.url))
const desktopDir = resolve(scriptDir, '..')
const packageJson = JSON.parse(
  readFileSync(join(desktopDir, 'package.json'), 'utf8'),
)

const platform =
  process.argv
    .find((argument) => argument.startsWith('--platform='))
    ?.slice('--platform='.length) ?? process.platform
const arch =
  process.argv
    .find((argument) => argument.startsWith('--arch='))
    ?.slice('--arch='.length) ?? process.arch
if (!['linux', 'win32', 'darwin'].includes(platform)) {
  throw new Error(`Неизвестная платформа: ${platform}`)
}
if (!['x64', 'arm64'].includes(arch)) {
  throw new Error(`Неизвестная архитектура: ${arch}`)
}

const suffix = arch === 'x64' ? '' : `-${arch}`
const unpackedDir =
  platform === 'win32'
    ? join(desktopDir, 'release', `win${suffix}-unpacked`)
    : platform === 'darwin'
      ? join(desktopDir, 'release', `mac${suffix}`, 'asledger.app')
      : join(desktopDir, 'release', `linux${suffix}-unpacked`)

const resourcesDir =
  platform === 'darwin'
    ? join(unpackedDir, 'Contents', 'Resources')
    : join(unpackedDir, 'resources')

const binaryName = platform === 'win32' ? 'asledgerd.exe' : 'asledgerd'
const binaryPath = join(resourcesDir, 'bin', binaryName)

const appAsar = join(resourcesDir, 'app.asar')
const required = [
  appAsar,
  binaryPath,
  join(resourcesDir, 'assets', 'logo', 'asledger-logo.svg'),
  join(resourcesDir, 'templates', 'charts', 'personal.json'),
  join(resourcesDir, 'LICENSE'),
]

if (!existsSync(unpackedDir)) {
  throw new Error(
    `Не найден распакованный пакет: ${unpackedDir}. Сначала выполните npm run package.`,
  )
}

for (const path of required) {
  if (!existsSync(path)) {
    throw new Error(`В пакете нет обязательного файла: ${path}`)
  }
}

// В app.asar не должно быть абсолютных путей сборочной машины.
const asar = readFileSync(appAsar)
for (const absolute of [desktopDir, resolve(desktopDir, '..')]) {
  for (const candidate of [absolute, absolute.replaceAll('\\', '/')]) {
    if (asar.includes(Buffer.from(candidate))) {
      throw new Error(`В app.asar найден абсолютный путь сборки: ${candidate}`)
    }
  }
}

const expected = `v${packageJson.version}`
const canExecute = platform === process.platform && arch === process.arch
if (platform === 'win32' || !canExecute) {
  const binary = readFileSync(binaryPath)
  if (!binary.includes(Buffer.from(expected))) {
    throw new Error(
      `Версия ${binaryName} не совпадает с package.json: не найдено ${expected}`,
    )
  }
  console.log(`Пакет ${unpackedDir} содержит бинарь, ассеты и шаблоны.`)
  console.log(`Версия ${binaryName}: ${expected} найдена в бинаре.`)
} else {
  const result = spawnSync(binaryPath, ['--version'], { encoding: 'utf8' })
  if (result.status !== 0) {
    throw new Error(
      `asledgerd --version завершился с кодом ${result.status}: ${result.stderr}`,
    )
  }
  if (!result.stdout.includes(expected)) {
    throw new Error(
      `Версия asledgerd не совпадает с package.json: ожидалось ${expected}, получено ${result.stdout.trim()}`,
    )
  }
  console.log(`Пакет ${unpackedDir} содержит бинарь, ассеты и шаблоны.`)
  console.log(`Версия asledgerd: ${result.stdout.trim()}`)
}
