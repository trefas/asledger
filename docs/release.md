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
| Android 14+ | `asledger-<версия>.apk` | приложение asledger |

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
sha256sum -c asledger-1.0.3.apk.sha256
```

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
   как `major*10000 + minor*100 + patch`. Workflow проверяет подпись,
   `versionName`, `versionCode` и `minSdk` через `apksigner`/`aapt`.
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
