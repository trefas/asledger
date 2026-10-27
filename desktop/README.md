# asledger Desktop

Desktop-клиент asledger на Electron. Отдельное приложение; сервер `asledgerd`
теперь шифрует книгу SQLCipher, а Android-клиент продолжает работать через API
без изменения протокола.

Сейчас в репозитории находятся **каркас** (#139), **менеджер процесса**
(#140), portable-конфигурация (#141), адреса для телефона (#143), **журнал
сервера** (#144) и **упаковка** (#145): окно Electron, безопасный preload-API,
запуск `asledgerd`, healthcheck, статусы, журнал, кнопки старта/остановки,
открытие книги в отдельном окне, ввод пароля книги, «Запомнить ключ» через
`safeStorage`, автоматическая миграция plaintext-книги `v1.2.0` при первом
запуске и готовые пакеты с `asledgerd`, ассетами и шаблонами.

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
npm run test:e2e  # Playwright smoke: окно, книга, журнал, остановка
npm run start     # запуск уже собранного приложения
npm run package   # распакованное приложение для текущей ОС
npm run package:dist  # AppImage/deb на Linux, NSIS/portable на Windows
npm run package:linux    # Linux AppImage/deb
npm run package:linux:arm64  # Linux arm64 AppImage/deb
npm run package:win      # Windows NSIS/portable (Windows runner или wine)
npm run package:win:zip  # Windows ZIP без wine (локально)
npm run verify:package   # проверить бинарь, ассеты и версию в пакете
```

`npm run package` создаёт каталог в `desktop/release/` и не требует прав
администратора. Перед упаковкой `build:backend` собирает `asledgerd` и
`asledgerctl` для текущей ОС/архитектуры, а `afterPack` кладёт оба бинаря в
`resources/bin`. `asledgerctl` нужен для смены пароля из окна приложения. Для
разработки используйте `npm run dev` или `npm start`.

### Те же проверки, что в CI

```bash
cd desktop
npm ci
npm run check
npm run build:backend
npm run test:e2e       # на Linux без графического сеанса: xvfb-run -a npm run test:e2e
npm run package:dist
npm run verify:package
```

Для Windows-джобы CI:

```powershell
cd desktop
npm ci
npm run check
npm run package:win
npm run verify:package:win
```

`npm run check` включает typecheck, unit-тесты Vitest и production-сборку.
Smoke-тест Playwright запускает Electron, ждёт healthcheck, проверяет журнал и
корректную остановку `asledgerd`, а также миграцию plaintext-книги `v1.2.0`;
он не входит в `npm run check`, чтобы обычные тесты работали без графического
сеанса.

В dev-режиме менеджер ищет `asledgerd` и `asledgerctl` в `../bin/`. Соберите
их командой `make build` в корне репозитория. Данные и журналы лежат рядом с
`config.json` (portable) либо в `app.getPath('userData')`, если portable-режим
недоступен. Для изоляции можно задать `ASLEDGER_DESKTOP_CONFIG`,
`ASLEDGER_DESKTOP_DATA_DIR`, `ASLEDGER_DESKTOP_LOG_DIR`, `ASLEDGERD_BIN` и
`ASLEDGERCTL_BIN`.

## Структура

```
src/main/       главный процесс Electron
src/preload/    узкий contextBridge-API для renderer
src/renderer/   HTML/CSS/TypeScript интерфейса
src/shared/     типы и чистые функции, общие для процессов
scripts/        сборка asledgerd, afterPack и проверка пакета
tests/          unit-тесты Vitest
```

Главный процесс не загружает Node.js в renderer: включены
`contextIsolation`, `sandbox` и отключён `nodeIntegration`. Renderer получает
только методы из `window.asledger`, описанные в `src/shared/ipc.ts`.

Логотип в шапке и иконка окна берутся из общего каталога `../assets/logo`:
renderer подключает его через `publicDir` electron-vite, а main использует
`icon-512.png`. Отдельной копии логотипа в `desktop/` нет — источник остаётся
один для webUI, Android и desktop.

## Сборка и упаковка

Версия desktop-приложения берётся из `desktop/package.json` (сейчас `1.3.3`).
`npm run build:backend` передаёт её в `asledgerd` и `asledgerctl` через
`-ldflags`, поэтому версии бинарей, имя артефакта и `package.json` согласованы.
При выпуске
по тегу версия синхронизируется заранее:

```bash
ASLEDGER_VERSION=v1.3.3 npm run sync-version
ASLEDGER_VERSION=v1.3.3 npm run verify:version
```

Перед упаковкой нужны Go, Node.js 22+, gcc и кросс-компилятор для выбранной
цели. Windows CI ставит MinGW через MSYS2, Linux arm64 — пакет
`gcc-aarch64-linux-gnu` и `libc6-dev-arm64-cross`.

Порядок упаковки:

1. `build:backend` собирает статические `asledgerd` и `asledgerctl` с cgo и
   SQLCipher (`CGO_ENABLED=1`,
   `-tags=netgo,osusergo,sqlite_omit_load_extension`,
   `-linkmode external -extldflags=-static`) в
   `resources/backend/<os>-<arch>/`; цель можно задать явно:
   `npm run build:backend -- --target=windows/amd64`. Для linux/arm64 нужен
   `aarch64-linux-gnu-gcc`, для windows/amd64 — `x86_64-w64-mingw32-gcc`;
   переопределить компилятор можно `ASLEDGER_CC=...`.
2. `electron-builder` собирает renderer/main/preload в `out/` и кладёт их в
   `app.asar`.
3. `afterPack` копирует `asledgerd[.exe]` и `asledgerctl[.exe]` в
   `resources/bin`.
4. `extraResources` добавляет `assets/logo`, `templates/charts`, `LICENSE` и
   `README.md`. Пути в приложении считаются от `process.resourcesPath`, а не от
   текущего каталога запуска.

Артефакты:

- Linux amd64: AppImage и deb (`asledger-1.3.3-linux-x86_64.AppImage`,
  `asledger-1.3.3-linux-amd64.deb`).
- Linux arm64: AppImage и deb (`asledger-1.3.3-linux-arm64.AppImage`,
  `asledger-1.3.3-linux-arm64.deb`), `npm run package:linux:arm64`.
- Windows amd64: NSIS, portable и ZIP (`asledger-1.3.3-win-x64.exe`,
  `asledger-1.3.3-win-x64-portable.exe`, `asledger-1.3.3-win-x64.zip`); ZIP
  собирается `npm run package:win:zip`, в том числе локально без wine.
- macOS: **не входит в MVP v1.3.3**. Нет сертификата и нотаризации; сборка и
  публикация macOS станут отдельной задачей после проверки требований.

Подпись Windows/macOS в MVP не выполняется: артефакты неподписанные, ОС может
показать предупреждение. Подпись — отдельная задача. AppImage, portable и ZIP
не требуют прав администратора; NSIS с `perMachine: false` ставится в
пользовательский каталог. deb — системный пакет, его установка обычно требует
root; для работы без администратора используйте AppImage.

Проверка распакованного пакета:

```bash
npm run verify:package       # Linux amd64: запускает asledgerd --version
npm run verify:package:linux:arm64  # проверяет arm64-пакет
npm run verify:package:win   # Windows: проверяет бинарь, ассеты и версию
```

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
  `asledgerd: listen tcp :8777: bind: address already in use`; окно показывает
  его в статусе и журнале и переводит состояние в «Ошибка».
- В `config.json` нет паролей, токенов и производных ключей. Пароль и
  raw-ключ в него не пишутся; sidecar `data/asledger.db.kdf.json` содержит
  только соль и параметры Argon2id. При переносе книги копируйте `.db` и
  `.kdf.json` вместе.
- Если книги ещё нет или `data/asledger.db` имеет размер 0 байт, окно
  «Новый пароль книги» создаёт зашифрованную книгу и sidecar; пустой файл не
  нужно удалять вручную.
- Если при первом запуске найдена plaintext-книга `v1.2.0`, окно просит новый
  пароль с подтверждением и запускает встроенный `asledgerctl migrate-encrypt`
  (пароль через stdin). Создаются `data/asledger.db.encrypted-backup.db` и
  sidecar, затем сервер стартует с тем же паролем. Обычный DB Browser без
  SQLCipher такую книгу не откроет — это не повреждение.
- Пароль передаётся `asledgerd` через stdin (`-password-fd=0`), а не через
  argv/env. «Запомнить ключ» сохраняет в `safeStorage` только 32-байтовый
  raw-ключ; на Linux при `basic_text` или без keyring сохранение отключено.

## Журнал и ошибки

Панель «Журнал сервера» показывает последние строки `logs/asledgerd.log` и
обновляется по мере работы:

- при открытии окна читается только хвост файла — до 200 строк и не более
  64 КиБ, файл не загружается целиком;
- кнопка «Скопировать логи» копирует именно текущий хвост, «Открыть файл»
  открывает журнал системным приложением, «Очистить» очищает только панель;
- пароли, токены, `Authorization`/`Bearer` и секретные JSON-поля маскируются
  перед записью в файл и перед показом в окне;
- статусы «Запуск…», «Готово», «Ошибка», «Остановлен» и «Остановка…» не дают
  запустить второй процесс во время остановки;
- при занятом порте, отсутствии `asledgerd`, таймауте healthcheck или падении
  сервера показывается причина, а путь к журналу всегда виден в карточке
  сервера.

Журнал создаётся в portable-каталоге или в `app.getPath('userData')`, поэтому
для чтения не нужны права администратора.

## Книга в окне Electron

После статуса «Готово» доступна кнопка «Открыть книгу». Она открывает
отдельное окно Electron с web-интерфейсом `asledgerd` по адресу
`http://127.0.0.1:<web_port>` (по умолчанию `8777`). Окно работает без Node.js
в renderer: `contextIsolation`, `sandbox`, `nodeIntegration: false`; внешние
ссылки открываются в системном браузере. Повторное нажатие фокусирует уже
открытое окно. Если сервер остановлен или ещё запускается, кнопка недоступна.

## Адреса для телефона

Раздел «Адреса для телефона» показывает IPv4-адреса активных интерфейсов и
готовые URL:

- API — `http://<IP>:<api_port>` для Android-клиента;
- web — `http://<IP>:<web_port>` для браузера.

Loopback, link-local (`169.254.0.0/16`), multicast и зарезервированные адреса
скрыты. Одинаковые адреса на разных интерфейсах схлопываются; VPN и виртуальные
адаптеры остаются в списке с именем интерфейса. Список обновляется раз в 5
секунд и по кнопке «Обновить». URL копируется в системный буфер обмена.

Телефон должен быть в той же локальной сети (обычно Wi-Fi). Приложение не
открывает порты автоматически и не обещает доступ из интернета: для внешнего
доступа нужны отдельные настройки сети и защиты.

## Безопасность

- CSP ограничивает источники скриптов, стилей и соединений.
- Новые окна и внешние ссылки обрабатываются через `shell.openExternal`.
- Renderer не получает пароль, сохранённый ключ и доступ к файловой системе.
- Пароль/ключ не попадают в `config.json`, `localStorage` и журнал; renderer
  передаёт пароль в main по узкому IPC-каналу.
- `safeStorage` используется только для raw-ключа; `basic_text` на Linux
  считается недоступным хранилищем.

## Следующие шаги

- #147 — документация, установка и релиз desktop (завершено).
- #138/#148/#150/#151/#152/#154 — SQLCipher, миграция, пароль и desktop-ключ
  (завершено).
- #153 — совместимость Android с зашифрованным `asledgerd` (реализовано в
  JVM-тесте и документации).
- #155 — релиз `v1.3.0` опубликован; ручные проверки миграции/отката и
  Windows runtime пропущены по решению владельца проекта, issue закрыт.
- #157/#158 — общий логотип desktop в шапке и окне (завершено, патч
  `v1.3.1`).
- #160/#161 — SQLCipher 4.19.0/SQLite 3.53.4 (завершено, патч `v1.3.2`).
- #162 — выпуск `v1.3.2` (завершено).
- #164 — первый запуск с plaintext/пустой книгой и патч `v1.3.3` (завершено).
