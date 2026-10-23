import { createServer } from 'node:http'

const args = process.argv.slice(2)

function value(name) {
  const index = args.indexOf(name)
  return index >= 0 ? args[index + 1] : undefined
}

const mode = value('--mode') ?? 'ready'
const port = Number(value('--port') ?? '0')

if (mode === 'exit') {
  console.error('fake-asledgerd: ошибка запуска')
  process.exit(3)
}

if (mode === 'port') {
  console.error('asledgerd: web: listen tcp :8777: bind: address already in use')
  process.exit(1)
}

if (mode === 'sleep') {
  console.log('fake-asledgerd: спит')
  setInterval(() => {}, 1000)
  process.on('SIGTERM', () => process.exit(0))
  process.on('SIGINT', () => process.exit(0))
} else {
  const server = createServer((request, response) => {
    if (request.url === '/api/v1/health') {
      response.writeHead(200, { 'content-type': 'application/json' })
      response.end('{"status":"ok"}')
      return
    }
    response.writeHead(404)
    response.end()
  })

  server.listen(port, '127.0.0.1', () => {
    console.log('fake-asledgerd: готов')
    console.log('password=hunter2 token=abc123')
    console.error('fake-asledgerd: тестовая строка stderr')
  })

  const stop = () => {
    server.close(() => process.exit(0))
  }
  process.on('SIGTERM', stop)
  process.on('SIGINT', stop)
}
