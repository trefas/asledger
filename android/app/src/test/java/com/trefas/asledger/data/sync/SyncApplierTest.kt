package com.trefas.asledger.data.sync

import com.trefas.asledger.data.TestLibrary
import com.trefas.asledger.data.db.AsledgerDatabase
import com.trefas.asledger.data.db.SyncMetaEntity
import com.trefas.asledger.data.network.PullCommitDto
import com.trefas.asledger.data.network.PullDto
import com.trefas.asledger.data.network.SnapshotAccountDto
import com.trefas.asledger.data.network.SnapshotLineDto
import com.trefas.asledger.data.network.SnapshotTransactionDto
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.test.runTest
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertThrows
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config

/** Дельта pull: хэш-цепочка, транзакционность и сохранение счётчика. */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [34])
class SyncApplierTest {
    private val database: AsledgerDatabase = TestLibrary.database()
    private val ops = """{"commit_id":"dev:1","operations":[]}"""
    private val commit = PullCommitDto(
        id = 9,
        clientId = "dev",
        actorId = 1,
        baseSeq = 0,
        opsJson = ops,
        resultSeq = 1,
        hash = chainHash("", ops),
        warnings = emptyList(),
        createdAt = "2026-01-01T00:00:00Z"
    )

    @After
    fun tearDown() {
        database.close()
    }

    private fun pull(commit: PullCommitDto = this.commit) = PullDto(
        formatVersion = 1,
        since = 0,
        headSeq = commit.resultSeq,
        chainHash = commit.hash,
        changes = emptyList(),
        transactions = listOf(
            SnapshotTransactionDto(
                id = 41,
                uuid = "remote-1",
                opType = "трата",
                date = "2026-01-02T12:00:00Z",
                description = "Сервер",
                actorId = 1,
                lines = listOf(
                    SnapshotLineDto(101, 41, 8, 10_000, "DEBIT"),
                    SnapshotLineDto(102, 41, 4, 10_000, "CREDIT")
                )
            )
        ),
        accounts = listOf(
            SnapshotAccountDto(
                id = 8,
                name = "Продукты",
                fullPath = "Расходы:Продукты",
                type = "расход",
                isLeaf = true
            )
        ),
        commits = listOf(commit)
    )

    @Test
    fun `дельта применяется вместе с meta и коммитом`() = runTest {
        database.syncMetaDao().upsert(
            SyncMetaEntity(
                headSeq = 0,
                chainHash = "",
                formatVersion = 1,
                initialLoaded = true,
                lastSyncAt = 0,
                localCounter = 7
            )
        )

        SyncApplier(database, now = { 123 }).apply(pull(), knownHash = "")

        val meta = database.syncMetaDao().get()!!
        assertEquals(1, meta.headSeq)
        assertEquals(commit.hash, meta.chainHash)
        assertEquals(7, meta.localCounter)
        assertEquals(1, database.accountDao().count())
        assertEquals("Сервер", database.transactionDao().get("remote-1")?.description)
        assertEquals(2, database.transactionDao().observeDetails("remote-1").first()?.lines?.size)
        assertEquals(1, database.commitDao().count())
    }

    @Test
    fun `подмена ops_json не применяет дельту`() = runTest {
        database.syncMetaDao().upsert(
            SyncMetaEntity(
                headSeq = 0,
                chainHash = "",
                formatVersion = 1,
                initialLoaded = true,
                lastSyncAt = 0
            )
        )
        val forged = commit.copy(hash = "чужой-хеш")

        assertThrows(ChainMismatchException::class.java) {
            kotlinx.coroutines.runBlocking {
                SyncApplier(database).apply(pull(forged), knownHash = "")
            }
        }
        assertEquals(0, database.accountDao().count())
        assertEquals(0, database.transactionDao().count())
    }
}
