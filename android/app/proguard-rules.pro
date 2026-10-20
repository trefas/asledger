# Правила ProGuard для release-сборки.
#
# kotlinx.serialization генерирует сериализаторы; Ktor использует reflection
# только для OkHttp. Основные модели размечены @Serializable, их правила
# подключает сам плагин. Здесь — запас для будущих DTO и отладочных имён.
-keepattributes *Annotation*, InnerClasses
-dontnote kotlinx.serialization.**

-keepclassmembers class kotlinx.serialization.json.** {
    *** Companion;
}
-keepclasseswithmembers class kotlinx.serialization.json.** {
    kotlinx.serialization.KSerializer serializer(...);
}

# Сохраняем имена классов приложения в отчётах о падениях.
-keepnames class com.trefas.asledger.** { *; }
