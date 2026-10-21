package com.trefas.asledger.data.sync

import com.trefas.asledger.data.TestLibrary
import com.trefas.asledger.data.db.AsledgerDatabase
import com.trefas.asledger.data.db.LineEntity
import com.trefas.asledger.data.db.OutboxEntity
import com.trefas.asledger.data.db.TransactionEntity
import com.trefas.asledger.util.normalizeSearch
import kotlinx.coroutines.test.runTest
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config

/** Импорт снапшота: идемпотентность, откат и сохранение очереди. */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [34])
class SnapshotImporterTest {
    private val database: AsledgerDatabase = TestLibrary.database()

    @After
    fun tearDown() {
        database.close()
    }

    @Test
    fun `снапшот заполняет книгу и метаданные`() = runTest {
        TestLibrary.imported(database)

        assertEquals(10, database.accountDao().count())
        assertEquals(1, database.userDao().count())
        assertEquals(4, database.transactionDao().count())
        val meta = database.syncMetaDao().get()
        assertNotNull(meta)
        assertEquals(4L, meta?.headSeq)
        assertEquals("hash-4", meta?.chainHash)
        assertEquals(1, meta?.formatVersion)
        assertTrue(meta?.initialLoaded == true)
    }

    @Test
    fun `повторный импорт не создаёт дублей`() = runTest {
        val snapshot = TestLibrary.imported(database)
        SnapshotImporter(database, now = { 1 }).import(snapshot)

        assertEquals(10, database.accountDao().count())
        assertEquals(4, database.transactionDao().count())
        assertEquals(8, database.transactionDao().lineCount())
    }

    @Test
    fun `очередь переживает полный ресинк`() = runTest {
        TestLibrary.imported(database)
        val now = 1_700_000_000_000L
        database.transactionDao().upsertAll(
            listOf(
                TransactionEntity(
                    uuid = "local-1",
                    serverId = null,
                    opType = "расход",
                    date = now,
                    description = "Черновик",
                    actorId = 1,
                    reversesId = null,
                    source = "android",
                    sourceHash = "",
                    createdAt = now,
                    commitId = 0,
                    searchKey = normalizeSearch("Черновик")
                )
            )
        )
        database.transactionDao().upsertLines(
            listOf(
                LineEntity(
                    serverId = null,
                    transactionUuid = "local-1",
                    accountId = 8,
                    amountKop = 1000,
                    side = "DEBIT"
                ),
                LineEntity(
                    serverId = null,
                    transactionUuid = "local-1",
                    accountId = 4,
                    amountKop = 1000,
                    side = "CREDIT"
                )
            )
        )
        database.outboxDao().upsert(
            OutboxEntity(
                commitId = "client:1",
                transactionUuid = "local-1",
                clientId = "client",
                baseSeq = 4,
                opsJson = "{}",
                status = "pending",
                createdAt = now,
                updatedAt = now
            )
        )

        SnapshotImporter(database, now = { 2 }).import(TestLibrary.snapshot())

        assertEquals(5, database.transactionDao().count())
        assertNotNull(database.transactionDao().get("local-1"))
        assertEquals(1, database.outboxDao().all().size)
    }

    @Test
    fun `пустой снапшот удаляет подтверждённые, но не черновик`() = runTest {
        TestLibrary.imported(database)
        val now = 1_700_000_000_000L
        database.transactionDao().upsertAll(
            listOf(
                TransactionEntity(
                    uuid = "local-1",
                    serverId = null,
                    opType = "расход",
                    date = now,
                    description = "Черновик",
                    actorId = 1,
                    reversesId = null,
                    source = "android",
                    sourceHash = "",
                    createdAt = now,
                    commitId = 0,
                    searchKey = normalizeSearch("Черновик")
                )
            )
        )
        database.transactionDao().upsertLines(
            listOf(
                LineEntity(
                    serverId = null,
                    transactionUuid = "local-1",
                    accountId = 8,
                    amountKop = 1000,
                    side = "DEBIT"
                )
            )
        )
        database.outboxDao().upsert(
            OutboxEntity(
                commitId = "client:1",
                transactionUuid = "local-1",
                clientId = "client",
                baseSeq = 4,
                opsJson = "{}",
                status = "pending",
                createdAt = now,
                updatedAt = now
            )
        )

        SnapshotImporter(database).import(
            TestLibrary.snapshot().copy(
                accounts = emptyList(),
                users = emptyList(),
                settings = emptyMap(),
                transactions = emptyList()
            )
        )

        assertEquals(0, database.accountDao().count())
        assertEquals(1, database.transactionDao().count())
        assertNotNull(database.transactionDao().get("local-1"))
        assertNull(database.transactionDao().get("uuid-1"))
    }

    @Test
    fun `ошибка структуры не меняет книгу`() = runTest {
        TestLibrary.imported(database)
        val broken = TestLibrary.snapshot().copy(formatVersion = 2)

        val failure = runCatching { SnapshotImporter(database).import(broken) }.exceptionOrNull()

        assertTrue(failure is SnapshotImportException)
        assertEquals(4, database.transactionDao().count())
        assertEquals(10, database.accountDao().count())
    }

    @Test
    fun `повтор uuid в снапшоте отклоняется до записи`() = runTest {
        val snapshot = TestLibrary.snapshot()
        val duplicated = snapshot.copy(
            transactions = snapshot.transactions + snapshot.transactions.first().copy(id = 999)
        )

        val failure = runCatching { SnapshotImporter(database).import(duplicated) }.exceptionOrNull()

        assertTrue(failure is SnapshotImportException)
        assertEquals(0, database.transactionDao().count())
    }
}
