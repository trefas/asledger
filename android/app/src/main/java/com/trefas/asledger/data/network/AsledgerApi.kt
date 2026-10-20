package com.trefas.asledger.data.network

import com.trefas.asledger.data.settings.ServerConfig
import io.ktor.client.HttpClient
import io.ktor.client.call.body
import io.ktor.client.request.bearerAuth
import io.ktor.client.request.delete
import io.ktor.client.request.get
import io.ktor.client.request.post
import io.ktor.client.request.setBody
import io.ktor.client.statement.HttpResponse
import io.ktor.client.statement.bodyAsText
import io.ktor.http.ContentType
import io.ktor.http.HttpStatusCode
import io.ktor.http.contentType
import io.ktor.http.isSuccess
import kotlinx.coroutines.CancellationException
import kotlinx.serialization.json.Json

/** Результат запроса к API. */
sealed interface ApiResult<out T> {
    data class Ok<T>(val value: T) : ApiResult<T>
    data class Failure(val error: ConnectionError) : ApiResult<Nothing>
}

/** Результат привязки устройства. */
sealed interface EnrollResult {
    data class Success(
        val token: String,
        val clientId: String,
        val userId: Long,
        val userName: String
    ) : EnrollResult

    data class UserRequired(val users: List<EnrollUser>) : EnrollResult

    data class Failure(val error: ConnectionError) : EnrollResult
}

/**
 * Клиент REST API asledger.
 *
 * Все методы возвращают типизированный результат и не бросают сетевые
 * исключения наружу. Токен передаётся только в заголовке Authorization.
 */
class AsledgerApi(
    private val client: HttpClient,
    private val onUnauthorized: () -> Unit = {}
) {
    private val json = Json {
        ignoreUnknownKeys = true
        explicitNulls = false
    }

    /** Проверяет `/health`, версию и совместимость формата. */
    suspend fun health(config: ServerConfig): ApiResult<HealthInfo> = request {
        val response = client.get("${config.baseUrl}/api/v1/health")
        if (!response.status.isSuccess()) {
            return@request ApiResult.Failure(errorFrom(response))
        }
        val health = runCatching { response.body<HealthInfo>() }.getOrNull()
            ?: return@request ApiResult.Failure(ConnectionError.NotAsledger)
        if (health.status != "ok") {
            return@request ApiResult.Failure(ConnectionError.NotAsledger)
        }
        if (health.formatVersion != SUPPORTED_FORMAT_VERSION) {
            return@request ApiResult.Failure(
                ConnectionError.IncompatibleFormat(
                    serverFormat = health.formatVersion,
                    supportedFormat = SUPPORTED_FORMAT_VERSION
                )
            )
        }
        ApiResult.Ok(health)
    }

    /** Привязывает устройство; при нескольких пользователях просит выбор. */
    suspend fun enroll(
        config: ServerConfig,
        clientId: String,
        deviceName: String,
        userId: Long? = null
    ): EnrollResult = try {
        val response = client.post("${config.baseUrl}/api/v1/auth/enroll") {
            contentType(ContentType.Application.Json)
            setBody(
                EnrollRequest(
                    clientId = clientId,
                    deviceName = deviceName,
                    userId = userId?.takeIf { it > 0 }
                )
            )
        }
        when {
            response.status == HttpStatusCode.OK -> {
                val body = runCatching { response.body<EnrollResponse>() }.getOrNull()
                if (body == null || body.token.isBlank()) {
                    EnrollResult.Failure(ConnectionError.NotAsledger)
                } else {
                    EnrollResult.Success(
                        token = body.token,
                        clientId = body.clientId.ifBlank { clientId },
                        userId = body.userId,
                        userName = body.userName
                    )
                }
            }

            response.status == HttpStatusCode.Conflict -> {
                val raw = response.bodyAsText()
                val choice = runCatching { json.decodeFromString<EnrollChoiceResponse>(raw) }.getOrNull()
                if (choice?.error?.code == ERR_ENROLL_USER_REQUIRED) {
                    EnrollResult.UserRequired(choice.users)
                } else {
                    EnrollResult.Failure(errorFromText(response.status.value, raw))
                }
            }

            else -> EnrollResult.Failure(errorFrom(response))
        }
    } catch (cancelled: CancellationException) {
        throw cancelled
    } catch (error: Throwable) {
        EnrollResult.Failure(classifyNetworkError(error))
    }

    /** Читает заголовок снапшота и проверяет совместимость формата. */
    suspend fun snapshotHeader(config: ServerConfig, token: String): ApiResult<SnapshotHeader> = request {
        val response = client.get("${config.baseUrl}/api/v1/sync/snapshot") {
            bearerAuth(token)
        }
        if (response.status == HttpStatusCode.Unauthorized) {
            onUnauthorized()
            return@request ApiResult.Failure(ConnectionError.Unauthorized)
        }
        if (!response.status.isSuccess()) {
            return@request ApiResult.Failure(errorFrom(response))
        }
        val header = runCatching { response.body<SnapshotHeader>() }.getOrNull()
            ?: return@request ApiResult.Failure(ConnectionError.NotAsledger)
        if (header.formatVersion != SUPPORTED_FORMAT_VERSION) {
            return@request ApiResult.Failure(
                ConnectionError.IncompatibleFormat(
                    serverFormat = header.formatVersion,
                    supportedFormat = SUPPORTED_FORMAT_VERSION
                )
            )
        }
        ApiResult.Ok(header)
    }

    /** Отзывает текущий токен на сервере. */
    suspend fun revokeToken(config: ServerConfig, token: String): ApiResult<Unit> = request {
        val response = client.delete("${config.baseUrl}/api/v1/auth/token") {
            bearerAuth(token)
        }
        if (response.status == HttpStatusCode.Unauthorized) {
            onUnauthorized()
            return@request ApiResult.Failure(ConnectionError.Unauthorized)
        }
        if (!response.status.isSuccess()) {
            return@request ApiResult.Failure(errorFrom(response))
        }
        ApiResult.Ok(Unit)
    }

    private suspend fun errorFrom(response: HttpResponse): ConnectionError =
        errorFromText(response.status.value, runCatching { response.bodyAsText() }.getOrDefault(""))

    private fun errorFromText(status: Int, raw: String): ConnectionError {
        val envelope = runCatching { json.decodeFromString<ApiErrorEnvelope>(raw) }.getOrNull()
        return classifyHttpError(status, envelope?.error?.code, envelope?.error?.message)
    }

    private suspend fun <T> request(block: suspend () -> ApiResult<T>): ApiResult<T> = try {
        block()
    } catch (cancelled: CancellationException) {
        throw cancelled
    } catch (error: Throwable) {
        ApiResult.Failure(classifyNetworkError(error))
    }

    private companion object {
        const val SUPPORTED_FORMAT_VERSION = 1
        const val ERR_ENROLL_USER_REQUIRED = "ERR_ENROLL_USER_REQUIRED"
    }
}
