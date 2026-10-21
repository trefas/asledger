package com.trefas.asledger.data

import androidx.room.Room
import com.trefas.asledger.data.db.AsledgerDatabase
import com.trefas.asledger.data.network.SnapshotDto
import com.trefas.asledger.data.sync.SnapshotImporter
import kotlinx.serialization.json.Json
import org.robolectric.RuntimeEnvironment

/** Общие помощники тестов слоя данных. */
internal object TestLibrary {
    private val json = Json { ignoreUnknownKeys = true }

    fun snapshot(): SnapshotDto {
        val text = checkNotNull(javaClass.getResource("/snapshot.json")?.readText()) {
            "не найден snapshot.json"
        }
        return json.decodeFromString(SnapshotDto.serializer(), text)
    }

    fun database(): AsledgerDatabase =
        Room.inMemoryDatabaseBuilder(
            RuntimeEnvironment.getApplication(),
            AsledgerDatabase::class.java
        )
            .allowMainThreadQueries()
            .build()

    suspend fun imported(database: AsledgerDatabase): SnapshotDto {
        val snapshot = snapshot()
        SnapshotImporter(database, now = { 1_700_000_000_000 }).import(snapshot)
        return snapshot
    }
}
