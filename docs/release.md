# Версионирование и выпуск релиза

Документ описывает, как устроены версия сборки, артефакты релиза и публикация
по тегу (разделы 13.2–13.3 эпика).

## Версия сборки

Версия, коммит и дата сборки задаются через `-ldflags` и живут в пакете
`internal/version`:

```bash
make build VERSION=v1.0.0
```

Makefile передаёт значения в линкер:

```
-X github.com/trefas/asledger/internal/version.Version=v1.0.0
-X github.com/trefas/asledger/internal/version.Commit=<хеш>
-X github.com/trefas/asledger/internal/version.Date=<RFC3339>
```

Версия видна:

- `asledgerctl version` — `asledgerctl v1.0.0 (abc1234, …)`;
- `asledgerd --version` (и `-version`) — `asledgerd v1.0.0 (…)`;
- в подвале каждой страницы web-интерфейса — `asledger · версия v1.0.0`;
- в журнале запуска демона (`version`, `web`, `api`, `db`).

Локальная сборка без `-ldflags` показывает `dev (none, unknown)` — это
внятное значение по умолчанию, а не пустая строка.

## Артефакты

Инструмент сборки — **GoReleaser** (`.goreleaser.yaml`): он даёт матрицу
платформ, архивы, контрольные суммы и release notes без самописного
пайплайна. SQLCipher требует cgo, поэтому сборка идёт с `CGO_ENABLED=1` и
статической линковкой; матрица и компиляторы описаны ниже.

| Платформа | Архив | Бинарники |
| --- | --- | --- |
| linux/amd64 | `asledger_<версия>_linux_amd64.tar.gz` | `asledgerd`, `asledgerctl` |
| linux/arm64 | `asledger_<версия>_linux_arm64.tar.gz` | `asledgerd`, `asledgerctl` |
| windows/amd64 | `asledger_<версия>_windows_amd64.zip` | `asledgerd.exe`, `asledgerctl.exe` |
| Android 12+ | `asledger-<версия>.apk` | приложение asledger |

В именах архивов версия печатается без префикса `v` (`asledger_1.0.0_…`), а в
самих бинарниках — как в теге (`asledgerd v1.0.0`): GoReleaser подставляет в
`-ldflags` значение `.Tag`.

### Сборка с cgo

SQLCipher встроен в драйвер `github.com/meow-io/go-sqlcipher v1.1.2`
(SQLCipher 4.19.0, SQLite 3.53.4; в `v1.3.0`–`v1.3.1` использовался
`mutecomm/go-sqlcipher/v4 v4.4.2`), поэтому `CGO_ENABLED=0` больше не
используется. Для релиза задаются:

```text
CGO_ENABLED=1
tags: netgo osusergo sqlite_omit_load_extension
ldflags: -s -w -linkmode external -extldflags "-static"
CC:
  linux/amd64   gcc
  linux/arm64   aarch64-linux-gnu-gcc
  windows/amd64 x86_64-w64-mingw32-gcc
```

GoReleaser запускается в `ghcr.io/goreleaser/goreleaser-cross:v1.26.10`
(Go 1.26.10, GoReleaser 2.18.3): образ содержит все три компилятора. Версия
образа закреплена; при обновлении Go её нужно поднять вместе с `go.mod`.
Локальный `make build` также требует cgo и gcc; `make check` на машине без
C-компилятора не пройдёт.

В каждый архив входят:

- `README.md`, `LICENSE`;
- `docs/` — установка, API, web, hledger, тестирование;
- `systemd/asledgerd.service` и `systemd/asledgerd.env` (Linux);
- `templates/charts/` — шаблонные планы счетов;
- `assets/logo/` — лого и favicon.

Рядом с архивами публикуется `checksums.txt` — **SHA-256** всех артефактов.
Для APK дополнительно публикуется `asledger-<версия>.apk.sha256`. Проверка:

```bash
sha256sum -c checksums.txt
sha256sum -c asledger-1.3.3.apk.sha256
```

## Desktop-клиент (Electron)

Desktop-клиент собирается отдельно из `desktop/` и кладёт в пакет свой
`asledgerd`, `asledgerctl`, ассеты и шаблоны. Версия синхронизируется с тегом:

```bash
cd desktop
ASLEDGER_VERSION="${GITHUB_REF_NAME}" npm run sync-version
ASLEDGER_VERSION="${GITHUB_REF_NAME}" npm run verify:version
npm run package:dist          # Linux amd64: AppImage и deb
npm run package:linux:arm64   # Linux arm64: AppImage и deb
npm run package:win           # Windows amd64: NSIS и portable
npm run package:win:zip       # Windows amd64: ZIP без установки
```

`build-backend` собирает `asledgerd` и `asledgerctl` с cgo и статической
линковкой. На Linux
для amd64 нужен `gcc`, для arm64 — `aarch64-linux-gnu-gcc` и
`libc6-dev-arm64-cross`. На Windows нужен MinGW (`x86_64-w64-mingw32-gcc`);
CI ставит его через MSYS2. `package:win` и `package:win:zip` запускаются на
Windows-раннере; локальная кросс-сборка ZIP возможна без wine, но требует
кросс-компилятор. macOS в MVP `v1.3.3` не входит: нет сертификата и
нотаризации. Windows-артефакты не подписываются; подпись — отдельная задача.

### Патч `v1.3.3`

`v1.3.3` — патч к `v1.3.2` для первого запуска desktop после перехода на
SQLCipher. Если рядом осталась plaintext-книга `v1.2.0`, desktop показывает
«Новый пароль для миграции», сам вызывает встроенный
`asledgerctl migrate-encrypt` (пароль только через stdin), создаёт
зашифрованную резервную копию и sidecar, после чего запускает `asledgerd` с тем
же паролем. Отсутствующий или нулевой `data/asledger.db` считается новой
книгой: окно «Новый пароль книги» создаёт зашифрованную книгу и sidecar, файл
не нужно удалять вручную. `ASLEDGER_*` очищаются из окружения дочерних
`asledgerctl`/`asledgerd`, а каталог на месте книги не считается отсутствующей
книгой (актуально для Windows).

Формат книги, DSN, KDF, sidecar и Android API не меняются. Android-клиент не
меняется, кроме версии релиза; Go-сервер и матрица платформ те же, что в
`v1.3.2`. Изменения: #164. Сценарии закреплены unit- и E2E-тестами, включая
создание книги из отсутствующего и 0-байтного файла; подробности — в
[docs/testing.md](testing.md).

### Патч `v1.3.2` (предыдущий)

`v1.3.2` — патч к `v1.3.1`: драйвер SQLCipher заменён на
`github.com/meow-io/go-sqlcipher v1.1.2` (SQLCipher 4.19.0, SQLite 3.53.4).
Закрыты CVE-2021-3119, CVE-2026-51936 и FTS5 CVE-2026-11822/11824; формат
книги, DSN, KDF, sidecar и Android API не меняются. Книги, созданные
`v1.3.0`/`v1.3.1`, открываются без миграции; обратная совместимость с
SQLCipher 4.4.2 проверена `TestOpenBookFromSQLCipher442` и фикстурой
`pkg/core/store/testdata/sqlcipher-4.4.2.db`.

Изменения: #160, #161. Android-клиент не меняется, кроме версии релиза;
desktop использует тот же общий логотип. Матрица платформ, артефакты и
требования к cgo/статической линковке не меняются.

Ручные проверки миграции `1.2.0 → 1.3.x`, отката и Windows runtime из
[#155](https://github.com/trefas/asledger/issues/155) были пропущены по решению
владельца проекта; #155, S22 и эпики #137/#138 остаются закрытыми.
Автоматические проверки и CI перечислены в [docs/testing.md](testing.md).

### Патч `v1.3.1` (предыдущий)

`v1.3.1` — патч к `v1.3.0`: desktop-клиент использует общий
`assets/logo/asledger-logo.svg` в шапке и `icon-512.png` как иконку основного
окна и окна книги. SQLCipher, формат книги, sidecar и API не менялись;
обновление с `v1.3.0` не требует миграции данных.

Изменения: #157, #158. Релиз `v1.3.0` остаётся точкой входа для шифрования;
его содержание и миграция с `v1.2.0` описаны ниже.

### Обновление S24 (#160)

S24 вошло в `v1.3.2`: драйвер SQLCipher заменён на
`github.com/meow-io/go-sqlcipher v1.1.2` (SQLCipher 4.19.0, SQLite 3.53.4).
Это закрывает CVE-2021-3119, CVE-2026-51936 и CVE-2026-11822/11824, не меняя
формат книги, DSN, KDF и Android-совместимость. Книги, созданные `v1.3.0`/
`v1.3.1`, открываются без миграции; регрессия проверяется
`TestOpenBookFromSQLCipher442`. Матрица платформ, артефакты и требования к
cgo/статической линковке не меняются.

### Содержание `v1.3.0`

- SQLCipher для книги, WAL и резервных копий; драйвер
  `github.com/mutecomm/go-sqlcipher/v4`, cgo и статическая линковка.
- Argon2id (64 МиБ, 3 прохода, 4 потока) и sidecar `<db>.kdf.json` без пароля
  и производного ключа.
- `asledgerctl migrate-encrypt` для plaintext-книг, `passwd` для смены пароля,
  зашифрованный `backup`.
- Пароль в desktop-клиенте, передача через stdin, «Запомнить ключ» только как
  32-байтовый raw-ключ в `safeStorage`; на Linux `basic_text` отключён.
- Первый запуск с подтверждением пароля и индикатором сложности, экран
  «Безопасность» с чек-листом и предупреждением об облачной папке, смена
  пароля из UI через встроенный `asledgerctl` с резервной копией.
- Android-клиент не меняет API и работает с зашифрованным `asledgerd` через
  токен устройства; JVM-интеграционный тест запускает сервер с
  `-password-file`.

### Обновление с `v1.2.0`

Книга `v1.2.0` была незашифрованной. В desktop-клиенте `v1.3.3` достаточно
запустить новую версию: при наличии plaintext-книги окно покажет «Новый пароль
для миграции», само выполнит `asledgerctl migrate-encrypt` через stdin и
создаст зашифрованную резервную копию и sidecar. Если книги ещё нет или
`data/asledger.db` имеет размер 0 байт, это считается новой книгой: окно
«Новый пароль книги» создаёт зашифрованную книгу и sidecar, пустой файл не
нужно удалять вручную. Для headless-установки или ручного контроля:

1. Остановите службу/`asledgerd` и сделайте резервную копию книги.
2. Выполните `asledgerctl migrate-encrypt -db /path/to/asledger.db` (пароль
   введите в TTY или передайте через `-password-file`). Команда создаст
   зашифрованную копию, проверит её и заменит книгу; sidecar
   `asledger.db.kdf.json` появится рядом.
3. Запустите `asledgerd` с тем же паролем: TTY, `-password-file`,
   `-password-fd` или systemd `LoadCredential`.
4. Сохраните пароль и sidecar. Без них зашифрованную книгу не открыть;
   резервная копия `migrate-encrypt` лежит рядом с книгой.

Если книга уже зашифрована, `migrate-encrypt` откажется работать. Для смены
пароля используйте `asledgerctl passwd`; для резервной копии —
`asledgerctl backup`. Обычный DB Browser for SQLite не открывает
SQLCipher-книгу без SQLCipher-плагина и ключа: ошибка `file is not a database`
там ожидаема и не означает повреждение.

### Артефакты `v1.3.3`

| Файл | Платформа | Установка |
| --- | --- | --- |
| `asledger-1.3.3-linux-x86_64.AppImage` | Linux amd64 | без установки |
| `asledger-1.3.3-linux-amd64.deb` | Linux amd64 | `sudo apt install ./…` |
| `asledger-1.3.3-linux-arm64.AppImage` | Linux arm64 | без установки |
| `asledger-1.3.3-linux-arm64.deb` | Linux arm64 | `sudo apt install ./…` |
| `asledger-1.3.3-win-x64.exe` | Windows x64 | NSIS, per-user |
| `asledger-1.3.3-win-x64-portable.exe` | Windows x64 | portable |
| `asledger-1.3.3-win-x64.zip` | Windows x64 | распаковать и запустить |
| `asledger-1.3.3.apk` | Android 12+ | установить APK |
| `asledger-1.3.3.apk.sha256` | Android | SHA-256 APK |
| `SHA256SUMS-linux` | Linux | контрольные суммы |
| `SHA256SUMS-windows` | Windows | контрольные суммы |

Ссылки на файлы релиза строятся по шаблону
`https://github.com/trefas/asledger/releases/download/v1.3.3/<файл>`;
страница релиза — `https://github.com/trefas/asledger/releases/tag/v1.3.3`.
Конкретные SHA-256 появляются в `SHA256SUMS-linux` и `SHA256SUMS-windows`
после сборки; они не хранятся в репозитории. Проверка:

```bash
# Linux/macOS или Git Bash
sha256sum -c SHA256SUMS-linux
sha256sum -c SHA256SUMS-windows

# Windows PowerShell
Get-FileHash .\asledger-1.3.3-win-x64.exe -Algorithm SHA256
```

Перед публикацией workflow сверяет версию `package.json`, `package-lock.json`,
`asledgerd --version` и тег. Для этого в desktop-джобах выполняются
`sync-version` и `verify:version`; упакованный бинарь проверяется
`verify:package`. Если версия не совпала, релиз не публикуется.

### Проверка релиза `v1.3.3`

1. Скачать `SHA256SUMS-linux` и `SHA256SUMS-windows`, проверить хэши скачанных
   файлов.
2. На чистой Linux-машине запустить AppImage без установки: окно открывается,
   статус «Готово», кнопка «Открыть книгу» показывает web-интерфейс,
   `asledgerd` останавливается при закрытии приложения.
3. На чистой Windows-машине проверить NSIS, portable и ZIP: установка без
   администратора (NSIS/portable), запуск, healthcheck, открытие книги,
   остановка процесса.
4. Первый запуск с отсутствующей книгой: удалить или переместить `data/`,
   запустить приложение, ввести новый пароль в окне «Новый пароль книги».
   Должны появиться `data/asledger.db` с заголовком не `SQLite format 3` и
   `data/asledger.db.kdf.json`; книга открывается в web-интерфейсе.
5. Первый запуск с пустым файлом: создать `data/asledger.db` нулевого размера
   и запустить приложение. Пустой файл должен быть заменён зашифрованной
   книгой без ручного удаления; проверки те же, что в шаге 4.
6. Обновление с `v1.2.0`: положить plaintext-книгу в `data/asledger.db`,
   запустить приложение, ввести новый пароль в окне «Новый пароль для
   миграции». Должны появиться `.encrypted-backup.db`, его sidecar и
   `.kdf.json`; повторный запуск открывает книгу новым паролем.
7. В desktop-клиенте сменить пароль: старый пароль перестаёт открывать книгу,
   новый открывает, резервная копия `<книга>.pre-rekey-*.db` и sidecar
   существуют. Проверить, что при выключенном «Запомнить» сохранённый ключ
   удалён, а при включённом — заменён.
8. Проверить, что Go-сервер и Android-клиент из того же тега не
   регрессировали: `make check`, `make android-check` и APK из релиза.
9. Ручные проверки [#155](https://github.com/trefas/asledger/issues/155):
   миграция `1.2.0 → 1.3.x`, откат и Windows runtime остаются пропущенными по
   решению владельца проекта; автоматические проверки, включая первый запуск
   с отсутствующей и 0-байтной книгой, перечислены в
   [docs/testing.md](testing.md).

### Публикация `v1.3.2` (предыдущая)

Релиз опубликован workflow
[40185031350](https://github.com/trefas/asledger/actions/runs/40185031350):
все job зелёные. APK `asledger-1.3.2.apk` имеет SHA-256
`f8ed6cfc669660b2f2fd91ca5787b70e2aea919e1a413b380539b31701e2d42a`;
скачанные артефакты проверены `sha256sum -c` по `checksums.txt`,
`SHA256SUMS-linux`, `SHA256SUMS-windows` и `asledger-1.3.2.apk.sha256`.

## Публикация по тегу

```bash
git tag -a v1.0.0 -m "asledger v1.0.0"
git push origin v1.0.0
```

Тег запускает workflow `.github/workflows/release.yml`:

1. **Проверки** — `make check`, `make test-race`, `make build`; отдельный шаг
   проверяет, что собранная версия совпадает с тегом (`grep` по выводу
   `asledgerctl version` и `asledgerd --version`).
2. **Android** — job собирает `asledgerd` для JVM-интеграционных тестов, гоняет
   тесты, покрытие, lint и ktlint, собирает release APK с R8 и подписывает его
   ключом из секретов. `versionName` берётся из тега, `versionCode` считается
   как `major*10000 + minor*100 + patch`. Workflow проверяет наличие v1-, v2- и
   v3-подписей, а также `versionName`, `versionCode` и `minSdk` через
   `apksigner`/`aapt`; для v1 явно задаётся `--min-sdk-version 21`.
3. **Desktop** — две независимые джобы `desktop-linux` и `desktop-windows`
   собирают AppImage/deb для amd64 и arm64, NSIS/portable/ZIP для Windows,
   проверяют пакеты и готовят `SHA256SUMS-linux`/`SHA256SUMS-windows`.
   macOS в матрицу не входит.
4. **Публикация** — job с
   `needs: [test, android, desktop-linux, desktop-windows]`, поэтому релиз не
   создаётся при падающих тестах, неподписанном APK или сломанной упаковке
   desktop. GoReleaser в `goreleaser-cross` собирает серверные артефакты и
   публикует GitHub Release, после чего в него загружаются APK, desktop-пакеты
   и файлы контрольных сумм.

Секреты Android: `ANDROID_KEYSTORE_BASE64`, `ANDROID_KEYSTORE_PASSWORD`,
`ANDROID_KEY_ALIAS`, `ANDROID_KEY_PASSWORD`. Ключ хранится вне репозитория;
без него release job падает с явной ошибкой, а не публикует debug-подпись.

Релиз `v1.0.2` прошёл этим workflow: run
[39777402938](https://github.com/trefas/asledger/actions/runs/39777402938),
все три job зелёные. APK `asledger-1.0.2.apk` скачан из релиза, SHA-256
`014fa958c4dbe2bef0e2e4561a22d46c89f7af99b5684f3d11742ed1e6bba9c6`
совпал с `.sha256`, подпись проверена `apksigner`, установка поверх `1.0.1`
сохранила данные приложения.

Релиз `v1.0.3` проверен так же: run
[39786725105](https://github.com/trefas/asledger/actions/runs/39786725105),
SHA-256 APK
`e17c40b79ebf3ff7ff1c85cd9026fe78671d1a71a2c2c8d06fb87588241f6ae2`,
установка поверх `1.0.2` сохранила данные.

Релиз `v1.0.4` содержит v2- и v3-подписи: run
[39794526068](https://github.com/trefas/asledger/actions/runs/39794526068).
APK `asledger-1.0.4.apk` имеет размер `4 561 412` байт и SHA-256
`756cf995583e64cb9aa6925dcce3d253a54a8c03449cc9538b96ccc2d61fd809`;
скачанный файл прошёл `sha256sum -c`, `apksigner`, `zipalign` (4 КБ и 16 КБ)
и установлен через `adb` на Android 14 (API 34) поверх `1.0.3`.

Релиз `v1.0.5` уточняет точку входа лаунчера: run
[39803143482](https://github.com/trefas/asledger/actions/runs/39803143482).
APK `asledger-1.0.5.apk` имеет размер `4 561 412` байт и SHA-256
`0dd8e4159e937facc7acafc7e27620c55c76b2f24510f9c2d583383c4d38f1c9`;
`aapt` подтверждает `launchable-activity`, `apksigner` — v2 и v3,
`zipalign` — 4 КБ и 16 КБ. Установка на API 34 прошла успешно.

Релиз `v1.0.6` снижает `targetSdk` до 34: run
[39812643245](https://github.com/trefas/asledger/actions/runs/39812643245).
Фактическая причина ошибки на Huawei nova 14i (Android 12, API 31) была в
`minSdk 34`, а не в `targetSdk`; это исправлено в `1.1.2`. APK
`asledger-1.0.6.apk` имеет размер `4 561 412` байт и SHA-256
`e59adaec195fe72b57c33dd450676d9d8d819c4ead36b75a28c41edc31c780e7`;
`aapt` подтверждает `targetSdkVersion:'34'` и `launchable-activity`,
`apksigner` — v2 и v3, `zipalign` — 4 КБ и 16 КБ. Установка на API 34 прошла
успешно.

Релиз `v1.0.7` снижает `compileSdk` до минимально возможного 35: run
[39824026596](https://github.com/trefas/asledger/actions/runs/39824026596).
APK `asledger-1.0.7.apk` имеет размер `4 561 412` байт и SHA-256
`4f1221e4c6b1c2bf27091888f7fa299f3632dd7069a124f9b68c6b62e01be6d3`;
`aapt` подтверждает `compileSdkVersion:'35'`, `targetSdkVersion:'34'` и
`launchable-activity`, `apksigner` — v2 и v3, `zipalign` — 4 КБ и 16 КБ.
Установка на API 34 прошла успешно.

Релиз `v1.1.0` выносит все пользовательские строки Android-клиента в ресурсы
и закрывает S16: run
[39831850681](https://github.com/trefas/asledger/actions/runs/39831850681).
APK `asledger-1.1.0.apk` имеет размер `4 690 568` байт и SHA-256
`f89efc408aee348f0fcbf15372009d7b767bc0e9a628b77c227dd561fa4a87a1`.
Скачанный файл прошёл `sha256sum -c`, `apksigner` (v2 и v3), `zipalign`
(4 КБ и 16 КБ) и установлен на эмулятор API 34 поверх `1.0.7` с сохранением
данных. `aapt` подтверждает `versionName=1.1.0`, `versionCode=10100`,
`minSdk=34`, `targetSdk=34`, `compileSdk=35` и `launchable-activity`.

Релиз `v1.1.1` добавляет в release-подпись схему v1 (JAR) в дополнение к v2
и v3 (`enableV1Signing = true`) для проверки гипотезы о нестандартном
установщике EMUI/HMS. Сборки до `1.1.0` включительно содержали только блоки
v2/v3 без JAR-манифеста. На Android 14 штатная проверка использует v2/v3,
поэтому [#135](https://github.com/trefas/asledger/issues/135) остаётся открытым
до успешной установки APK из файлового менеджера Huawei и проверки запуска.
Workflow дополнительно проверяет v1-подпись, наличие `META-INF/MANIFEST.MF`,
`launchable-activity` и выравнивание готового APK перед публикацией.

Релиз опубликован workflow
[39843998616](https://github.com/trefas/asledger/actions/runs/39843998616).
APK `asledger-1.1.1.apk` имеет размер `4 702 387` байт и SHA-256
`a67cc16a4fb30157de83afa64891f9a8bcc93eac79a56a61c8f3cccc02a6af4d`.
`apksigner --min-sdk-version 21` подтверждает v1, v2 и v3, `zipalign`
(4 КБ и 16 КБ) проходит, `aapt` видит `launchable-activity`, `versionName=1.1.1`
и `versionCode=10101`. Скачанный APK установлен на эмулятор API 34, данные
приложения сохранились.

На Huawei nova 14i `1.1.1` не установился: `adb install` вернул
`INSTALL_FAILED_OLDER_SDK` (требуется API 34, устройство работает на API 31).
Поэтому v1-подпись не была причиной #135.

Релиз `v1.1.2` снижает `minSdk` до 31 и оставляет `targetSdk` 34. На Huawei
nova 14i (Android 12, API 31) APK устанавливается, приложение запускается,
иконка `asledger` видна в шторке EMUI Launcher. На эмуляторе API 34 сборка
также устанавливается и запускается.

Опубликован [v1.1.2](https://github.com/trefas/asledger/releases/tag/v1.1.2),
workflow
[39856818274](https://github.com/trefas/asledger/actions/runs/39856818274).
APK `asledger-1.1.2.apk` имеет размер `4 718 767` байт и SHA-256
`ae2cc4cc6e9d9afa91a5879372ab058c1e6e4635c4f6337b06ca9744fc49797d`;
`aapt` подтверждает `versionName=1.1.2`, `versionCode=10102`,
`sdkVersion:'31'`, `targetSdkVersion:'34'` и `launchable-activity`,
`apksigner --min-sdk-version 21` — v1, v2 и v3, `zipalign` — 4 КБ и 16 КБ.

Release notes собираются автоматически из истории коммитов (GoReleaser,
`changelog`): группы «Новые возможности», «Исправления», «Производительность»,
«Документация» по conventional-префиксам `feat:`, `fix:`, `perf:`, `docs:`;
остальные коммиты попадают в «Прочие изменения». Слияния исключаются.

Конфигурация релиза проверяется в обычном CI (`goreleaser check`), поэтому
поломка `.goreleaser.yaml` видна до тега.

## Воспроизводимость

- версия Go фиксируется в `go.mod` и используется CI через
  `go-version-file`;
- `CGO_ENABLED=1`, фиксированные теги и `-trimpath` задают сборку SQLCipher;
- версия `goreleaser-cross` закреплена и содержит Go 1.26.10 и нужные
  кросс-компиляторы;
- тесты и статические проверки выполняются тем же `make`, что и локально;
- проверка соответствия версии тегу не даёт выпустить артефакт с чужой версией.
