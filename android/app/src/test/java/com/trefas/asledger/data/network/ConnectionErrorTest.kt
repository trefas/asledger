package com.trefas.asledger.data.network

import java.io.IOException
import java.net.ConnectException
import java.net.SocketTimeoutException
import java.net.UnknownHostException
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

/** Проверки разбора сетевых ошибок и подсказок пользователю. */
class ConnectionErrorTest {
    @Test
    fun `неизвестный хост распознаётся отдельно`() {
        assertEquals(
            ConnectionError.HostNotFound,
            classifyNetworkError(UnknownHostException("asledger.local"))
        )
    }

    @Test
    fun `закрытый порт распознаётся как отказ соединения`() {
        assertEquals(
            ConnectionError.ConnectionRefused,
            classifyNetworkError(ConnectException("connection refused"))
        )
    }

    @Test
    fun `таймаут сокета распознаётся`() {
        assertEquals(
            ConnectionError.Timeout,
            classifyNetworkError(SocketTimeoutException("timeout"))
        )
    }

    @Test
    fun `общий сбой ввода-вывода считается потерей сети`() {
        assertEquals(
            ConnectionError.NetworkUnavailable,
            classifyNetworkError(IOException("network is unreachable"))
        )
    }

    @Test
    fun `неизвестная ошибка не раскрывает секретов`() {
        val error = classifyNetworkError(IllegalStateException("boom"))
        assertTrue(error is ConnectionError.Unexpected)
    }

    @Test
    fun `401 становится требованием повторной привязки`() {
        assertEquals(
            ConnectionError.Unauthorized,
            classifyHttpError(401, "ERR_UNAUTHORIZED", "токен отозван")
        )
    }

    @Test
    fun `404 подсказывает проверить порт API`() {
        val problem = ConnectionError.Http(
            status = 404,
            code = "ERR_NOT_FOUND",
            message = "не найдено"
        ).toProblem()

        assertTrue(problem.hint.contains("8778"))
    }

    @Test
    fun `несовместимый формат объясняет версии`() {
        val problem = ConnectionError.IncompatibleFormat(
            serverFormat = 2,
            supportedFormat = 1
        ).toProblem()

        assertEquals("Версия сервера несовместима", problem.title)
        assertTrue(problem.hint.contains("2"))
        assertTrue(problem.hint.contains("1"))
    }
}
