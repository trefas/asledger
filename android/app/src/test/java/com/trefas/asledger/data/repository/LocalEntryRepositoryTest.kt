package com.trefas.asledger.data.repository

import com.trefas.asledger.data.TestLibrary
import com.trefas.asledger.data.db.AsledgerDatabase
import com.trefas.asledger.data.db.OutboxStatus
import com.trefas.asledger.data.model.DraftLine
import com.trefas.asledger.data.model.EntryDraft
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.test.runTest
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config

/** Черновик и очередь: перезапуск, редактирование, удаление и commit_id. */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [34])
class LocalEntryRepositoryTest {
    private val database: AsledgerDatabase = TestLibrary.database()
    private var clock = 1_700_000_000_000L

    private fun repository() = LocalEntryRepository(database, now = { clock })

    private fun draft(description: String = "Продукты") = EntryDraft(
        opType = "трата",
        date = clock,
        description = description,
        lines = listOf(
            DraftLine(8, 25_000, "расход", "DEBIT"),
            DraftLine(1, 25_000, "источник", "CREDIT")
        )
    )

    @After
    fun tearDown() {
        database.close()
    }

    @Test
    fun `черновик и outbox создаются одной транзакцией`() = runTest {
        TestLibrary.imported(database)
        val uuid = repository().save(draft(), clientId = "client-1", actorId = 1)

        val transaction = database.transactionDao().get(uuid)
        assertEquals("Продукты", transaction?.description)
        assertNull(transaction?.serverId)
        val outbox = database.outboxDao().byTransaction(uuid)
        assertEquals(OutboxStatus.PENDING, outbox?.status)
        assertEquals("client-1:1", outbox?.commitId)
        assertEquals(2, database.transactionDao().observeDetails(uuid).first()?.lines?.size)
        assertTrue(outbox!!.opsJson.contains("\"role\":\"расход\""))
    }

    @Test
    fun `номер коммита и очередь переживают перезапуск`() = runTest {
        TestLibrary.imported(database)
        repository().save(draft(), clientId = "client-1", actorId = 1)
        clock += 1_000
        repository().save(draft("Такси"), clientId = "client-1", actorId = 1)

        val rows = database.outboxDao().all().sortedBy { it.createdAt }
        assertEquals(2, rows.size)
        assertEquals("client-1:1", rows[0].commitId)
        assertEquals("client-1:2", rows[1].commitId)
        assertEquals(2, database.outboxDao().observePendingCount().first())
    }

    @Test
    fun `редактирование отклонённого черновика выдаёт новый commit_id`() = runTest {
        TestLibrary.imported(database)
        val uuid = repository().save(draft(), clientId = "client-1", actorId = 1)
        val first = database.outboxDao().byTransaction(uuid)!!
        database.outboxDao().upsert(
            first.copy(status = OutboxStatus.REJECTED, errorCode = "ERR_MATRIX_CELL")
        )

        clock += 2_000
        repository().update(uuid, draft("Исправлено"), clientId = "client-1", actorId = 1)

        val second = database.outboxDao().byTransaction(uuid)!!
        assertNotEquals(first.commitId, second.commitId)
        assertEquals(OutboxStatus.PENDING, second.status)
        assertNull(second.errorCode)
        assertEquals("Исправлено", database.transactionDao().get(uuid)?.description)
    }

    @Test
    fun `удаление черновика убирает строки и outbox`() = runTest {
        TestLibrary.imported(database)
        val uuid = repository().save(draft(), clientId = "client-1", actorId = 1)

        repository().delete(uuid)

        assertNull(database.transactionDao().get(uuid))
        assertNull(database.outboxDao().byTransaction(uuid))
        assertEquals(8, database.transactionDao().lineCount())
    }

    @Test
    fun `синхронизированную проводку нельзя удалить или изменить`() = runTest {
        TestLibrary.imported(database)
        val uuid = repository().save(draft(), clientId = "client-1", actorId = 1)
        database.outboxDao().upsert(
            database.outboxDao().byTransaction(uuid)!!.copy(status = OutboxStatus.SYNCED)
        )

        runCatching { repository().delete(uuid) }
            .onSuccess { throw AssertionError("удаление должно быть запрещено") }
        runCatching { repository().update(uuid, draft("Новое"), "client-1", 1) }
            .onSuccess { throw AssertionError("редактирование должно быть запрещено") }
    }
}
