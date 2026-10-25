# Android-клиент asledger

Мобильный клиент для Android 12 (API 31) и новее. Один модуль `:app` на Kotlin,
Jetpack Compose (Material 3), Room, Ktor и WorkManager. Сервер — `asledgerd` из
корня репозитория; протокол и границы версии описаны в
[../docs/android.md](../docs/android.md), REST API — в
[../docs/api.md](../docs/api.md).

## Требования

| Компонент | Версия |
| --- | --- |
| JDK | 21 |
| Android SDK | platform 35/36, build-tools 35.0.0/36.0.0 |
| Gradle | через wrapper (`./gradlew`) |
| Kotlin | 2.1.20 |
| compileSdk | 35 |
| minSdk / targetSdk | 31 / 34 |

Переменные окружения:

```bash
export JAVA_HOME=/path/to/jdk-21
export ANDROID_HOME=/path/to/android-sdk
```

`local.properties` с `sdk.dir` создаётся Android Studio или вручную; он не
коммитится.

## Быстрый старт

```bash
# из корня репозитория
make build                 # bin/asledgerd и bin/asledgerctl
# В TTY asledgerd спросит пароль книги; для скриптов передайте
# -password-file или -password-fd, но не пароль в argv.
./bin/asledgerd -db /tmp/asledger-dev.db

# в другом терминале
cd android
./gradlew :app:assembleDebug
adb install -r app/build/outputs/apk/debug/app-debug.apk
```

При первом запуске сервера откройте `http://localhost:8777`, создайте
пользователя и импортируйте план счетов. Телефон и компьютер должны быть в
одной локальной сети; в приложении укажите IP компьютера и порт API `8778`.

Для эмулятора адрес компьютера — `10.0.2.2`, порт `8778`.

## Структура

```
app/src/main/java/com/trefas/asledger/
├── AsledgerApp.kt        Application, создаёт AppContainer
├── MainActivity.kt       единственная Activity, Compose
├── data/
│   ├── db/               Room: сущности, DAO, миграции
│   ├── model/            доменные модели и статусы
│   ├── network/          Ktor-клиент, DTO, ошибки соединения
│   ├── repository/       Flow-запросы, черновики, роли, остатки
│   ├── security/         Android Keystore для токена
│   ├── settings/         DataStore: адрес, сессия, онбординг
│   └── sync/             SnapshotImporter, SyncApplier, SyncManager, Worker
├── ui/
│   ├── components/       общие Composable: каркас, бренд
│   ├── navigation/       NavHost и нижняя навигация
│   ├── screens/          экраны и ViewModel
│   └── theme/            цвета, типографика, темы
└── util/                 деньги, даты, строковые списки
```

Соглашения:

- зависимости создаются в `AppContainer` и передаются через ViewModel-фабрики;
  глобальных mutable-синглтонов нет;
- ViewModel хранит `StateFlow<...UiState>`, Composable не ходит в сеть и Room;
- пользовательские тексты — только в `res/values/strings.xml`; в Compose и
  слое данных нет русских литералов;
- слой данных не зависит от `Context`: ошибки, прогресс и названия статусов
  передаются как `@StringRes`, а UI разрешает их через `stringResource`;
- деньги — только `Long` в копейках, без `Float`/`Double`; форматирование — в
  `util/MoneyFormat.kt`;
- даты — `Long` epoch millis, форматирование — в `util/ServerTime.kt`;
  относительные подписи выбирает UI через `FriendlyDate`;
- сетевые и бизнес-ошибки не выбрасываются в UI, а возвращаются
  `ApiResult`/`ConnectionError`; `ConnectionProblem` хранит ресурсы заголовка и
  подсказки.

## Тесты и проверки

```bash
cd android
./gradlew :app:testDebugUnitTest
./gradlew :app:verifyDataCoverage    # порог 70 % строк data/**
./gradlew :app:lintDebug
./gradlew :app:ktlintCheck
./gradlew :app:assembleDebug
./gradlew :app:assembleRelease
```

Из корня: `make android-check` запускает debug APK, unit-тесты, lint и ktlint.

JVM-тесты на Robolectric работают без эмулятора. Для UI-тестов Compose
используется `createComposeRule`; Room в тестах создаётся с прямыми
query/transaction executors, чтобы suspend-запросы были детерминированы.

### Интеграционные тесты с сервером

`com.trefas.asledger.integration.AndroidServerIntegrationTest` запускает
настоящий `asledgerd` на случайных портах, передаёт ему пароль через файл
(книга шифруется SQLCipher), настраивает книгу через web-форму и проходит
сценарии двух клиентов, rebase, rejected и resync. Android-клиент не получает
пароль: он работает через API с токеном устройства. Внешняя сеть не
используется.

```bash
make build
cd android
ASLEDGERD_BIN=../bin/asledgerd ./gradlew :app:testDebugUnitTest \
  --tests 'com.trefas.asledger.integration.*'
```

Без `ASLEDGERD_BIN` тест пропускается. В CI сервер собирается перед Gradle.

## Сборка release APK

Ключ и пароли не коммитятся. Локально создайте
`android/keystore.properties`:

```properties
storeFile=/absolute/path/asledger-release.jks
storePassword=…
keyAlias=asledger
keyPassword=…
```

или задайте `ANDROID_KEYSTORE_PATH`, `ANDROID_KEYSTORE_PASSWORD`,
`ANDROID_KEY_ALIAS`, `ANDROID_KEY_PASSWORD`. Затем:

```bash
./gradlew :app:assembleRelease
```

Без ключа release-сборка подписывается debug-ключом и печатает
предупреждение — такой APK нельзя публиковать.

`versionName` по умолчанию `1.3.1`, `versionCode` — `10301`. В CI они
переопределяются из тега: `ANDROID_VERSION_NAME`, `ANDROID_VERSION_CODE`.
Release workflow проверяет подпись и версию через `apksigner`/`aapt`, публикует
APK и `.sha256`.

## Локальный сервер и сброс данных

```bash
# сервер с отдельной книгой и портами; в TTY он спросит пароль книги
./bin/asledgerd -db /tmp/asledger-dev.db \
  -api-addr 127.0.0.1:8778 -web-addr 127.0.0.1:8777

# сброс данных приложения (эмулятор/устройство)
adb shell pm clear com.trefas.asledger
adb shell pm clear com.trefas.asledger.debug
```

Очистка данных приложения удаляет DataStore, Room и ключ токена. Книга на
сервере не меняется; после сброса устройство нужно привязать заново.

## Диагностика

| Симптом | Что проверить |
| --- | --- |
| «Сервер не отвечает» | запущен ли `asledgerd`, порт `8778`, брандмауэр |
| «Адрес не найден» | IP/имя компьютера, одна ли сеть у телефона и ПК |
| «Обновите asledgerd» | `format_version` сервера должен быть `1` |
| 401 после синхронизации | устройство отозвано; привяжите заново, черновики сохранятся |
| Проводка отклонена | откройте «Синхронизация» → «Исправить», проверьте счёт и роль |
| Release APK не ставится поверх | он подписан другим ключом; используйте ключ из секретов CI |

Логи: `adb logcat | grep -i asledger`. Токен в журналы не пишется.

## Известные ограничения

- Поддерживается только `format_version = 1`.
- План счетов на телефоне только для чтения.
- Нет отчётов, сверки, импорта hledger, мультивалютности и push-уведомлений.
- Интерфейс только на русском; все пользовательские строки находятся в `res/values/strings.xml`.
- HTTPS/VPN не настроены: предполагается доверенная домашняя сеть.

Вклад в проект и обязательные проверки — в [../CONTRIBUTING.md](../CONTRIBUTING.md).
