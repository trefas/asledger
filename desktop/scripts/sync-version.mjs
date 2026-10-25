import { readFileSync, writeFileSync } from 'node:fs'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const scriptDir = dirname(fileURLToPath(import.meta.url))
const desktopDir = resolve(scriptDir, '..')
const check = process.argv.includes('--check')
const rawVersion =
  process.env.ASLEDGER_VERSION ??
  process.argv
    .find((argument) => argument.startsWith('--version='))
    ?.slice('--version='.length)

if (!rawVersion) {
  throw new Error(
    'Задайте версию: ASLEDGER_VERSION=v1.3.0 npm run sync-version',
  )
}
const version = rawVersion.replace(/^v/, '')
if (!/^\d+\.\d+\.\d+(?:[-+][0-9A-Za-z.-]+)?$/.test(version)) {
  throw new Error(`Некорректная версия: ${rawVersion}`)
}

const packagePath = join(desktopDir, 'package.json')
const lockPath = join(desktopDir, 'package-lock.json')
const packageJson = JSON.parse(readFileSync(packagePath, 'utf8'))
const lock = JSON.parse(readFileSync(lockPath, 'utf8'))

if (check) {
  const lockVersion = lock.packages?.['']?.version ?? lock.version
  if (
    packageJson.version !== version ||
    lock.version !== version ||
    lockVersion !== version
  ) {
    throw new Error(
      `Версия не совпадает: package.json=${packageJson.version}, lock=${lock.version}/${lockVersion}, ожидалось ${version}`,
    )
  }
  console.log(`Версия ${version} согласована в package.json и package-lock.json.`)
  process.exit(0)
}

packageJson.version = version
lock.version = version
if (lock.packages?.['']) {
  lock.packages[''].version = version
}
writeFileSync(packagePath, `${JSON.stringify(packageJson, null, 2)}\n`)
writeFileSync(lockPath, `${JSON.stringify(lock, null, 2)}\n`)
console.log(`Версия desktop-приложения: ${version}`)
