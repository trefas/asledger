package com.trefas.asledger.data.settings

/**
 * Адрес книги на сервере.
 *
 * Хост хранится без схемы и пути, порт — отдельно. По умолчанию используется
 * порт API 8778; web-интерфейс на 8777 приложению не нужен.
 */
data class ServerConfig(
    val host: String,
    val port: Int = DEFAULT_PORT,
) {
    /** Базовый URL API без завершающего слэша. */
    val baseUrl: String
        get() = if (host.contains(':')) {
            "http://[$host]:$port"
        } else {
            "http://$host:$port"
        }

    companion object {
        /** Порт API по умолчанию (web-интерфейс слушает 8777). */
        const val DEFAULT_PORT: Int = 8778

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
