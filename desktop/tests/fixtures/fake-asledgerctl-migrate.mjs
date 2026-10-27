import { readFileSync, writeFileSync } from 'node:fs'

const output = process.env.FAKE_ASLEDGERCTL_OUT
const args = process.argv.slice(2)
const password = readFileSync(0, 'utf8')
const payload = {
  args,
  env: process.env,
  password,
}
if (output) {
  writeFileSync(output, JSON.stringify(payload))
}
if (process.env.FAKE_ASLEDGERCTL_FAIL === '1') {
  console.error(`ошибка миграции: ${password}`)
  process.exit(1)
}
console.log('книга зашифрована: fake')
console.log('резервная копия: /tmp/fake-encrypted-backup.db')
