package com.trefas.asledger.data

import android.content.Context
import com.trefas.asledger.BuildConfig
import com.trefas.asledger.data.settings.DataStoreServerConfigStore
import com.trefas.asledger.data.settings.ServerConfigStore
import io.ktor.client.HttpClient
import io.ktor.client.engine.okhttp.OkHttp
import io.ktor.client.plugins.HttpRequestRetry
import io.ktor.client.plugins.HttpTimeout
import io.ktor.client.plugins.contentnegotiation.ContentNegotiation
import io.ktor.client.plugins.defaultRequest
import io.ktor.client.request.accept
import io.ktor.http.ContentType
import io.ktor.serialization.kotlinx.json.json
import java.io.IOException
import kotlinx.serialization.json.Json

// Повторы только для сетевых ошибок и 5xx: записывающие операции идемпотентны
// по uuid проводки или client_id устройства, поэтому повтор не создаёт дубль.
private const val HTTP_MAX_RETRIES = 2

/**
 * Ручной контейнер зависимостей.
 *
 * Пока в нём настройки сервера и HTTP-клиент; по мере реализации вех S11–S12
 * сюда добавятся база Room и очередь синхронизации.
 */
class AppContainer(context: Context) {
    /** Контекст приложения: не держим Activity. */
    val appContext: Context = context.applicationContext

    /** Адрес и порт сервера книги. */
    val serverConfig: ServerConfigStore = DataStoreServerConfigStore(appContext)

    /** HTTP-клиент для REST API asledger. */
    val httpClient: HttpClient by lazy { createHttpClient() }

    private fun createHttpClient(): HttpClient = HttpClient(OkHttp) {
        expectSuccess = false
        install(ContentNegotiation) {
            json(
                Json {
                    ignoreUnknownKeys = true
                    explicitNulls = false
                    encodeDefaults = true
                }
            )
        }
        install(HttpTimeout) {
            requestTimeoutMillis = BuildConfig.API_TIMEOUT_SECONDS * 1000
            connectTimeoutMillis = BuildConfig.API_CONNECT_TIMEOUT_SECONDS * 1000
            socketTimeoutMillis = BuildConfig.API_TIMEOUT_SECONDS * 1000
        }
        install(HttpRequestRetry) {
            retryOnServerErrors(maxRetries = HTTP_MAX_RETRIES)
            retryOnExceptionIf { _, cause -> cause is IOException }
            exponentialDelay()
        }
        defaultRequest {
            accept(ContentType.Application.Json)
        }
    }
}
