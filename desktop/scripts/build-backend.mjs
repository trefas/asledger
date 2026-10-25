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

// SQLCipher требует cgo и C-компилятор под каждую цель. На Linux amd64
// достаточно системного gcc; arm64 и Windows собираются кросс-компиляторами
// из GoReleaser-cross или установленных пакетов.
const ccByTarget = {
  'linux/amd64': 'gcc',
  'linux/arm64': 'aarch64-linux-gnu-gcc',
  'windows/amd64': 'x86_64-w64-mingw32-gcc',
  'windows/arm64': 'aarch64-w64-mingw32-gcc',
  'darwin/amd64': 'clang',
  'darwin/arm64': 'clang',
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
  const outputDir = join(
    desktopDir,
    'resources',
    'backend',
    `${goos}-${goarch}`,
  )
  mkdirSync(outputDir, { recursive: true })

  const cc = process.env.ASLEDGER_CC ?? ccByTarget[target]
  if (!cc) {
    throw new Error(
      `Не задан C-компилятор для ${target}; установите его или задайте ASLEDGER_CC`,
    )
  }

  const ldflags = [
    '-s',
    '-w',
    '-linkmode external',
    `-X github.com/trefas/asledger/internal/version.Version=v${version}`,
    `-X github.com/trefas/asledger/internal/version.Commit=${commit}`,
    `-X github.com/trefas/asledger/internal/version.Date=${date}`,
  ]
  // macOS не входит в MVP и не поддерживает -static; для Linux/Windows
  // бинарник должен быть самодостаточным.
  if (goos !== 'darwin') {
    ldflags.push('-extldflags=-static')
  }
  const ldflagsText = ldflags.join(' ')

  // Desktop использует asledgerd для работы и asledgerctl для смены пароля.
  for (const command of ['asledgerd', 'asledgerctl']) {
    const binary = goos === 'windows' ? `${command}.exe` : command
    const output = join(outputDir, binary)
    const result = spawnSync(
      'go',
      [
        'build',
        '-trimpath',
        '-tags=netgo,osusergo,sqlite_omit_load_extension',
        '-ldflags',
        ldflagsText,
        '-o',
        output,
        `./cmd/${command}`,
      ],
      {
        cwd: repoRoot,
        env: {
          ...process.env,
          CGO_ENABLED: '1',
          GOOS: goos,
          GOARCH: goarch,
          CC: cc,
        },
        stdio: 'inherit',
      },
    )
    if (result.status !== 0) {
      throw new Error(`Не удалось собрать ${command} для ${target}`)
    }
    console.log(`${command} ${target} (CC=${cc}) -> ${output}`)
  }
}
