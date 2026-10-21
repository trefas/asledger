package com.trefas.asledger.data.repository

import com.trefas.asledger.data.TestLibrary
import com.trefas.asledger.data.db.AsledgerDatabase
import com.trefas.asledger.data.db.LineEntity
import com.trefas.asledger.data.db.OutboxEntity
import com.trefas.asledger.data.db.TransactionEntity
import com.trefas.asledger.data.model.JournalFilter
import com.trefas.asledger.util.normalizeSearch
import com.trefas.asledger.util.parseServerTime
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.test.runTest
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config

/** Репозитории читают импортированную книгу и реагируют на изменения. */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [34])
class RepositoryTest {
    private val database: AsledgerDatabase = TestLibrary.database()

    @After
    fun tearDown() {
        database.close()
    }

    @Test
    fun `дерево счетов и поиск по кириллице`() = runTest {
        TestLibrary.imported(database)
        val repository = AccountRepository(database.accountDao())

        val tree = repository.observeTree().first()
        assertEquals(5, tree.size)
        val assets = tree.first { it.account.name == "Активы" }
        assertEquals(listOf("Наличные"), assets.children.map { it.account.name })

        val found = repository.search("пРОДУКТ").first()
        assertEquals(listOf("Продукты"), found.map { it.name })

        val nested = repository.search("АКТИВЫ:НАЛ").first()
        assertEquals(listOf("Наличные"), nested.map { it.name })
        assertEquals(5, repository.observeLeaves().first().size)
    }

    @Test
    fun `журнал сортирует, фильтрует и листает`() = runTest {
        TestLibrary.imported(database)
        val repository = TransactionRepository(database.transactionDao())

        val firstPage = repository.observePage(limit = 2, offset = 0).first()
        assertEquals(listOf("uuid-4", "uuid-3"), firstPage.map { it.uuid })
        assertEquals(4, repository.observeCount().first())

        val secondPage = repository.observePage(limit = 2, offset = 2).first()
        assertEquals(listOf("uuid-2", "uuid-1"), secondPage.map { it.uuid })

        val byText = repository.observePage(
            filter = JournalFilter(query = "ПРОДУКТ")
        ).first()
        assertEquals(listOf("uuid-2"), byText.map { it.uuid })
        assertEquals("Анна", byText.single().actorName)
        assertEquals(20000, byText.single().amountKop)

        val byDate = repository.observePage(
            filter = JournalFilter(
                fromDate = parseServerTime("2026-01-10T00:00:00Z"),
                toDate = parseServerTime("2026-01-15T23:59:59Z")
            )
        ).first()
        assertEquals(listOf("uuid-3", "uuid-2"), byDate.map { it.uuid })

        val byAccount = repository.observePage(
            filter = JournalFilter(accountId = 4)
        ).first()
        assertEquals(4, byAccount.size)

        val byType = repository.observePage(
            filter = JournalFilter(opType = "расход")
        ).first()
        assertEquals(listOf("uuid-2"), byType.map { it.uuid })

        val details = repository.observeDetails("uuid-3").first()
        assertEquals(2, details?.lines?.size)
        assertFalse(details?.pending == true)
    }

    @Test
    fun `локальный черновик виден фильтру очереди`() = runTest {
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

        val repository = TransactionRepository(database.transactionDao())
        val pending = repository.observePage(
            filter = JournalFilter(pendingOnly = true)
        ).first()

        assertEquals(1, pending.size)
        assertTrue(pending.single().pending)
        assertTrue(repository.observeDetails("local-1").first()?.pending == true)
        assertEquals(1, database.outboxDao().observePendingCount().first())
    }
}
