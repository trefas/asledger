# asledger Desktop

Desktop-клиент asledger на Electron. Отдельное приложение, не связанное с
Android-клиентом: сервер `asledgerd` и Android остаются без изменений.

Сейчас в репозитории находятся **каркас** (#139) и **менеджер процесса**
(#140): окно Electron, безопасный preload-API, запуск `asledgerd`, healthcheck,
журнал и кнопки старта/остановки. Локальные IPv4 и упаковка появятся в
следующих задачах S17–S19.

## Требования

- Node.js **22.12+** (проверено на Node 26).
- npm.
- Для сборки пакетов — `electron-builder`; для запуска окна нужен графический
  сеанс (в CI используется xvfb).

## Команды

```bash
cd desktop
npm ci
npm run dev       # Electron + Vite dev server
npm run check     # typecheck, unit-тесты и production-сборка
npm run start     # запуск уже собранного приложения
npm run package   # несобранный каталог приложения для текущей ОС
npm run package:dist
```

`npm run package` создаёт каталог в `desktop/release/` и не требует прав
администратора. Сейчас в пакет не кладётся `asledgerd`: это задача #145.
Поэтому запуск из `release/linux-unpacked` покажет ошибку «бинарь не найден»;
для разработки используйте `npm run dev` или `npm start`.

В dev-режиме менеджер ищет `asledgerd` в `../bin/`. Соберите его командой
`make build` в корне репозитория. Данные и журналы лежат рядом с `config.json`
(portable) либо в `app.getPath('userData')`, если portable-режим недоступен.
Для изоляции можно задать `ASLEDGER_DESKTOP_CONFIG`,
`ASLEDGER_DESKTOP_DATA_DIR`, `ASLEDGER_DESKTOP_LOG_DIR` и `ASLEDGERD_BIN`.

## Структура

```
src/main/       главный процесс Electron
src/preload/    узкий contextBridge-API для renderer
src/renderer/   HTML/CSS/TypeScript интерфейса
src/shared/     типы и чистые функции, общие для процессов
tests/          unit-тесты Vitest
```

Главный процесс не загружает Node.js в renderer: включены
`contextIsolation`, `sandbox` и отключён `nodeIntegration`. Renderer получает
только методы из `window.asledger`, описанные в `src/shared/ipc.ts`.

## Конфигурация

При первом запуске создаётся `config.json`:

```json
{
  "web_port": 8777,
  "api_port": 8778,
  "data_path": "data",
  "log_path": "logs"
}
```

- Если каталог приложения доступен для записи, используется portable-режим:
  `config.json` лежит рядом с приложением.
- Иначе конфиг переносится в `app.getPath('userData')`.
- `data_path` и `log_path` могут быть относительными (от каталога конфига) или
  абсолютными.
- Переменная `ASLEDGER_DESKTOP_CONFIG` задаёт явный путь к конфигу; это удобно
  для тестов и переносимых сборок.
- Неверный JSON или порт не заменяются молча значениями по умолчанию: при
  запуске будет показана ошибка.
- Если порт занят, `asledgerd` завершается с сообщением вида
  `web: listen tcp :8777: bind: address already in use`; окно показывает его
  в журнале и переводит статус в «Ошибка».
- В `config.json` нет паролей, токенов и производных ключей. Секреты появятся
  только в задачах шифрования (#138) и будут храниться через системное
  хранилище, а не в `config.json` или `localStorage`.

## Безопасность

- CSP ограничивает источники скриптов, стилей и соединений.
- Новые окна и внешние ссылки обрабатываются через `shell.openExternal`.
- В каркасе нет паролей, токенов и доступа к файловой системе из renderer.
- Секреты появятся только в задачах шифрования (#138) и будут храниться через
  системное хранилище, а не в `config.json` или `localStorage`.

## Следующие шаги

- #142 — контракт запуска, healthcheck и ошибки портов на стороне сервера.
- #143 — локальные IPv4 и URL для телефона.
- #145 — упаковка бинаря, ресурсов и установщиков.
- #146 — CI, smoke/E2E и проверка артефактов.
