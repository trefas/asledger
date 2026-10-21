package com.trefas.asledger.data.db

import android.content.Context
import androidx.room.Database
import androidx.room.Room
import androidx.room.RoomDatabase

/** Локальная реплика книги asledger. */
@Database(
    entities = [
        AccountEntity::class,
        TransactionEntity::class,
        LineEntity::class,
        UserEntity::class,
        SettingEntity::class,
        SyncMetaEntity::class,
        OutboxEntity::class,
        CommitEntity::class
    ],
    version = 1,
    exportSchema = true
)
abstract class AsledgerDatabase : RoomDatabase() {
    abstract fun accountDao(): AccountDao
    abstract fun transactionDao(): TransactionDao
    abstract fun userDao(): UserDao
    abstract fun settingDao(): SettingDao
    abstract fun syncMetaDao(): SyncMetaDao
    abstract fun outboxDao(): OutboxDao
    abstract fun commitDao(): CommitDao

    companion object {
        /** Имя файла базы в каталоге приложения. */
        const val NAME = "asledger.db"

        /** Создаёт базу приложения; миграции появятся вместе с версией 2. */
        fun create(context: Context): AsledgerDatabase =
            Room.databaseBuilder(context, AsledgerDatabase::class.java, NAME).build()
    }
}
