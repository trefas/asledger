package com.trefas.asledger.integration

import com.trefas.asledger.data.TestLibrary
import com.trefas.asledger.data.db.AsledgerDatabase
import com.trefas.asledger.data.db.OutboxStatus
import com.trefas.asledger.data.model.DraftLine
import com.trefas.asledger.data.model.EntryDraft
import com.trefas.asledger.data.network.ApiResult
import com.trefas.asledger.data.network.AsledgerApi
import com.trefas.asledger.data.network.EnrollResult
import com.trefas.asledger.data.network.PushOperationDto
import com.trefas.asledger.data.network.createHttpClient
import com.trefas.asledger.data.repository.LocalEntryRepository
import com.trefas.asledger.data.security.TokenStore
import com.trefas.asledger.data.settings.ServerConfig
import com.trefas.asledger.data.settings.ServerConfigStore
import com.trefas.asledger.data.sync.SnapshotImporter
import com.trefas.asledger.data.sync.SyncApplier
import com.trefas.asledger.data.sync.SyncManager
import com.trefas.asledger.data.sync.SyncOutcome
import io.ktor.client.HttpClient
import io.ktor.client.engine.okhttp.OkHttp
import java.io.File
import java.net.HttpURLConnection
import java.net.ServerSocket
import java.net.URL
import java.net.URLEncoder
import java.nio.charset.StandardCharsets
import java.nio.file.Files
import java.util.concurrent.TimeUnit
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.runBlocking
import kotlinx.serialization.encodeToString
import kotlinx.serialization.json.Json
import org.junit.After
import org.junit.AfterClass
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertTrue
import org.junit.Assume
import org.junit.Before
import org.junit.BeforeClass
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config

/**
 * JVM-интеграция Android-клиента с настоящим `asledgerd`.
 *
 * Тест запускает собранный Go-сервер на случайных портах поверх изолированной
 * книги во временном каталоге. Внешняя сеть не используется: только localhost.
 * Если бинарника нет, тест пропускается — CI собирает его перед запуском.
 */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [34])
class AndroidServerIntegrationTest {
    private lateinit var http: HttpClient
    private lateinit var api: AsledgerApi

    @Before
    fun setUp() {
        http = createHttpClient(OkHttp.create())
        api = AsledgerApi(http)
    }

    @After
    fun tearDown() {
        http.close()
    }

    @Test
    fun `два клиента видят проводки после офлайн-push и pull с rebase`() = runBlocking {
        val first = enrollClient("jvm-client-a")
        val second = enrollClient("jvm-client-b")
        try {
            val firstUuid = first.createDraft("Завтрак")
            val firstOutcome = first.syncManager.sync(first.config)
            assertTrue(firstOutcome is SyncOutcome.Success)
            assertEquals(OutboxStatus.SYNCED, first.outbox(firstUuid).status)

            // Второй клиент был офлайн и отправляет проводку со старым base_seq.
            val secondUuid = second.createDraft("Обед")
            val rebaseOutcome = second.syncManager.sync(second.config)
            assertTrue(rebaseOutcome is SyncOutcome.Success)
            val warnings = (rebaseOutcome as SyncOutcome.Success).warnings
            assertTrue("ожидалось предупреждение rebase: $warnings", warnings.isNotEmpty())
            assertEquals(OutboxStatus.SYNCED, second.outbox(secondUuid).status)

            // Первый клиент подтягивает проводку второго.
            val pulled = first.syncManager.sync(first.config)
            assertTrue(pulled is SyncOutcome.Success)
            assertNotNull(first.details(secondUuid))
            assertEquals("Обед", first.details(secondUuid)?.transaction?.description)

            val snapshot = api.snapshot(first.config, first.token)
            assertTrue(snapshot is ApiResult.Ok)
            val uuids = (snapshot as ApiResult.Ok).value.transactions.map { it.uuid }
            assertTrue(firstUuid in uuids)
            assertTrue(secondUuid in uuids)
        } finally {
            first.close()
            second.close()
        }
    }

    @Test
    fun `rejected сохраняется и переживает полный resync`() = runBlocking {
        val client = enrollClient("jvm-client-c")
        try {
            val uuid = client.createDraft("Неверный счёт")
            val outbox = client.outbox(uuid)
            val operation = Json.decodeFromString<PushOperationDto>(outbox.opsJson)
            val broken = operation.copy(
                entries = operation.entries.map { it.copy(accountId = 999_999) }
            )
            client.database.outboxDao().upsert(
                outbox.copy(opsJson = Json.encodeToString(broken))
            )

            val rejected = client.syncManager.sync(client.config)
            assertTrue("ожидался rejected: $rejected", rejected is SyncOutcome.Rejected)
            val afterReject = client.outbox(uuid)
            assertEquals(OutboxStatus.REJECTED, afterReject.status)
            assertNotNull(afterReject.errorCode)

            // Ломаем хэш цепочки: клиент обязан загрузить снапшот заново,
            // сохранив отклонённую проводку для ручного исправления.
            val meta = client.database.syncMetaDao().get()
            assertNotNull(meta)
            client.database.syncMetaDao().upsert(
                meta!!.copy(chainHash = "повреждённая-цепочка")
            )

            val resynced = client.syncManager.sync(client.config)
            assertTrue("ожидался resync: $resynced", resynced is SyncOutcome.Success)
            val afterResync = client.outbox(uuid)
            assertEquals(OutboxStatus.REJECTED, afterResync.status)
            assertNotNull(client.details(uuid))
        } finally {
            client.close()
        }
    }

    private suspend fun enrollClient(label: String): TestClient {
        val clientId = "$label-${System.nanoTime()}"
        val base = ServerConfig(
            host = "127.0.0.1",
            port = apiPort,
            clientId = clientId
        )
        val enrolled = api.enroll(base, clientId, "JVM $label")
        assertTrue("enroll: $enrolled", enrolled is EnrollResult.Success)
        val success = enrolled as EnrollResult.Success
        val config = base.copy(
            clientId = success.clientId.ifBlank { clientId },
            userId = success.userId,
            userName = success.userName,
            serverVersion = "test",
            formatVersion = SNAPSHOT_FORMAT
        )
        val darkHttp = createHttpClient(OkHttp.create())
        val darkApi = AsledgerApi(darkHttp)
        val database = TestLibrary.database()
        val snapshot = darkApi.snapshot(config, success.token)
        assertTrue("snapshot: $snapshot", snapshot is ApiResult.Ok)
        SnapshotImporter(database).import((snapshot as ApiResult.Ok).value)

        val tokens = MemoryTokenStore(success.token)
        val store = FakeConfigStore(config)
        val manager = SyncManager(
            database = database,
            api = darkApi,
            tokenStore = tokens,
            importer = SnapshotImporter(database),
            applier = SyncApplier(database),
            configStore = store,
            now = System::currentTimeMillis
        )
        return TestClient(database, config, success.token, darkApi, darkHttp, manager)
    }

    private inner class TestClient(
        val database: AsledgerDatabase,
        val config: ServerConfig,
        val token: String,
        private val clientApi: AsledgerApi,
        private val clientHttp: HttpClient,
        val syncManager: SyncManager
    ) {
        private val entries = LocalEntryRepository(database)

        suspend fun createDraft(description: String): String {
            val accounts = database.accountDao().all()
            val expense = accounts.first {
                it.isLeaf && !it.isArchived && it.type == "расход"
            }
            val asset = accounts.first {
                it.isLeaf && !it.isArchived && it.type == "актив"
            }
            return entries.save(
                draft = EntryDraft(
                    opType = "трата",
                    date = System.currentTimeMillis(),
                    description = description,
                    lines = listOf(
                        DraftLine(expense.id, 25_000, "расход", "DEBIT"),
                        DraftLine(asset.id, 25_000, "источник", "CREDIT")
                    )
                ),
                clientId = config.clientId,
                actorId = config.userId
            )
        }

        suspend fun outbox(uuid: String) =
            requireNotNull(database.outboxDao().byTransaction(uuid))

        suspend fun details(uuid: String) =
            database.transactionDao().observeDetails(uuid).first()

        /** Проверяет, что клиент действительно видит книгу через API. */
        suspend fun serverSnapshot() = clientApi.snapshot(config, token)

        fun close() {
            database.close()
            clientHttp.close()
        }
    }

    private class MemoryTokenStore(private val value: String) : TokenStore {
        override suspend fun save(token: String) = Unit
        override suspend fun load(): String = value
        override suspend fun clear() = Unit
    }

    private class FakeConfigStore(initial: ServerConfig) : ServerConfigStore {
        private val state = MutableStateFlow<ServerConfig?>(initial)
        override val config: Flow<ServerConfig?> = state

        override suspend fun saveServer(
            host: String,
            port: Int,
            serverVersion: String,
            formatVersion: Int
        ) = Unit

        override suspend fun saveSession(userId: Long, userName: String, lastSyncAt: Long) = Unit

        override suspend fun saveServerInfo(serverVersion: String, formatVersion: Int) = Unit

        override suspend fun saveLastSync(epochMillis: Long) {
            state.value = state.value?.copy(lastSyncAt = epochMillis)
        }

        override suspend fun setOnboarded() = Unit
        override suspend fun clearSession() = Unit
        override suspend fun clear() {
            state.value = null
        }
    }

    companion object {
        private const val SNAPSHOT_FORMAT = 1
        private const val CHART = """{"name":"Тест","accounts":[
  {"name":"Активы","type":"актив","children":[
    {"name":"Наличные","type":"актив"},
    {"name":"Карта","type":"актив"}]},
  {"name":"Обязательства","type":"пассив","children":[
    {"name":"Кредит","type":"пассив"}]},
  {"name":"Расходы","type":"расход","children":[
    {"name":"Продукты","type":"расход"}]},
  {"name":"Доходы","type":"доход","children":[
    {"name":"Зарплата","type":"доход"}]}
]}"""
        private const val USER_NAME = "Юзер JVM"
        private const val USER_SECRET = "секрет-jvm-теста"

        private lateinit var process: Process
        private var apiPort = 0
        private var webPort = 0

        @BeforeClass
        @JvmStatic
        fun startServer() {
            val root = findRepoRoot()
            val binary = System.getenv("ASLEDGERD_BIN")
                ?.let(::File)
                ?: File(root, "bin/asledgerd")
            Assume.assumeTrue(
                "Нет ${binary.path}; соберите make build или задайте ASLEDGERD_BIN",
                binary.canExecute()
            )
            apiPort = freePort()
            webPort = freePort()
            val temp = Files.createTempDirectory("asledger-jvm-it").toFile()
            val log = File(temp, "asledgerd.log")
            process = ProcessBuilder(
                binary.absolutePath,
                "-api-addr", "127.0.0.1:$apiPort",
                "-web-addr", "127.0.0.1:$webPort",
                "-db", File(temp, "book.db").absolutePath,
                "-assets", File(root, "assets/logo").absolutePath,
                "-charts-dir", File(root, "templates/charts").absolutePath,
                "-log-level", "error"
            )
                .directory(root)
                .redirectErrorStream(true)
                .redirectOutput(log)
                .start()
            waitForHealth(log)
            setupBook()
        }

        @AfterClass
        @JvmStatic
        fun stopServer() {
            if (::process.isInitialized) {
                process.destroy()
                if (!process.waitFor(5, TimeUnit.SECONDS)) {
                    process.destroyForcibly()
                }
            }
        }

        private fun setupBook() {
            val body = listOf(
                "user_name" to USER_NAME,
                "user_secret" to USER_SECRET,
                "content" to CHART
            ).joinToString("&") { (key, value) ->
                "${encode(key)}=${encode(value)}"
            }
            val response = postForm(
                "http://127.0.0.1:$webPort/setup/import",
                body
            )
            if (response.first == 303) return
            check(response.second.contains("Журнал") || response.second.contains("asledger")) {
                "настройка книги: ${response.first} ${response.second.take(300)}"
            }
        }

        private fun waitForHealth(log: File) {
            val deadline = System.currentTimeMillis() + 20_000
            while (System.currentTimeMillis() < deadline) {
                if (!process.isAlive) {
                    error("asledgerd завершился: ${log.readText().take(1000)}")
                }
                val ready = runCatching {
                    val connection = URL(
                        "http://127.0.0.1:$apiPort/api/v1/health"
                    ).openConnection() as HttpURLConnection
                    connection.connectTimeout = 1_000
                    connection.readTimeout = 1_000
                    connection.requestMethod = "GET"
                    val body = connection.inputStream.bufferedReader().use { it.readText() }
                    connection.disconnect()
                    body.contains("\"status\":\"ok\"")
                }.getOrDefault(false)
                if (ready) return
                Thread.sleep(100)
            }
            error("asledgerd не поднялся: ${log.readText().take(1000)}")
        }

        private fun postForm(url: String, body: String): Pair<Int, String> {
            val connection = URL(url).openConnection() as HttpURLConnection
            connection.connectTimeout = 5_000
            connection.readTimeout = 5_000
            connection.requestMethod = "POST"
            connection.doOutput = true
            connection.setRequestProperty(
                "Content-Type",
                "application/x-www-form-urlencoded; charset=UTF-8"
            )
            connection.outputStream.use { it.write(body.toByteArray(StandardCharsets.UTF_8)) }
            val code = connection.responseCode
            val stream = if (code in 200..399) connection.inputStream else connection.errorStream
            val text = stream?.bufferedReader()?.use { it.readText() }.orEmpty()
            connection.disconnect()
            return code to text
        }

        private fun findRepoRoot(): File {
            var current = File(
                requireNotNull(System.getProperty("user.dir"))
            ).absoluteFile
            repeat(6) {
                if (File(current, "go.mod").isFile &&
                    File(current, "templates/charts").isDirectory
                ) {
                    return current
                }
                current = current.parentFile ?: return@repeat
            }
            error("не найден корень репозитория от ${System.getProperty("user.dir")}")
        }

        private fun freePort(): Int = ServerSocket(0).use { it.localPort }

        private fun encode(value: String): String =
            URLEncoder.encode(value, StandardCharsets.UTF_8)
    }
}
