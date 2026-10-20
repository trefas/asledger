package com.trefas.asledger.data.settings

import java.util.UUID

/**
 * Адрес книги и состояние привязки устройства.
 *
 * Хост хранится без схемы и пути, порт — отдельно. По умолчанию используется
 * порт API 8778; web-интерфейс на 8777 приложению не нужен. Секрет (токен) в
 * этой модели не хранится: он лежит только в зашифрованном виде в Keystore.
 */
data class ServerConfig(
    val host: String,
    val port: Int = DEFAULT_PORT,
    val clientId: String = "",
    val userId: Long = 0,
    val userName: String = "",
    val serverVersion: String = "",
    val formatVersion: Int = 0,
    val onboarded: Boolean = false,
    val lastSyncAt: Long = 0
) {
    /** Базовый URL API без завершающего слэша. */
    val baseUrl: String
        get() = if (host.contains(':')) {
            "http://[$host]:$port"
        } else {
            "http://$host:$port"
        }

    /** Устройство привязано к пользователю и готово к синхронизации. */
    val hasSession: Boolean
        get() = clientId.isNotBlank() && userId > 0

    companion object {
        /** Порт API по умолчанию (web-интерфейс слушает 8777). */
        const val DEFAULT_PORT: Int = 8778

        /** Новый постоянный идентификатор устройства. */
        fun newClientId(): String = UUID.randomUUID().toString()

        /** Приводит ввод пользователя к хосту без схемы и пути. */
        fun normalizeHost(raw: String): String {
            var value = raw.trim()
                .removePrefix("http://")
                .removePrefix("https://")
            value = value.substringBefore('/').substringBefore('?').substringBefore('#')
            // IPv6 в скобках: «[::1]:8778» → «::1».
            if (value.startsWith("[")) {
                val end = value.indexOf(']')
                if (end > 0) {
                    return value.substring(1, end)
                }
            }
            // «192.168.1.5:8778» в поле адреса: порт указан отдельно.
            if (value.count { it == ':' } == 1) {
                value = value.substringBefore(':')
            }
            return value.trim()
        }
    }
}
