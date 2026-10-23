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
пайплайна. Сборка статическая (`CGO_ENABLED=0`, `-trimpath`).

| Платформа | Архив | Бинарники |
| --- | --- | --- |
| linux/amd64 | `asledger_<версия>_linux_amd64.tar.gz` | `asledgerd`, `asledgerctl` |
| linux/arm64 | `asledger_<версия>_linux_arm64.tar.gz` | `asledgerd`, `asledgerctl` |
| windows/amd64 | `asledger_<версия>_windows_amd64.zip` | `asledgerd.exe`, `asledgerctl.exe` |
| Android 12+ | `asledger-<версия>.apk` | приложение asledger |

В именах архивов версия печатается без префикса `v` (`asledger_1.0.0_…`), а в
самих бинарниках — как в теге (`asledgerd v1.0.0`): GoReleaser подставляет в
`-ldflags` значение `.Tag`.

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
sha256sum -c asledger-1.1.2.apk.sha256
```

## Desktop-клиент (Electron)

Desktop-клиент собирается отдельно из `desktop/` и кладёт в пакет свой
`asledgerd`, ассеты и шаблоны. Версия синхронизируется с тегом:

```bash
cd desktop
ASLEDGER_VERSION="${GITHUB_REF_NAME}" npm run sync-version
ASLEDGER_VERSION="${GITHUB_REF_NAME}" npm run verify:version
npm run package:dist
```

Артефакты: Linux AppImage/deb (amd64 и arm64), Windows NSIS/portable (amd64).
Для кросс-сборки Windows без wine доступен ZIP (`npm run package:win:zip`).
macOS в MVP `v1.2.0` не входит: нет сертификата и нотаризации. Windows и macOS
артефакты пока не подписываются; подпись — отдельная задача.

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
3. **Публикация** — job с `needs: [test, android]`, поэтому релиз не создаётся
   при падающих тестах или неподписанном APK. GoReleaser собирает артефакты и
   публикует GitHub Release, после чего в него загружаются APK и `.sha256`.

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
- `CGO_ENABLED=0` и `-trimpath` убирают зависимость от окружения сборки;
- тесты и статические проверки выполняются тем же `make`, что и локально;
- проверка соответствия версии тегу не даёт выпустить артефакт с чужой версией.
