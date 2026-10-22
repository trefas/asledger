package com.trefas.asledger.data.network

import androidx.annotation.StringRes
import com.trefas.asledger.R
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
    data class InvalidAddress(@StringRes val detailRes: Int) : ConnectionError

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
    data class InvalidResponse(@StringRes val detailRes: Int = 0) : ConnectionError

    /** Формат снапшота несовместим с приложением. */
    data class IncompatibleFormat(
        val serverFormat: Int,
        val supportedFormat: Int
    ) : ConnectionError

    /** Непредвиденная ошибка; detail не содержит секретов. */
    data class Unexpected(
        val detail: String? = null,
        @StringRes val detailRes: Int = 0
    ) : ConnectionError
}

/** Готовая к показу часть текста: ресурс с аргументами или динамическая строка. */
sealed interface ConnectionDetail {
    data class Resource(
        @StringRes val id: Int,
        val args: List<Any> = emptyList()
    ) : ConnectionDetail

    data class Text(val value: String) : ConnectionDetail
}

/** Понятное пользователю объяснение ошибки и подсказка. */
data class ConnectionProblem(
    @StringRes val titleRes: Int,
    val hint: ConnectionDetail
)

/** Преобразует ошибку в текст для экрана. */
fun ConnectionError.toProblem(): ConnectionProblem = when (this) {
    is ConnectionError.InvalidAddress -> ConnectionProblem(
        titleRes = R.string.connection_invalid_address_title,
        hint = ConnectionDetail.Resource(detailRes)
    )

    ConnectionError.NetworkUnavailable -> ConnectionProblem(
        titleRes = R.string.connection_network_unavailable_title,
        hint = ConnectionDetail.Resource(R.string.connection_network_unavailable_hint)
    )

    ConnectionError.HostNotFound -> ConnectionProblem(
        titleRes = R.string.connection_host_not_found_title,
        hint = ConnectionDetail.Resource(R.string.connection_host_not_found_hint)
    )

    ConnectionError.ConnectionRefused -> ConnectionProblem(
        titleRes = R.string.connection_refused_title,
        hint = ConnectionDetail.Resource(R.string.connection_refused_hint)
    )

    ConnectionError.Timeout -> ConnectionProblem(
        titleRes = R.string.connection_timeout_title,
        hint = ConnectionDetail.Resource(R.string.connection_timeout_hint)
    )

    ConnectionError.Unauthorized -> ConnectionProblem(
        titleRes = R.string.connection_unauthorized_title,
        hint = ConnectionDetail.Resource(R.string.connection_unauthorized_hint)
    )

    is ConnectionError.Http -> {
        val codeSuffix = code?.takeIf { it.isNotBlank() }?.let { " ($it)" }.orEmpty()
        val messageSuffix = message?.takeIf { it.isNotBlank() }?.let { ": $it" }.orEmpty()
        ConnectionProblem(
            titleRes = R.string.connection_http_title,
            hint = ConnectionDetail.Resource(
                id = if (status == 404) {
                    R.string.connection_http_404_hint
                } else {
                    R.string.connection_http_hint
                },
                args = listOf(status, codeSuffix, messageSuffix)
            )
        )
    }

    ConnectionError.NotAsledger -> ConnectionProblem(
        titleRes = R.string.connection_not_asledger_title,
        hint = ConnectionDetail.Resource(R.string.connection_not_asledger_hint)
    )

    is ConnectionError.InvalidResponse -> ConnectionProblem(
        titleRes = R.string.connection_invalid_response_title,
        hint = if (detailRes != 0) {
            ConnectionDetail.Resource(detailRes)
        } else {
            ConnectionDetail.Resource(R.string.connection_invalid_response_hint)
        }
    )

    is ConnectionError.IncompatibleFormat -> ConnectionProblem(
        titleRes = R.string.connection_incompatible_title,
        hint = ConnectionDetail.Resource(
            id = R.string.connection_incompatible_hint,
            args = listOf(supportedFormat, serverFormat)
        )
    )

    is ConnectionError.Unexpected -> ConnectionProblem(
        titleRes = R.string.connection_unexpected_title,
        hint = when {
            detailRes != 0 -> ConnectionDetail.Resource(detailRes)
            !detail.isNullOrBlank() -> ConnectionDetail.Text(detail)
            else -> ConnectionDetail.Resource(R.string.connection_unexpected_hint)
        }
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
    else -> ConnectionError.Unexpected(detail = error.message ?: error::class.simpleName)
}

/** Сопоставляет HTTP-статус и конверт ошибки. */
fun classifyHttpError(status: Int, code: String?, message: String?): ConnectionError {
    if (status == 401) {
        return ConnectionError.Unauthorized
    }
    return ConnectionError.Http(status = status, code = code, message = message)
}
