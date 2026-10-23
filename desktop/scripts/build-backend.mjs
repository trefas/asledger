import { spawnSync } from 'node:child_process'
import { mkdirSync, readFileSync } from 'node:fs'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const scriptDir = dirname(fileURLToPath(import.meta.url))
const desktopDir = resolve(scriptDir, '..')
const repoRoot = resolve(desktopDir, '..')
const packageJson = JSON.parse(
  readFileSync(join(desktopDir, 'package.json'), 'utf8'),
)
const version = (process.env.ASLEDGER_VERSION ?? packageJson.version).replace(
  /^v/,
  '',
)

const platformMap = {
  win32: 'windows',
  linux: 'linux',
  darwin: 'darwin',
}

const archMap = {
  x64: 'amd64',
  arm64: 'arm64',
  ia32: '386',
  arm: 'arm',
}

function currentTarget() {
  const goos = platformMap[process.platform]
  const goarch = archMap[process.arch]
  if (!goos || !goarch) {
    throw new Error(
      `Неизвестная платформа для сборки: ${process.platform}/${process.arch}`,
    )
  }
  return `${goos}/${goarch}`
}

function parseTargets() {
  const fromArgs = process.argv
    .slice(2)
    .filter((argument) => argument.startsWith('--target='))
    .map((argument) => argument.slice('--target='.length))
  const fromEnv = (process.env.ASLEDGER_DESKTOP_TARGETS ?? '')
    .split(',')
    .map((value) => value.trim())
    .filter(Boolean)
  const targets = fromArgs.length > 0 ? fromArgs : fromEnv
  return targets.length > 0 ? targets : [currentTarget()]
}

function gitValue(args, fallback) {
  const result = spawnSync('git', args, {
    cwd: repoRoot,
    encoding: 'utf8',
  })
  const value = result.status === 0 ? result.stdout.trim() : ''
  return value || fallback
}

const commit = gitValue(['rev-parse', '--short', 'HEAD'], 'none')
const date = gitValue(['show', '-s', '--format=%cI', 'HEAD'], 'unknown')

for (const target of parseTargets()) {
  const [goos, goarch] = target.split('/')
  if (!goos || !goarch) {
    throw new Error(`Ожидался target вида linux/amd64, получено: ${target}`)
  }
  const binary = goos === 'windows' ? 'asledgerd.exe' : 'asledgerd'
  const output = join(
    desktopDir,
    'resources',
    'backend',
    `${goos}-${goarch}`,
    binary,
  )
  mkdirSync(dirname(output), { recursive: true })

  const ldflags = [
    '-s',
    '-w',
    `-X github.com/trefas/asledger/internal/version.Version=v${version}`,
    `-X github.com/trefas/asledger/internal/version.Commit=${commit}`,
    `-X github.com/trefas/asledger/internal/version.Date=${date}`,
  ].join(' ')

  const result = spawnSync(
    'go',
    ['build', '-trimpath', '-ldflags', ldflags, '-o', output, './cmd/asledgerd'],
    {
      cwd: repoRoot,
      env: {
        ...process.env,
        CGO_ENABLED: '0',
        GOOS: goos,
        GOARCH: goarch,
      },
      stdio: 'inherit',
    },
  )
  if (result.status !== 0) {
    throw new Error(`Не удалось собрать asledgerd для ${target}`)
  }
  console.log(`asledgerd ${target} -> ${output}`)
}
