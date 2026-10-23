# asledger Desktop

Desktop-клиент asledger на Electron. Отдельное приложение, не связанное с
Android-клиентом: сервер `asledgerd` и Android остаются без изменений.

Сейчас в репозитории находятся **каркас** (#139), **менеджер процесса**
(#140), portable-конфигурация (#141), адреса для телефона (#143), **журнал
сервера** (#144) и **упаковка** (#145): окно Electron, безопасный preload-API,
запуск `asledgerd`, healthcheck, статусы, журнал, кнопки старта/остановки и
готовые пакеты с `asledgerd`, ассетами и шаблонами.

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
npm run package   # распакованное приложение для текущей ОС
npm run package:dist  # AppImage/deb на Linux, NSIS/portable на Windows
npm run package:linux    # Linux AppImage/deb
npm run package:linux:arm64  # Linux arm64 AppImage/deb
npm run package:win      # Windows NSIS/portable (Windows runner или wine)
npm run package:win:zip  # Windows ZIP без wine (локально)
npm run verify:package   # проверить бинарь, ассеты и версию в пакете
```

`npm run package` создаёт каталог в `desktop/release/` и не требует прав
администратора. Перед упаковкой `build:backend` собирает `asledgerd` для
текущей ОС/архитектуры, а `afterPack` кладёт его в `resources/bin`. Для
разработки используйте `npm run dev` или `npm start`.

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
scripts/        сборка asledgerd, afterPack и проверка пакета
tests/          unit-тесты Vitest
```

Главный процесс не загружает Node.js в renderer: включены
`contextIsolation`, `sandbox` и отключён `nodeIntegration`. Renderer получает
только методы из `window.asledger`, описанные в `src/shared/ipc.ts`.

## Сборка и упаковка

Версия desktop-приложения берётся из `desktop/package.json` (сейчас `1.2.0`).
`npm run build:backend` передаёт её в `asledgerd` через `-ldflags`, поэтому
`asledgerd --version`, имя артефакта и `package.json` согласованы. При выпуске
по тегу версия синхронизируется заранее:

```bash
ASLEDGER_VERSION=v1.2.0 npm run sync-version
ASLEDGER_VERSION=v1.2.0 npm run verify:version
```

Порядок упаковки:

1. `build:backend` собирает статический `asledgerd` (`CGO_ENABLED=0`) в
   `resources/backend/<os>-<arch>/`; цель можно задать явно:
   `npm run build:backend -- --target=windows/amd64`.
2. `electron-builder` собирает renderer/main/preload в `out/` и кладёт их в
   `app.asar`.
3. `afterPack` копирует нужный бинарь в `resources/bin/asledgerd[.exe]`.
4. `extraResources` добавляет `assets/logo`, `templates/charts`, `LICENSE` и
   `README.md`. Пути в приложении считаются от `process.resourcesPath`, а не от
   текущего каталога запуска.

Артефакты:

- Linux amd64: AppImage и deb (`asledger-1.2.0-linux-x86_64.AppImage`,
  `asledger-1.2.0-linux-amd64.deb`).
- Linux arm64: AppImage и deb (`asledger-1.2.0-linux-arm64.AppImage`,
  `asledger-1.2.0-linux-arm64.deb`), `npm run package:linux:arm64`.
- Windows amd64: NSIS и portable (`asledger-1.2.0-win-x64.exe` и
  `asledger-1.2.0-win-x64-portable.exe`); для локальной кросс-сборки без wine
  доступен ZIP: `npm run package:win:zip`.
- macOS: **не входит в MVP v1.2.0**. Нет сертификата и нотаризации; сборка и
  публикация macOS станут отдельной задачей после проверки требований.

Подпись Windows/macOS в MVP не выполняется: артефакты неподписанные, ОС может
показать предупреждение. Подпись — отдельная задача. AppImage, portable и ZIP
не требуют прав администратора; NSIS с `perMachine: false` ставится в
пользовательский каталог. deb — системный пакет, его установка обычно требует
root; для работы без администратора используйте AppImage.

Проверка распакованного пакета:

```bash
npm run verify:package       # Linux/macOS: запускает asledgerd --version
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
- В `config.json` нет паролей, токенов и производных ключей. Секреты появятся
  только в задачах шифрования (#138) и будут храниться через системное
  хранилище, а не в `config.json` или `localStorage`.

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
- В каркасе нет паролей, токенов и доступа к файловой системе из renderer.
- Секреты появятся только в задачах шифрования (#138) и будут храниться через
  системное хранилище, а не в `config.json` или `localStorage`.

## Следующие шаги

- #146 — CI, smoke/E2E и проверка артефактов.
