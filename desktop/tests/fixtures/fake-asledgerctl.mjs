import { readFileSync, writeFileSync } from 'node:fs'

const output = process.env.FAKE_ASLEDGERCTL_OUT
const args = process.argv.slice(2)
const currentPassword = readFileSync(0, 'utf8')
const fileIndex = args.indexOf('-new-password-file')
const newPassword =
  fileIndex >= 0 ? readFileSync(args[fileIndex + 1], 'utf8') : readFileSync(3, 'utf8')
const payload = {
  args,
  env: process.env,
  currentPassword,
  newPassword,
}
if (output) {
  writeFileSync(output, JSON.stringify(payload))
}
if (process.env.FAKE_ASLEDGERCTL_FAIL === '1') {
  console.error(`ошибка: ${currentPassword} / ${newPassword}`)
  process.exit(1)
}
