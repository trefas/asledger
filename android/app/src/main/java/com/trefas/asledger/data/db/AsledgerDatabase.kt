package com.trefas.asledger.data.db

import android.content.Context
import androidx.room.Database
import androidx.room.Room
import androidx.room.RoomDatabase
import androidx.room.migration.Migration
import androidx.sqlite.db.SupportSQLiteDatabase

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
    version = 2,
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

        /**
         * Версия 2 добавила роль строки, счётчик локальных коммитов и
         * диагностику отправки. Данные книги при обновлении не теряются.
         */
        val MIGRATION_1_2 = object : Migration(1, 2) {
            override fun migrate(db: SupportSQLiteDatabase) {
                db.execSQL("ALTER TABLE lines ADD COLUMN role TEXT NOT NULL DEFAULT ''")
                db.execSQL(
                    "ALTER TABLE sync_meta ADD COLUMN localCounter INTEGER NOT NULL DEFAULT 0"
                )
                db.execSQL("ALTER TABLE outbox ADD COLUMN errorCode TEXT")
                db.execSQL(
                    "ALTER TABLE outbox ADD COLUMN warningsJson TEXT NOT NULL DEFAULT '[]'"
                )
                db.execSQL(
                    "ALTER TABLE outbox ADD COLUMN duplicateUuidsJson TEXT NOT NULL DEFAULT '[]'"
                )
                db.execSQL("DROP INDEX IF EXISTS index_outbox_commitId")
                db.execSQL("CREATE INDEX index_outbox_commitId ON outbox(commitId)")
            }
        }

        /** Создаёт базу приложения. */
        fun create(context: Context): AsledgerDatabase =
            Room.databaseBuilder(context, AsledgerDatabase::class.java, NAME)
                .addMigrations(MIGRATION_1_2)
                .build()
    }
}
