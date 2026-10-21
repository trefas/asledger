package com.trefas.asledger.data.sync

import com.trefas.asledger.data.TestLibrary
import com.trefas.asledger.data.db.AsledgerDatabase
import com.trefas.asledger.data.network.AsledgerApi
import com.trefas.asledger.data.network.ConnectionError
import com.trefas.asledger.data.network.createHttpClient
import com.trefas.asledger.data.security.TokenStore
import com.trefas.asledger.data.settings.ServerConfig
import com.trefas.asledger.data.settings.ServerConfigStore
import io.ktor.client.engine.mock.MockEngine
import io.ktor.client.engine.mock.respond
import io.ktor.http.HttpHeaders
import io.ktor.http.headersOf
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.test.runTest
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config

/** Загрузчик книги: успех, отсутствие токена и повреждённый ответ. */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [34])
class LibraryLoaderTest {
    private val database: AsledgerDatabase = TestLibrary.database()

    @After
    fun tearDown() {
        database.close()
    }

    @Test
    fun `успешная загрузка импортирует книгу и обновляет время`() = runTest {
        val fixture = checkNotNull(javaClass.getResource("/snapshot.json")).readText()
        val engine = MockEngine { request ->
            assertEquals("Bearer secret", request.headers[HttpHeaders.Authorization])
            respond(
                content = fixture,
                headers = headersOf(HttpHeaders.ContentType, "application/json")
            )
        }
        val configStore = FakeConfigStore()
        val progress = mutableListOf<LibraryProgress>()
        val loader = LibraryLoader(
            api = AsledgerApi(createHttpClient(engine)),
            tokenStore = FakeTokenStore("secret"),
            importer = SnapshotImporter(database, now = { 1000 }),
            configStore = configStore,
            now = { 2000 }
        )

        val result = loader.load(config, onProgress = { progress += it })

        assertTrue(result is LibraryLoadResult.Success)
        assertEquals(4, database.transactionDao().count())
        assertEquals(10, database.accountDao().count())
        assertEquals(2000, configStore.lastSync)
        assertEquals(100, progress.last().percent)
    }

    @Test
    fun `без токена книга не загружается`() = runTest {
        val loader = LibraryLoader(
            api = AsledgerApi(createHttpClient(MockEngine { error("сеть не нужна") })),
            tokenStore = FakeTokenStore(null),
            importer = SnapshotImporter(database),
            configStore = FakeConfigStore()
        )

        val result = loader.load(config)

        assertEquals(ConnectionError.Unauthorized, (result as LibraryLoadResult.Failure).error)
        assertEquals(0, database.transactionDao().count())
    }

    @Test
    fun `повреждённый снапшот не меняет базу`() = runTest {
        val engine = MockEngine {
            respond(
                content = """{"format_version":1,"transactions":"не список"}""",
                headers = headersOf(HttpHeaders.ContentType, "application/json")
            )
        }
        val loader = LibraryLoader(
            api = AsledgerApi(createHttpClient(engine)),
            tokenStore = FakeTokenStore("secret"),
            importer = SnapshotImporter(database),
            configStore = FakeConfigStore()
        )

        val result = loader.load(config)

        assertTrue((result as LibraryLoadResult.Failure).error is ConnectionError.InvalidResponse)
        assertEquals(0, database.transactionDao().count())
    }

    private val config = ServerConfig(
        host = "192.168.1.10",
        port = 8778,
        clientId = "client-1",
        userId = 1,
        userName = "Анна",
        serverVersion = "v1.0.0",
        formatVersion = 1
    )

    private class FakeTokenStore(private var token: String?) : TokenStore {
        override suspend fun save(token: String) {
            this.token = token
        }

        override suspend fun load(): String? = token

        override suspend fun clear() {
            token = null
        }
    }

    private class FakeConfigStore : ServerConfigStore {
        private val state = MutableStateFlow<ServerConfig?>(null)
        override val config: StateFlow<ServerConfig?> = state
        var lastSync: Long = 0

        override suspend fun saveServer(
            host: String,
            port: Int,
            serverVersion: String,
            formatVersion: Int
        ) = Unit

        override suspend fun saveSession(userId: Long, userName: String, lastSyncAt: Long) = Unit

        override suspend fun saveServerInfo(serverVersion: String, formatVersion: Int) = Unit

        override suspend fun saveLastSync(epochMillis: Long) {
            lastSync = epochMillis
        }

        override suspend fun setOnboarded() = Unit

        override suspend fun clearSession() = Unit

        override suspend fun clear() = Unit
    }
}
