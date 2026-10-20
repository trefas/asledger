package com.trefas.asledger.data.settings

import org.junit.Assert.assertEquals
import org.junit.Test

/** Проверки нормализации адреса сервера. */
class ServerConfigTest {

    @Test
    fun `порт по умолчанию — 8778`() {
        assertEquals(8778, ServerConfig.DEFAULT_PORT)
    }

    @Test
    fun `baseUrl содержит хост и порт`() {
        val config = ServerConfig(host = "192.168.1.10", port = 8778)
        assertEquals("http://192.168.1.10:8778", config.baseUrl)
    }

    @Test
    fun `normalizeHost убирает схему и путь`() {
        assertEquals(
            "192.168.1.10",
            ServerConfig.normalizeHost("http://192.168.1.10:8778/api/v1"),
        )
        assertEquals(
            "asledger.local",
            ServerConfig.normalizeHost(" https://asledger.local/ "),
        )
    }

    @Test
    fun `normalizeHost отделяет порт от адреса`() {
        assertEquals("192.168.1.10", ServerConfig.normalizeHost("192.168.1.10:8778"))
    }

    @Test
    fun `normalizeHost не ломает IPv6`() {
        assertEquals("::1", ServerConfig.normalizeHost("http://[::1]:8778/"))
    }
}
