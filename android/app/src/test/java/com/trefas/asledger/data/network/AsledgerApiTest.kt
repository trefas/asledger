package com.trefas.asledger.data.network

import com.trefas.asledger.data.settings.ServerConfig
import io.ktor.client.engine.mock.MockEngine
import io.ktor.client.engine.mock.MockRequestHandleScope
import io.ktor.client.engine.mock.respond
import io.ktor.http.HttpHeaders
import io.ktor.http.HttpStatusCode
import io.ktor.http.content.TextContent
import io.ktor.http.headersOf
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

/** Проверки клиента API на MockEngine, без сети и сервера. */
class AsledgerApiTest {
    private val config = ServerConfig(host = "192.168.1.10", port = 8778)

    @Test
    fun `health возвращает версию и формат`() = runTest {
        val engine = MockEngine { request ->
            assertEquals("/api/v1/health", request.url.encodedPath)
            respondJson("""{"status":"ok","version":"v1.0.0","format_version":1}""")
        }

        val result = api(engine).health(config)

        val health = (result as ApiResult.Ok).value
        assertEquals("v1.0.0", health.version)
        assertEquals(1, health.formatVersion)
    }

    @Test
    fun `health блокирует несовместимый формат`() = runTest {
        val engine = MockEngine {
            respondJson("""{"status":"ok","version":"v9","format_version":2}""")
        }

        val result = api(engine).health(config)

        val error = (result as ApiResult.Failure).error
        assertTrue(error is ConnectionError.IncompatibleFormat)
        assertEquals(2, (error as ConnectionError.IncompatibleFormat).serverFormat)
    }

    @Test
    fun `health отличает чужой сервис от asledger`() = runTest {
        val engine = MockEngine {
            respond(
                content = "<html>web</html>",
                status = HttpStatusCode.OK,
                headers = headersOf(HttpHeaders.ContentType, "text/html")
            )
        }

        val result = api(engine).health(config)

        assertEquals(ConnectionError.NotAsledger, (result as ApiResult.Failure).error)
    }

    @Test
    fun `health передаёт HTTP-ошибку с кодом`() = runTest {
        val engine = MockEngine {
            respondJson(
                """{"error":{"code":"ERR_NOT_FOUND","message":"нет такой книги"}}""",
                HttpStatusCode.NotFound
            )
        }

        val result = api(engine).health(config)

        val error = (result as ApiResult.Failure).error
        assertTrue(error is ConnectionError.Http)
        error as ConnectionError.Http
        assertEquals(404, error.status)
        assertEquals("ERR_NOT_FOUND", error.code)
    }

    @Test
    fun `enroll сохраняет токен и пользователя`() = runTest {
        var requestBody = ""
        val engine = MockEngine { request ->
            requestBody = (request.body as TextContent).text
            respondJson(
                """{"token":"secret","client_id":"client-1","user_id":5,"user_name":"Анна"}"""
            )
        }

        val result = api(engine).enroll(config, "client-1", "Pixel 7")

        val success = result as EnrollResult.Success
        assertEquals("secret", success.token)
        assertEquals(5, success.userId)
        assertEquals("Анна", success.userName)
        assertTrue(requestBody.contains("\"client_id\":\"client-1\""))
    }

    @Test
    fun `enroll просит выбрать пользователя при 409`() = runTest {
        val engine = MockEngine {
            respondJson(
                """
                {
                  "error": {
                    "code": "ERR_ENROLL_USER_REQUIRED",
                    "message": "выберите пользователя"
                  },
                  "users": [
                    {"id": 1, "name": "Анна"},
                    {"id": 2, "name": "Борис"}
                  ]
                }
                """.trimIndent(),
                HttpStatusCode.Conflict
            )
        }

        val result = api(engine).enroll(config, "client-1", "Pixel 7")

        val choice = result as EnrollResult.UserRequired
        assertEquals(2, choice.users.size)
        assertEquals("Борис", choice.users[1].name)
    }

    @Test
    fun `повторный enroll передаёт выбранного пользователя`() = runTest {
        var requestBody = ""
        val engine = MockEngine { request ->
            requestBody = (request.body as TextContent).text
            respondJson(
                """{"token":"secret","client_id":"client-1","user_id":2,"user_name":"Борис"}"""
            )
        }

        val result = api(engine).enroll(config, "client-1", "Pixel 7", userId = 2)

        assertTrue(result is EnrollResult.Success)
        assertTrue(requestBody.contains("\"user_id\":2"))
    }

    @Test
    fun `снапшот с чужим форматом не принимается`() = runTest {
        val engine = MockEngine {
            respondJson("""{"format_version":2,"head_seq":1,"chain_hash":"abc"}""")
        }

        val result = api(engine).snapshotHeader(config, token = "secret")

        val error = (result as ApiResult.Failure).error
        assertTrue(error is ConnectionError.IncompatibleFormat)
    }

    @Test
    fun `401 на снапшоте очищает сессию через callback`() = runTest {
        var expired = false
        val engine = MockEngine {
            respondJson(
                """{"error":{"code":"ERR_UNAUTHORIZED","message":"токен отозван"}}""",
                HttpStatusCode.Unauthorized
            )
        }

        val result = api(engine) { expired = true }.snapshotHeader(config, token = "secret")

        assertTrue(expired)
        assertEquals(ConnectionError.Unauthorized, (result as ApiResult.Failure).error)
    }

    private fun api(
        engine: MockEngine,
        onUnauthorized: () -> Unit = {}
    ): AsledgerApi = AsledgerApi(createHttpClient(engine), onUnauthorized)
}

private fun MockRequestHandleScope.respondJson(
    content: String,
    status: HttpStatusCode = HttpStatusCode.OK
) = respond(
    content = content,
    status = status,
    headers = headersOf(HttpHeaders.ContentType, "application/json")
)
