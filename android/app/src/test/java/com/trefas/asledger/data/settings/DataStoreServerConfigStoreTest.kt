package com.trefas.asledger.data.settings

import kotlinx.coroutines.flow.first
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.RuntimeEnvironment
import org.robolectric.annotation.Config

/** Проверки сохранения адреса и сессии в DataStore. */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [34])
class DataStoreServerConfigStoreTest {
    @Test
    fun `адрес и сессия переживают перечитывание`() = runTest {
        val store = DataStoreServerConfigStore(RuntimeEnvironment.getApplication())
        store.clear()
        assertNull(store.config.first())

        store.saveServer(
            host = "192.168.1.10",
            port = 8778,
            serverVersion = "v1.0.0",
            formatVersion = 1
        )
        val saved = store.config.first()
        requireNotNull(saved)
        assertEquals("192.168.1.10", saved.host)
        assertEquals(8778, saved.port)
        assertTrue(saved.clientId.isNotBlank())
        assertFalse(saved.hasSession)

        store.saveSession(userId = 5, userName = "Анна", lastSyncAt = 123)
        val enrolled = store.config.first()
        requireNotNull(enrolled)
        assertTrue(enrolled.hasSession)
        assertEquals("Анна", enrolled.userName)
        assertEquals(enrolled.clientId, saved.clientId)

        store.clearSession()
        val forgottenUser = store.config.first()
        requireNotNull(forgottenUser)
        assertFalse(forgottenUser.hasSession)
        assertEquals("192.168.1.10", forgottenUser.host)

        store.clear()
        assertNull(store.config.first())
    }
}
