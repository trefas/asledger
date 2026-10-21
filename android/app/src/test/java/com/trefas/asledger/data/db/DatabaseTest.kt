package com.trefas.asledger.data.db

import android.content.Context
import androidx.room.Room
import com.trefas.asledger.data.TestLibrary
import com.trefas.asledger.util.normalizeSearch
import java.io.File
import kotlinx.coroutines.test.runTest
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.RuntimeEnvironment
import org.robolectric.annotation.Config

/** Схема Room, уникальность UUID и повторное открытие файла. */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [34])
class DatabaseTest {
    private val context: Context = RuntimeEnvironment.getApplication()

    @After
    fun tearDown() {
        context.deleteDatabase("schema-test.db")
    }

    @Test
    fun `пустая база создаётся и открывается повторно`() = runTest {
        val first = Room.databaseBuilder(context, AsledgerDatabase::class.java, "schema-test.db")
            .allowMainThreadQueries()
            .build()
        first.openHelper.writableDatabase
        first.close()

        val second = Room.databaseBuilder(context, AsledgerDatabase::class.java, "schema-test.db")
            .allowMainThreadQueries()
            .build()
        second.openHelper.writableDatabase
        assertEquals(0, second.accountDao().count())
        second.close()
    }

    @Test
    fun `схема экспортирована в репозиторий`() {
        val relative = "schemas/com.trefas.asledger.data.db.AsledgerDatabase/1.json"
        val candidates = listOf(File(relative), File("app/$relative"))
        assertTrue(
            "нет экспортированной схемы: ${candidates.map { it.absolutePath }}",
            candidates.any { it.isFile }
        )
    }

    @Test
    fun `повторная вставка проводки по uuid обновляет строку`() = runTest {
        val database = TestLibrary.database()
        val entity = TransactionEntity(
            uuid = "uuid-1",
            serverId = 1,
            opType = "расход",
            date = 1,
            description = "Первый",
            actorId = 1,
            reversesId = null,
            source = "web",
            sourceHash = "",
            createdAt = 1,
            commitId = 1,
            searchKey = normalizeSearch("Первый")
        )
        database.transactionDao().upsertAll(listOf(entity))
        database.transactionDao().upsertAll(listOf(entity.copy(description = "Второй")))

        assertEquals(1, database.transactionDao().count())
        assertEquals("Второй", database.transactionDao().get("uuid-1")?.description)
        database.close()
    }

    @Test
    fun `head seq и chain hash хранятся в одной записи`() = runTest {
        val database = TestLibrary.database()
        database.syncMetaDao().upsert(
            SyncMetaEntity(
                headSeq = 7,
                chainHash = "hash-7",
                formatVersion = 1,
                initialLoaded = true,
                lastSyncAt = 10
            )
        )
        database.syncMetaDao().upsert(
            SyncMetaEntity(
                headSeq = 8,
                chainHash = "hash-8",
                formatVersion = 1,
                initialLoaded = true,
                lastSyncAt = 20
            )
        )

        val meta = database.syncMetaDao().get()
        assertEquals(8L, meta?.headSeq)
        assertEquals("hash-8", meta?.chainHash)
        database.close()
    }
}
