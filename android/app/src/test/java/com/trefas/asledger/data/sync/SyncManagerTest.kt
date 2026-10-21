package com.trefas.asledger.data.sync

import com.trefas.asledger.data.TestLibrary
import com.trefas.asledger.data.db.AsledgerDatabase
import com.trefas.asledger.data.db.OutboxStatus
import com.trefas.asledger.data.model.DraftLine
import com.trefas.asledger.data.model.EntryDraft
import com.trefas.asledger.data.network.AsledgerApi
import com.trefas.asledger.data.network.createHttpClient
import com.trefas.asledger.data.repository.LocalEntryRepository
import com.trefas.asledger.data.security.TokenStore
import com.trefas.asledger.data.settings.ServerConfig
import com.trefas.asledger.data.settings.ServerConfigStore
import io.ktor.client.engine.mock.MockEngine
import io.ktor.client.engine.mock.respond
import io.ktor.http.HttpHeaders
import io.ktor.http.HttpStatusCode
import io.ktor.http.content.TextContent
import io.ktor.http.headersOf
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.flowOf
import kotlinx.coroutines.test.runTest
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config

/** Push: accepted, rejected и повтор с тем же commit_id. */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [34])
class SyncManagerTest {
    private val database: AsledgerDatabase = TestLibrary.database()
    private val config = ServerConfig(
        host = "192.168.1.10",
        clientId = "client-1",
        userId = 1,
        userName = "Анна"
    )
    private val requests = mutableListOf<String>()

    @After
    fun tearDown() {
        database.close()
    }

    private suspend fun manager(
        engine: MockEngine,
        token: TokenStore = FakeTokenStore("secret")
    ): SyncManager {
        TestLibrary.imported(database)
        val api = AsledgerApi(createHttpClient(engine))
        return SyncManager(
            database = database,
            api = api,
            tokenStore = token,
            importer = SnapshotImporter(database),
            applier = SyncApplier(database),
            configStore = FakeConfigStore,
            now = { 100 }
        )
    }

    private suspend fun draft() {
        LocalEntryRepository(database, now = { 1 }).save(
            EntryDraft(
                opType = "трата",
                date = 1,
                description = "Продукты",
                lines = listOf(
                    DraftLine(8, 25_000, "расход", "DEBIT"),
                    DraftLine(4, 25_000, "источник", "CREDIT")
                )
            ),
            clientId = config.clientId,
            actorId = 1
        )
    }

    private fun engine(response: String, status: HttpStatusCode = HttpStatusCode.OK) =
        MockEngine { request ->
            requests += (request.body as TextContent).text
            respond(
                content = response,
                status = status,
                headers = headersOf(HttpHeaders.ContentType, "application/json")
            )
        }

    @Test
    fun `успешный push переводит очередь в synced`() = runTest {
        val manager = manager(
            engine(
                """{"status":"accepted","commit_id":"client-1:1","result_seq":5,"applied":["x"],"warnings":[]}"""
            )
        )
        draft()

        val result = manager.pushOnly(config)

        assertTrue(result is SyncOutcome.Success)
        val outbox = database.outboxDao().all().single()
        assertEquals(OutboxStatus.SYNCED, outbox.status)
        assertTrue(requests.single().contains("\"commit_id\":\"client-1:1\""))
    }

    @Test
    fun `rejected сохраняет код и сообщение, не теряя черновик`() = runTest {
        val manager = manager(
            engine(
                """{"status":"rejected","commit_id":"client-1:1","result_seq":4,
                   "warnings":[],"error":{"code":"ERR_MATRIX_CELL","message":"счёт не подходит"}}""",
                HttpStatusCode.UnprocessableEntity
            )
        )
        draft()

        val result = manager.pushOnly(config)

        val rejected = result as SyncOutcome.Rejected
        assertEquals("ERR_MATRIX_CELL", rejected.code)
        val outbox = database.outboxDao().all().single()
        assertEquals(OutboxStatus.REJECTED, outbox.status)
        assertEquals("ERR_MATRIX_CELL", outbox.errorCode)
        assertTrue(database.transactionDao().all().any { it.source == "android" })
    }

    @Test
    fun `повтор после ошибки использует тот же commit_id`() = runTest {
        val manager = manager(
            engine(
                """{"status":"accepted","commit_id":"client-1:1","result_seq":5,"applied":[],"warnings":[]}"""
            )
        )
        draft()
        manager.pushOnly(config)
        val firstBody = requests.single()

        database.outboxDao().upsert(
            database.outboxDao().all().single().copy(status = OutboxStatus.PENDING)
        )
        manager.pushOnly(config)

        assertEquals(2, requests.size)
        assertEquals(firstBody, requests[1])
    }

    @Test
    fun `без токена ничего не отправляется`() = runTest {
        val manager = manager(
            engine("""{"status":"accepted"}"""),
            token = FakeTokenStore(null)
        )
        draft()

        val result = manager.pushOnly(config)

        assertTrue(result is SyncOutcome.Failure)
        assertEquals(OutboxStatus.PENDING, database.outboxDao().all().single().status)
    }
}

private class FakeTokenStore(private var token: String?) : TokenStore {
    override suspend fun save(token: String) {
        this.token = token
    }

    override suspend fun load(): String? = token

    override suspend fun clear() {
        token = null
    }
}

private object FakeConfigStore : ServerConfigStore {
    override val config: Flow<ServerConfig?> = flowOf(null)
    override suspend fun saveServer(
        host: String,
        port: Int,
        serverVersion: String,
        formatVersion: Int
    ) = Unit

    override suspend fun saveSession(userId: Long, userName: String, lastSyncAt: Long) = Unit
    override suspend fun saveServerInfo(serverVersion: String, formatVersion: Int) = Unit
    override suspend fun saveLastSync(epochMillis: Long) = Unit
    override suspend fun setOnboarded() = Unit
    override suspend fun clearSession() = Unit
    override suspend fun clear() = Unit
}
