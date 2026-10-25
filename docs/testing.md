# Конвенции тестирования asledger

Документ описывает обязательные правила написания тестов. Они нужны, чтобы
выполнялось требование эпика: **один автотест на каждый чекбокс критериев
приёмки и каждую ячейку матрицы правил**.

## Стек

- Стандартный пакет `testing`. Внешние библиотеки (например, `testify`)
  добавляются только при явной необходимости и с обоснованием в PR.
- Табличные тесты — предпочтительная форма для перебора вариантов.
- Тесты живут рядом с кодом в файлах `*_test.go`.

## Именование

- Трассируемые тесты критериев приёмки: `TestASL_<раздел>_<пункт>`.
  Пример: `TestASL_14_money_123_45` — критерий «сумма 123.45 хранится как 12345».
- Внутренние тесты модулей: `Test<Что>` (`TestParse`, `TestString`).
- Подтесты — через `t.Run` с русским описанием сценария.

Имя трассируемого теста позволяет сопоставить его с чекбоксом критериев эпика
без дополнительных таблиц.

## Обязательный минимум

- Каждый чекбокс раздела «Критерии приёмки» эпика покрыт хотя бы одним тестом.
- Каждая ячейка матрицы правил ASL-2.2 (тип операции × состав строк) имеет
  положительный и отрицательный тест.
- Каждый инвариант (Σ DEBIT = Σ CREDIT, только листовые счета, положительные
  суммы, диапазон даты, `min_balance`, служебные счета) проверяется отдельно.
- Регрессия оформляется тестом до исправления.

## Изоляция и хелперы

- Временные каталоги — `t.TempDir()`, переменные окружения — `t.Setenv()`.
- Помощники помечаются `t.Helper()`, чтобы в отчёте была строка вызова.
- Тесты не зависят от порядка выполнения и от данных реальной книги.
- Фикстуры базы данных и плана счетов появятся вместе со слоем данных (веха S1)
  в общем пакете тестовых помощников.

## Деньги

- Денежная арифметика — только целые копейки. Типы `float32`/`float64` в
  денежном коде запрещены и проверяются целью `make lint-money`.
- Эталонное значение: `123.45` ⇄ `12345` копеек.

## Запуск

```bash
make test        # все тесты
make test-race   # тесты с детектором гонок
make cover       # покрытие
make cover-check # общий порог покрытия (COVERAGE_MIN, по умолчанию 70 %)
make check       # форматирование, статические проверки и тесты
```

## Трассировка критериев

Карта «критерий приёмки эпика (раздел 14) → автотест» — в
[traceability.md](traceability.md). Её проверяет `scripts/check-traceability.sh`
(цель `make lint-trace`, входит в `make check`): CI падает, если критерий не
имеет теста или ссылается на несуществующий тест. Непокрытый критерий
помечается в карте как `план: #N` с ссылкой на закрывающую задачу.

### Интеграционные тесты

Стенд вехи S7 живёт в `internal/integration` и переиспользуется всеми
интеграционными тестами: сервер на случайном порту поверх временной книги
(`newStand`) и независимые клиенты с локальными репликами (`newReplica`).
Тесты ходят только по localhost и детерминированы.

### Проверки релиза

`internal/release` статически проверяет артефакты выпуска: конфигурацию
GoReleaser (платформы, архивы, SHA-256, шаблоны и ассеты, `-ldflags`),
workflow публикации по тегу (`needs: test`), systemd-юнит и инструкции по
установке. Это даёт автотесты для критериев раздела 14, которые иначе
проверялись бы только вручную.

### Шифрование SQLCipher в `v1.3.0`–`v1.3.1`

Проверки, которые закрывают требования #148–#154:

| Что проверяется | Тест/команда | Результат |
| --- | --- | --- |
| Книга не содержит plaintext-маркеров, заголовок не `SQLite format 3` | `pkg/core/store/store_test.go`, `TestOpenEncryptedRoundTrip` | проходит |
| Зашифрованная книга без ключа даёт `ErrKeyRequired` | `TestOpenEncryptedRequiresKey` | проходит |
| Неверный ключ не повреждает книгу | `TestOpenWrongKey` | проходит |
| Перешифровка и `sqlcipher_export` | `pkg/core/store/export_test.go` | проходит |
| Миграция plaintext → SQLCipher, backup, sidecar | `cmd/asledgerctl/encrypt_test.go`, `TestMigrateEncrypt*` | проходит |
| Неудачная миграция не меняет исходную книгу и не создаёт sidecar | `TestMigrateEncryptBackupExists`, `TestMigrateEncryptBackupWriteFails` | проходит |
| `migrate-encrypt -dry-run` не меняет схему и не оставляет WAL | `TestMigrateEncryptDryRun` | проходит |
| Смена пароля, неверный старый пароль, зашифрованный backup | `cmd/asledgerctl/encrypt_test.go`, `TestPasswd*`, `TestBackupEncrypted` | проходит |
| Argon2id, sidecar, источники пароля/fd/raw-ключа | `internal/key/key_test.go` | проходит |
| Пароль не попадает в argv/env и журнал | `desktop/tests/backend.test.ts`, `desktop/tests/password-change.test.ts` | проходит |
| Electron: первый запуск, смена пароля, старый пароль, backup | `desktop/tests/e2e/desktop.spec.ts` | проходит на Linux |

Команды и результаты на 2026-10-25 (Linux amd64):

```bash
make check          # go vet, тесты, проверки денег и трассировки — успешно
make test-race      # go test -race ./... — успешно
make cover-check    # покрытие 74.3 % при пороге 70 %
cd desktop
npm run check       # typecheck, 61 unit-тест, production-сборка — успешно
npm run test:e2e    # Electron smoke + смена пароля — 1 тест, успешно
```

Ограничения: Windows runtime SQLCipher и desktop-пакет локально не проверялись;
Android JVM-тест обновлён, но в среде разработки нет JDK/Android SDK, поэтому
локальный прогон не выполнялся. Эти проверки остаются в CI и в ручном
чек-листе релиза.

### Обновление SQLCipher в S24 (#160)

Проверки нового драйвера `github.com/meow-io/go-sqlcipher v1.1.2`
(SQLCipher 4.19.0, SQLite 3.53.4):

| Что проверяется | Тест/команда | Результат |
| --- | --- | --- |
| Книга, созданная SQLCipher 4.4.2, открывается, читается, мигрирует и экспортируется | `pkg/core/store/compat_test.go`, `TestOpenBookFromSQLCipher442` | проходит |
| Книга 4.19.0 открывается драйвером 4.4.2 с тем же raw-key | ручная проверка `sqlcipher-compat` | проходит |
| Версии и параметры книги | `PRAGMA cipher_version`, `select sqlite_version()`, `PRAGMA cipher_page_size`, `kdf_iter` | 4.19.0 / 3.53.4 / 4096 / 256000 |
| Полный набор тестов проекта | `CGO_ENABLED=1 go test ./...` | проходит |
| Кросс-сборка Linux amd64/arm64 и Windows amd64 | `goreleaser-cross:v1.26.10`, `-extldflags=-static` | проходит |
| Отсутствие динамического OpenSSL/libcrypto | `ldd` и `objdump -p` | Linux статический; Windows: ADVAPI32/KERNEL32/msvcrt |

Команды на 2026-10-25 (Linux amd64):

```bash
CGO_ENABLED=1 go test -count=1 -run TestOpenBookFromSQLCipher442 -v ./pkg/core/store
make check
make test-race
make cover-check
cd desktop
npm run check
npm run test:e2e
npm run package:dist
npm run verify:package
```

Результаты на Linux amd64: `make check` и `make test-race` успешно,
покрытие 74.3 %, desktop — 61 unit-тест, E2E — 1 тест, пакет и
`verify:package` успешно. Ограничение: Windows runtime по-прежнему
проверяется только в CI; локального Windows/Wine окружения нет.
Android-клиент не менялся, его JVM-тест остаётся за CI.
