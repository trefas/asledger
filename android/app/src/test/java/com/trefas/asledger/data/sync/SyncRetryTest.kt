package com.trefas.asledger.data.sync

import com.trefas.asledger.data.network.ConnectionError
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

/** Политика повторов: только временные ошибки и ограниченное число попыток. */
class SyncRetryTest {
    @Test
    fun `временные сетевые ошибки повторяются`() {
        listOf(
            ConnectionError.NetworkUnavailable,
            ConnectionError.HostNotFound,
            ConnectionError.ConnectionRefused,
            ConnectionError.Timeout
        ).forEach { error ->
            assertTrue(error.toString(), SyncRetry.isTransient(error))
            assertTrue(error.toString(), SyncRetry.shouldRetry(error, attempt = 0))
        }
    }

    @Test
    fun `ошибки протокола и авторизации не повторяются`() {
        listOf(
            ConnectionError.Unauthorized,
            ConnectionError.NotAsledger,
            ConnectionError.Http(status = 500),
            ConnectionError.InvalidResponse("битый JSON"),
            ConnectionError.InvalidAddress("порт")
        ).forEach { error ->
            assertFalse(error.toString(), SyncRetry.isTransient(error))
            assertFalse(error.toString(), SyncRetry.shouldRetry(error, attempt = 0))
        }
    }

    @Test
    fun `после предельного числа попыток задача завершается`() {
        assertTrue(SyncRetry.shouldRetry(ConnectionError.Timeout, 0))
        assertTrue(
            SyncRetry.shouldRetry(
                ConnectionError.Timeout,
                SyncRetry.MAX_ATTEMPTS - 2
            )
        )
        assertFalse(
            SyncRetry.shouldRetry(
                ConnectionError.Timeout,
                SyncRetry.MAX_ATTEMPTS - 1
            )
        )
    }
}
