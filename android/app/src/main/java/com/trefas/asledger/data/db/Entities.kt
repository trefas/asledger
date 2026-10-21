package com.trefas.asledger.data.db

import androidx.room.Entity
import androidx.room.ForeignKey
import androidx.room.Index
import androidx.room.PrimaryKey

/**
 * Счёт локальной реплики.
 *
 * `id` — идентификатор сервера; он же используется в строках проводок.
 * `searchKey` хранит имя и путь в нижнем регистре, чтобы поиск по кириллице не
 * зависел от регистра на любой версии SQLite.
 */
@Entity(
    tableName = "accounts",
    indices = [
        Index(value = ["fullPath"], unique = true),
        Index(value = ["parentId"]),
        Index(value = ["searchKey"])
    ]
)
data class AccountEntity(
    @PrimaryKey val id: Long,
    val name: String,
    val fullPath: String,
    val type: String,
    val parentId: Long?,
    val isLeaf: Boolean,
    val minBalanceKop: Long,
    val isArchived: Boolean,
    val isSystem: Boolean,
    val createdAt: Long,
    val searchKey: String
)

/** Заголовок проводки. Ключ — UUID, потому что офлайн-проводка ещё не имеет id сервера. */
@Entity(
    tableName = "transactions",
    indices = [
        Index(value = ["uuid"], unique = true),
        Index(value = ["serverId"], unique = true),
        Index(value = ["date"]),
        Index(value = ["opType"]),
        Index(value = ["searchKey"])
    ]
)
data class TransactionEntity(
    @PrimaryKey val uuid: String,
    val serverId: Long?,
    val opType: String,
    /** Дата проводки в миллисекундах Unix. */
    val date: Long,
    val description: String,
    val actorId: Long,
    val reversesId: Long?,
    val source: String,
    val sourceHash: String,
    val createdAt: Long,
    val commitId: Long,
    val searchKey: String
)

/** Строка проводки: счёт, сумма в копейках и сторона, назначенная движком. */
@Entity(
    tableName = "lines",
    foreignKeys = [
        ForeignKey(
            entity = TransactionEntity::class,
            parentColumns = ["uuid"],
            childColumns = ["transactionUuid"],
            onDelete = ForeignKey.CASCADE
        )
    ],
    indices = [
        Index(value = ["transactionUuid"]),
        Index(value = ["accountId"]),
        Index(value = ["serverId"], unique = true)
    ]
)
data class LineEntity(
    @PrimaryKey(autoGenerate = true) val id: Long = 0,
    /** id строки на сервере; у локального черновика null. */
    val serverId: Long?,
    val transactionUuid: String,
    val accountId: Long,
    val amountKop: Long,
    /** DEBIT или CREDIT; для локальной строки выводится из роли. */
    val side: String,
    /** Роль строки из матрицы сервера; у импортированных строк пустая. */
    val role: String = ""
)

/** Пользователь книги без хеша токена. */
@Entity(tableName = "users", indices = [Index(value = ["searchKey"])])
data class UserEntity(
    @PrimaryKey val id: Long,
    val name: String,
    val searchKey: String
)

/** Настройка книги. */
@Entity(tableName = "settings")
data class SettingEntity(
    @PrimaryKey val key: String,
    val value: String
)

/**
 * Метаданные синхронизации в одной записи (id = 1).
 *
 * `initialLoaded` отделяет первый импорт от последующих дельт: до него журнал
 * не показывается, чтобы пользователь не видел пустую книгу.
 */
@Entity(tableName = "sync_meta")
data class SyncMetaEntity(
    @PrimaryKey val id: Int = SINGLETON_ID,
    val headSeq: Long,
    val chainHash: String,
    val formatVersion: Int,
    val initialLoaded: Boolean,
    val lastSyncAt: Long,
    /** Последний выданный локальный номер коммита. */
    val localCounter: Long = 0
) {
    companion object {
        const val SINGLETON_ID = 1
    }
}

/** Статусы очереди отправки. */
object OutboxStatus {
    const val DRAFT = "draft"
    const val PENDING = "pending"
    const val SENDING = "sending"
    const val SYNCED = "synced"
    const val REJECTED = "rejected"

    /** Статусы, которые ещё нужно отправить. */
    val ACTIVE = listOf(PENDING, SENDING)
}

/**
 * Очередь отправки локальных проводок.
 *
 * `commitId` одинаков у всех операций одного пакета: это делает повторную
 * отправку идемпотентной. Индекс неуникальный, потому что пакет объединяет
 * несколько операций под одним коммитом.
 */
@Entity(
    tableName = "outbox",
    indices = [
        Index(value = ["status"]),
        Index(value = ["transactionUuid"], unique = true),
        Index(value = ["commitId"])
    ]
)
data class OutboxEntity(
    @PrimaryKey(autoGenerate = true) val id: Long = 0,
    val commitId: String,
    val transactionUuid: String,
    val clientId: String,
    val baseSeq: Long,
    /** JSON одной операции без обёртки пакета. */
    val opsJson: String,
    /** draft, pending, sending, synced или rejected. */
    val status: String,
    val attempts: Int = 0,
    val lastError: String? = null,
    /** Код отклонения сервера, например ERR_MATRIX_CELL. */
    val errorCode: String? = null,
    /** Предупреждения последней отправки в JSON-массиве строк. */
    val warningsJson: String = "[]",
    /** UUID возможных дублей в JSON-массиве строк. */
    val duplicateUuidsJson: String = "[]",
    val createdAt: Long,
    val updatedAt: Long
)

/** Коммит синхронизации, полученный с сервера (S12). */
@Entity(
    tableName = "commits",
    indices = [
        Index(value = ["hash"]),
        Index(value = ["clientId", "resultSeq"])
    ]
)
data class CommitEntity(
    @PrimaryKey val id: Long,
    val clientId: String,
    val actorId: Long,
    val baseSeq: Long,
    val opsJson: String,
    val resultSeq: Long,
    val hash: String,
    val warningsJson: String,
    val createdAt: Long
)
