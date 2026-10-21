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

# Kotlinx.serialization: сериализаторы DTO создаются плагином, но имена
# свойств и аннотации нужны для чтения snake_case-контракта сервера.
-keepattributes Signature, RuntimeVisibleAnnotations
-keepclassmembers @kotlinx.serialization.Serializable class ** {
    *** Companion;
    *** serializer(...);
}
-keepclasseswithmembers class **$$serializer {
    *;
}

# Ktor и OkHttp не используют reflection для моделей, но их точки входа
# вызываются из корутин и должны пережить обфускацию.
-dontwarn io.ktor.**
-dontwarn okhttp3.**
-dontwarn okio.**
-keep class io.ktor.client.engine.okhttp.** { *; }
-keepclassmembers class io.ktor.client.plugins.contentnegotiation.** { *; }

# Room генерирует реализации DAO и базы; их имена нужны Room.
-keep class * extends androidx.room.RoomDatabase { *; }
-keep @androidx.room.Entity class * { *; }
-keep class androidx.room.** { *; }
