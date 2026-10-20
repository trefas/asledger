# Android-приложение asledger

Мобильный клиент книги asledger. Совместимость — **Android 14 (API 34) и новее**.
Исходники сервера и документация — в корне репозитория.

## Что уже есть

- каркас Gradle + Kotlin + Jetpack Compose (Material 3), один модуль `:app`;
- фирменная тема, логотип, adaptive icon, splash;
- навигация: подключение, привязка, журнал, ввод, план счетов, настройки;
- подключение по адресу и порту (по умолчанию `8778`) с проверкой
  `/api/v1/health`, версии и `format_version`;
- привязка без пароля через `POST /api/v1/auth/enroll`: один пользователь —
  автоматически, несколько — выбор из списка имён;
- токен устройства в Android Keystore (AES/GCM), в DataStore только
  шифротекст; при `401` — повторная привязка;
- понятные ошибки: DNS, отказ соединения, таймаут, HTTP 4xx/5xx, чужой сервис;
- экран «Диагностика»: адрес, версия сервера, формат, пользователь, время
  последней синхронизации; токен не показывается;
- DataStore для адреса, `client_id`, пользователя и метаданных; HTTP-клиент
  Ktor с таймаутами и ограниченными повторами;
- unit-тесты API на MockEngine, разбора ошибок и DataStore, Compose-тесты
  подключения, выбора пользователя и нижней навигации (Robolectric).

Экраны журнала, ввода и плана счетов показывают пустые состояния: данные
появятся в вехах S11–S13. Серверные роли движка
(`GET /api/v1/engine/roles`) уже реализованы.

## Требования

- JDK 21;
- Android SDK: платформа `android-36`, build-tools `36.0.0`;
- переменная `ANDROID_HOME` или файл `local.properties` с `sdk.dir=…`
  (файл не коммитится).

`minSdk 34`, `compileSdk`/`targetSdk` 36.

## Сборка

```bash
cd android
./gradlew :app:assembleDebug        # debug APK
./gradlew :app:testDebugUnitTest    # unit-тесты и Compose-тест навигации
./gradlew :app:lintDebug            # Android Lint
./gradlew :app:ktlintCheck          # стиль Kotlin
```

Из корня репозитория:

```bash
make android-check                  # сборка + тесты + lint + ktlint
make android-debug                  # только debug APK
```

Debug APK: `android/app/build/outputs/apk/debug/app-debug.apk`.
Пакет debug-сборки — `com.trefas.asledger.debug`, release — `com.trefas.asledger`,
поэтому обе версии можно держать на устройстве одновременно.

## Структура

```
app/src/main/java/com/trefas/asledger/
  AsledgerApp.kt          Application и контейнер
  MainActivity.kt         единственная Activity
  data/
    AppContainer.kt       ручной DI: DataStore, Keystore, Ktor, API
    network/              клиент API, модели, разбор ошибок подключения
    security/             токен в Android Keystore (AES/GCM)
    settings/             адрес, пользователь и метаданные в DataStore
  ui/
    components/           логотип, каркас экрана, пустые состояния
    navigation/           маршруты и нижняя навигация
    screens/              подключение, привязка, диагностика, журнал, ввод,
                          счета, настройки
    theme/                палитра, типографика, форма
  util/                   логирование в debug, имя устройства, даты
```

## Маршруты

| Маршрут | Экран | Когда открывается |
| --- | --- | --- |
| `setup` | подключение | нет сохранённого адреса или «Изменить адрес» |
| `enroll` | привязка устройства | адрес сохранён, пользователь ещё не выбран |
| `journal` | журнал | стартовый после привязки |
| `entry` | новая проводка | кнопка «+» в журнале и нижняя навигация |
| `accounts` | план счетов | нижняя навигация |
| `settings` | настройки | нижняя навигация |
| `diagnostics` | диагностика | из настроек |

Стартовый маршрут выбирается по DataStore: нет адреса — `setup`, нет
пользователя — `enroll`, иначе `journal`. Нижняя навигация скрыта на
`setup`/`enroll` и использует `saveState`/`restoreState`, поэтому системная
кнопка «Назад» возвращает к предыдущему разделу, а не закрывает приложение
сразу. После `401` приложение открывает `enroll` повторно; адрес и черновики
сохраняются.

## Бренд

Логотип и иконки берутся из общего каталога `assets/logo/`. После обновления
логотипа:

```bash
android/scripts/gen-icons.sh
```

Скрипт пересобирает `mipmap-*` из `assets/logo/icon-512.png` (нужен
ImageMagick). Готовые PNG хранятся в репозитории, чтобы CI не зависел от
ImageMagick.

## Проверки

CI-джоба `Android (debug APK)` на каждый push запускает
`assembleDebug testDebugUnitTest lintDebug ktlintCheck` и прикладывает APK как
артефакт. Релизный APK будет подписан и добавлен в GitHub Release в вехе S15.

## Границы первой версии

Нет iOS, Google Play, редактирования плана счетов с телефона, отчётов и
сверки, мультивалютности, push-уведомлений, биометрии и нескольких книг.
Транспорт — HTTP в доверенной локальной сети; HTTPS/VPN остаются за рамками.
