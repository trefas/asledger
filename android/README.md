# Android-приложение asledger

Мобильный клиент книги asledger. Совместимость — **Android 14 (API 34) и новее**.
Исходники сервера и документация — в корне репозитория.

## Что уже есть

- каркас Gradle + Kotlin + Jetpack Compose (Material 3), один модуль `:app`;
- фирменная тема, логотип, adaptive icon, splash;
- навигация: подключение, журнал, ввод, план счетов, настройки;
- экран подключения с адресом и портом (по умолчанию `8778`);
- DataStore для адреса сервера, HTTP-клиент Ktor, контейнер зависимостей;
- unit-тесты нормализации адреса, Compose-тест нижней навигации (Robolectric)
  и CI-сборка debug APK.

Экраны журнала, ввода и плана счетов показывают пустые состояния: данные
появятся в вехах S11–S13. Серверная привязка без пароля
(`POST /api/v1/auth/enroll`) и роли движка (`GET /api/v1/engine/roles`) уже
реализованы.

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
    AppContainer.kt       ручной DI: DataStore, Ktor
    settings/             адрес и порт сервера
  ui/
    components/           логотип, каркас экрана, пустые состояния
    navigation/           маршруты и нижняя навигация
    screens/              подключение, журнал, ввод, счета, настройки
    theme/                палитра, типографика, форма
  util/Log.kt             логирование только в debug, без токенов и сумм
```

## Маршруты

| Маршрут | Экран | Когда открывается |
| --- | --- | --- |
| `setup` | подключение | нет сохранённого адреса |
| `journal` | журнал | стартовый после подключения |
| `entry` | новая проводка | кнопка «+» в журнале и нижняя навигация |
| `accounts` | план счетов | нижняя навигация |
| `settings` | настройки | нижняя навигация |
| `sync-status` | синхронизация | из настроек |

Стартовый маршрут выбирается по наличию адреса в DataStore. Нижняя навигация
скрыта на экране подключения и использует `saveState`/`restoreState`, поэтому
системная кнопка «Назад» возвращает к предыдущему разделу, а не закрывает
приложение сразу.

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
`assembleDebug testDebugUnitTest lintDebug` и прикладывает APK как артефакт.
Релизный APK будет подписан и добавлен в GitHub Release в вехе S15.

## Границы первой версии

Нет iOS, Google Play, редактирования плана счетов с телефона, отчётов и
сверки, мультивалютности, push-уведомлений, биометрии и нескольких книг.
Транспорт — HTTP в доверенной локальной сети; HTTPS/VPN остаются за рамками.
