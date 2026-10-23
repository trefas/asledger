const { chmodSync, copyFileSync, existsSync, mkdirSync } = require('node:fs')
const { join } = require('node:path')

const platformMap = {
  linux: 'linux',
  win32: 'windows',
  darwin: 'darwin',
}

const archMap = {
  0: '386',
  1: 'amd64',
  2: 'arm',
  3: 'arm64',
  ia32: '386',
  x64: 'amd64',
  armv7l: 'arm',
  arm64: 'arm64',
}

/**
 * Кладёт asledgerd в resources/bin упакованного приложения.
 * Бинарь готовит scripts/build-backend.mjs для нужной ОС и архитектуры.
 */
module.exports = async function afterPack(context) {
  if (context.arch === 4 || context.arch === 'universal') {
    throw new Error('Универсальная сборка macOS не поддерживается в MVP')
  }
  const goos = platformMap[context.electronPlatformName]
  const goarch = archMap[context.arch]
  if (!goos || !goarch) {
    throw new Error(
      `Неизвестная цель упаковки: ${context.electronPlatformName}/${context.arch}`,
    )
  }

  const binary = goos === 'windows' ? 'asledgerd.exe' : 'asledgerd'
  const source = join(
    __dirname,
    '..',
    'resources',
    'backend',
    `${goos}-${goarch}`,
    binary,
  )
  if (!existsSync(source)) {
    throw new Error(
      `Не найден ${source}. Соберите бинарь: npm run build:backend -- --target=${goos}/${goarch}`,
    )
  }

  const resourcesDir = context.packager.getResourcesDir(context.appOutDir)
  const targetDir = join(resourcesDir, 'bin')
  const target = join(targetDir, binary)
  mkdirSync(targetDir, { recursive: true })
  copyFileSync(source, target)
  if (goos !== 'windows') {
    chmodSync(target, 0o755)
  }
  console.log(`afterPack: ${binary} (${goos}/${goarch}) -> ${target}`)
}
