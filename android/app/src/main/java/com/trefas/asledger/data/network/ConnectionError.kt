package com.trefas.asledger.data.network

import io.ktor.client.network.sockets.ConnectTimeoutException
import io.ktor.client.plugins.HttpRequestTimeoutException
import java.io.IOException
import java.net.ConnectException
import java.net.NoRouteToHostException
import java.net.UnknownHostException
import javax.net.ssl.SSLException

/** Классы ошибок подключения, которые важно различать пользователю. */
sealed interface ConnectionError {
    /** Некорректный адрес или порт. */
    data class InvalidAddress(val detail: String) : ConnectionError

    /** Сеть недоступна или соединение оборвалось. */
    data object NetworkUnavailable : ConnectionError

    /** Имя хоста не разрешается. */
    data object HostNotFound : ConnectionError

    /** Порт закрыт или сервер не запущен. */
    data object ConnectionRefused : ConnectionError

    /** Сервер не ответил за отведённое время. */
    data object Timeout : ConnectionError

    /** Токен отозван или недействителен. */
    data object Unauthorized : ConnectionError

    /** Сервер ответил HTTP-ошибкой. */
    data class Http(
        val status: Int,
        val code: String? = null,
        val message: String? = null
    ) : ConnectionError

    /** По адресу отвечает не asledger (например, web-порт 8777). */
    data object NotAsledger : ConnectionError

    /** Ответ пришёл, но его структура не совпала с контрактом. */
    data class InvalidResponse(val detail: String = "") : ConnectionError

    /** Формат снапшота несовместим с приложением. */
    data class IncompatibleFormat(
        val serverFormat: Int,
        val supportedFormat: Int
    ) : ConnectionError

    /** Непредвиденная ошибка; detail не содержит секретов. */
    data class Unexpected(val detail: String) : ConnectionError
}

/** Понятное пользователю объяснение ошибки и подсказка. */
data class ConnectionProblem(
    val title: String,
    val hint: String
)

/** Преобразует ошибку в текст для экрана. */
fun ConnectionError.toProblem(): ConnectionProblem = when (this) {
    is ConnectionError.InvalidAddress -> ConnectionProblem(
        title = "Проверьте адрес",
        hint = detail
    )

    ConnectionError.NetworkUnavailable -> ConnectionProblem(
        title = "Нет связи с сервером",
        hint = "Проверьте Wi-Fi и убедитесь, что телефон и компьютер в одной сети."
    )

    ConnectionError.HostNotFound -> ConnectionProblem(
        title = "Адрес не найден",
        hint = "Проверьте IP или имя компьютера. Имя должно разрешаться в локальной сети."
    )

    ConnectionError.ConnectionRefused -> ConnectionProblem(
        title = "Сервер не отвечает",
        hint = "Запущен ли asledgerd? Проверьте порт API — по умолчанию 8778."
    )

    ConnectionError.Timeout -> ConnectionProblem(
        title = "Сервер не ответил вовремя",
        hint = "Проверьте сеть и брандмауэр компьютера, затем попробуйте ещё раз."
    )

    ConnectionError.Unauthorized -> ConnectionProblem(
        title = "Устройство отвязано",
        hint = "Токен отозван или устарел. Привяжите устройство заново."
    )

    is ConnectionError.Http -> ConnectionProblem(
        title = "Сервер вернул ошибку",
        hint = buildString {
            if (!message.isNullOrBlank()) {
                append(message)
            } else {
                append("HTTP ")
                append(status)
            }
            if (!code.isNullOrBlank()) {
                append(" (")
                append(code)
                append(")")
            }
            if (status == 404) {
                append(". Проверьте порт API 8778, а не web-интерфейс 8777.")
            }
        }
    )

    ConnectionError.NotAsledger -> ConnectionProblem(
        title = "Это не asledger",
        hint = "По адресу отвечает другой сервис. Укажите порт API 8778, а не web-порт 8777."
    )

    is ConnectionError.InvalidResponse -> ConnectionProblem(
        title = "Повреждённый ответ",
        hint = detail.ifBlank {
            "Сервер вернул данные неизвестного формата. Попробуйте ещё раз."
        }
    )

    is ConnectionError.IncompatibleFormat -> ConnectionProblem(
        title = "Версия сервера несовместима",
        hint = "Обновите asledgerd. Приложение поддерживает формат $supportedFormat, " +
            "сервер сообщил $serverFormat."
    )

    is ConnectionError.Unexpected -> ConnectionProblem(
        title = "Не удалось подключиться",
        hint = detail.ifBlank { "Неизвестная ошибка. Попробуйте ещё раз." }
    )
}

/** Сопоставляет исключение сети с понятным классом ошибки. */
fun classifyNetworkError(error: Throwable): ConnectionError = when (error) {
    is UnknownHostException -> ConnectionError.HostNotFound
    is ConnectException -> ConnectionError.ConnectionRefused
    is NoRouteToHostException -> ConnectionError.NetworkUnavailable
    is SSLException -> ConnectionError.NetworkUnavailable
    is ConnectTimeoutException -> ConnectionError.Timeout
    is HttpRequestTimeoutException -> ConnectionError.Timeout
    is java.net.SocketTimeoutException -> ConnectionError.Timeout
    is IOException -> ConnectionError.NetworkUnavailable
    else -> ConnectionError.Unexpected(
        error.message ?: error::class.simpleName ?: "неизвестная ошибка"
    )
}

/** Сопоставляет HTTP-статус и конверт ошибки. */
fun classifyHttpError(status: Int, code: String?, message: String?): ConnectionError {
    if (status == 401) {
        return ConnectionError.Unauthorized
    }
    return ConnectionError.Http(status = status, code = code, message = message)
}
