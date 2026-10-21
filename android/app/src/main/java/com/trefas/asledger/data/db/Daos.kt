package com.trefas.asledger.data.db

import androidx.room.Dao
import androidx.room.Embedded
import androidx.room.Query
import androidx.room.Relation
import androidx.room.Transaction
import androidx.room.Upsert
import kotlinx.coroutines.flow.Flow

/** Запросы к плану счетов. */
@Dao
interface AccountDao {
    @Query("SELECT * FROM accounts ORDER BY fullPath")
    fun observeAll(): Flow<List<AccountEntity>>

    @Query("SELECT * FROM accounts WHERE isArchived = 0 ORDER BY fullPath")
    fun observeActive(): Flow<List<AccountEntity>>

    @Query("SELECT * FROM accounts WHERE isLeaf = 1 AND isArchived = 0 ORDER BY fullPath")
    fun observeLeaves(): Flow<List<AccountEntity>>

    @Query("SELECT * FROM accounts WHERE searchKey LIKE '%' || :query || '%' ORDER BY fullPath")
    fun search(query: String): Flow<List<AccountEntity>>

    @Query("SELECT * FROM accounts")
    suspend fun all(): List<AccountEntity>

    @Upsert
    suspend fun upsertAll(accounts: List<AccountEntity>)

    @Query("DELETE FROM accounts")
    suspend fun clear()

    @Query("SELECT COUNT(*) FROM accounts")
    suspend fun count(): Int
}

/** Краткая строка журнала: заголовок, сумма по дебету и признак очереди. */
data class TransactionSummaryRow(
    @Embedded val transaction: TransactionEntity,
    val actorName: String?,
    val amountKop: Long,
    val pendingCount: Int
)

/** Обороты счёта: остаток до периода и движения внутри периода. */
data class AccountMovement(
    val accountId: Long,
    val openingDebit: Long,
    val openingCredit: Long,
    val periodDebit: Long,
    val periodCredit: Long
)

/** Проводка с строками для экрана деталей. */
data class TransactionWithLines(
    @Embedded val transaction: TransactionEntity,
    @Relation(parentColumn = "uuid", entityColumn = "transactionUuid")
    val lines: List<LineEntity>
)

/** Запросы журнала и импорта проводок. */
@Dao
interface TransactionDao {
    @Query(
        """
        SELECT t.*,
               u.name AS actorName,
               COALESCE((
                   SELECT SUM(l.amountKop) FROM lines l
                   WHERE l.transactionUuid = t.uuid AND l.side = 'DEBIT'
               ), 0) AS amountKop,
               (
                   SELECT COUNT(*) FROM outbox o
                   WHERE o.transactionUuid = t.uuid AND o.status IN ('pending', 'sending')
               ) AS pendingCount
          FROM transactions t
          LEFT JOIN users u ON u.id = t.actorId
         WHERE (:fromDate IS NULL OR t.date >= :fromDate)
           AND (:toDate IS NULL OR t.date <= :toDate)
           AND (:accountId IS NULL OR EXISTS (
                   SELECT 1 FROM lines l2
                   WHERE l2.transactionUuid = t.uuid AND l2.accountId = :accountId
               ))
           AND (:opType IS NULL OR t.opType = :opType)
           AND (:query IS NULL OR t.searchKey LIKE '%' || :query || '%')
           AND (:pendingOnly = 0 OR EXISTS (
                   SELECT 1 FROM outbox o2
                   WHERE o2.transactionUuid = t.uuid AND o2.status IN ('pending', 'sending')
               ))
         ORDER BY t.date DESC, t.createdAt DESC, t.uuid DESC
         LIMIT :limit OFFSET :offset
        """
    )
    fun observePage(
        fromDate: Long?,
        toDate: Long?,
        accountId: Long?,
        opType: String?,
        query: String?,
        pendingOnly: Int,
        limit: Int,
        offset: Int
    ): Flow<List<TransactionSummaryRow>>

    @Query(
        """
        SELECT COUNT(*)
          FROM transactions t
         WHERE (:fromDate IS NULL OR t.date >= :fromDate)
           AND (:toDate IS NULL OR t.date <= :toDate)
           AND (:accountId IS NULL OR EXISTS (
                   SELECT 1 FROM lines l2
                   WHERE l2.transactionUuid = t.uuid AND l2.accountId = :accountId
               ))
           AND (:opType IS NULL OR t.opType = :opType)
           AND (:query IS NULL OR t.searchKey LIKE '%' || :query || '%')
           AND (:pendingOnly = 0 OR EXISTS (
                   SELECT 1 FROM outbox o2
                   WHERE o2.transactionUuid = t.uuid AND o2.status IN ('pending', 'sending')
               ))
        """
    )
    fun observeCount(
        fromDate: Long?,
        toDate: Long?,
        accountId: Long?,
        opType: String?,
        query: String?,
        pendingOnly: Int
    ): Flow<Int>

    @Transaction
    @Query("SELECT * FROM transactions WHERE uuid = :uuid")
    fun observeDetails(uuid: String): Flow<TransactionWithLines?>

    @Query(
        """
        SELECT EXISTS(
            SELECT 1 FROM outbox
             WHERE transactionUuid = :uuid AND status IN ('pending', 'sending')
        )
        """
    )
    fun observePending(uuid: String): Flow<Boolean>

    /**
     * Обороты по счетам за период.
     *
     * `pending = 0` — подтверждённые проводки, `1` — очередь. Остаток на начало
     * считается только при заданном `fromDate`.
     */
    @Query(
        """
        SELECT l.accountId AS accountId,
               COALESCE(SUM(CASE
                   WHEN :fromDate IS NOT NULL AND t.date < :fromDate AND l.side = 'DEBIT'
                   THEN l.amountKop ELSE 0 END), 0) AS openingDebit,
               COALESCE(SUM(CASE
                   WHEN :fromDate IS NOT NULL AND t.date < :fromDate AND l.side = 'CREDIT'
                   THEN l.amountKop ELSE 0 END), 0) AS openingCredit,
               COALESCE(SUM(CASE
                   WHEN (:fromDate IS NULL OR t.date >= :fromDate)
                    AND (:toDate IS NULL OR t.date <= :toDate)
                    AND l.side = 'DEBIT'
                   THEN l.amountKop ELSE 0 END), 0) AS periodDebit,
               COALESCE(SUM(CASE
                   WHEN (:fromDate IS NULL OR t.date >= :fromDate)
                    AND (:toDate IS NULL OR t.date <= :toDate)
                    AND l.side = 'CREDIT'
                   THEN l.amountKop ELSE 0 END), 0) AS periodCredit
          FROM lines l
          JOIN transactions t ON t.uuid = l.transactionUuid
         WHERE (
                 :pendingFlag = 0 AND NOT EXISTS (
                     SELECT 1 FROM outbox o
                      WHERE o.transactionUuid = t.uuid
                        AND o.status IN ('pending', 'sending')
                 )
               ) OR (
                 :pendingFlag = 1 AND EXISTS (
                     SELECT 1 FROM outbox o
                      WHERE o.transactionUuid = t.uuid
                        AND o.status IN ('pending', 'sending')
                 )
               )
         GROUP BY l.accountId
        """
    )
    fun observeMovements(
        fromDate: Long?,
        toDate: Long?,
        pendingFlag: Int
    ): Flow<List<AccountMovement>>

    @Query("SELECT * FROM transactions WHERE uuid = :uuid")
    suspend fun get(uuid: String): TransactionEntity?

    @Query("SELECT * FROM transactions")
    suspend fun all(): List<TransactionEntity>

    @Upsert
    suspend fun upsertAll(transactions: List<TransactionEntity>)

    @Upsert
    suspend fun upsertLines(lines: List<LineEntity>)

    @Query("DELETE FROM lines WHERE transactionUuid = :uuid")
    suspend fun deleteLines(uuid: String)

    @Query(
        """
        DELETE FROM lines WHERE transactionUuid NOT IN (
            SELECT transactionUuid FROM outbox WHERE status IN ('pending', 'sending')
        )
        """
    )
    suspend fun deleteConfirmedLines()

    @Query(
        """
        DELETE FROM transactions WHERE uuid NOT IN (
            SELECT transactionUuid FROM outbox WHERE status IN ('pending', 'sending')
        )
        """
    )
    suspend fun deleteConfirmedTransactions()

    @Query("DELETE FROM lines WHERE transactionUuid IN (:uuids)")
    suspend fun deleteLinesFor(uuids: List<String>)

    @Query("SELECT COUNT(*) FROM transactions")
    suspend fun count(): Int

    @Query("SELECT COUNT(*) FROM lines")
    suspend fun lineCount(): Int
}

/** Запросы пользователей и настроек книги. */
@Dao
interface UserDao {
    @Query("SELECT * FROM users ORDER BY name")
    fun observeAll(): Flow<List<UserEntity>>

    @Query("SELECT * FROM users WHERE id = :id")
    suspend fun get(id: Long): UserEntity?

    @Upsert
    suspend fun upsertAll(users: List<UserEntity>)

    @Query("DELETE FROM users")
    suspend fun clear()

    @Query("SELECT COUNT(*) FROM users")
    suspend fun count(): Int
}

@Dao
interface SettingDao {
    @Query("SELECT * FROM settings ORDER BY key")
    fun observeAll(): Flow<List<SettingEntity>>

    @Query("SELECT value FROM settings WHERE key = :key")
    suspend fun value(key: String): String?

    @Upsert
    suspend fun upsertAll(settings: List<SettingEntity>)

    @Query("DELETE FROM settings")
    suspend fun clear()
}

/** Метаданные синхронизации в одной строке. */
@Dao
interface SyncMetaDao {
    @Query("SELECT * FROM sync_meta WHERE id = 1")
    fun observe(): Flow<SyncMetaEntity?>

    @Query("SELECT * FROM sync_meta WHERE id = 1")
    suspend fun get(): SyncMetaEntity?

    @Upsert
    suspend fun upsert(meta: SyncMetaEntity)

    @Query("DELETE FROM sync_meta")
    suspend fun clear()
}

/** Очередь отправки. */
@Dao
interface OutboxDao {
    @Query("SELECT COUNT(*) FROM outbox WHERE status IN ('pending', 'sending')")
    fun observePendingCount(): Flow<Int>

    @Query("SELECT transactionUuid FROM outbox WHERE status IN ('pending', 'sending')")
    suspend fun pendingUuids(): List<String>

    @Query("SELECT * FROM outbox ORDER BY createdAt")
    suspend fun all(): List<OutboxEntity>

    @Upsert
    suspend fun upsert(entry: OutboxEntity)

    @Query("DELETE FROM outbox")
    suspend fun clear()
}

/** Коммиты синхронизации. */
@Dao
interface CommitDao {
    @Query("SELECT * FROM commits ORDER BY resultSeq DESC")
    fun observeAll(): Flow<List<CommitEntity>>

    @Upsert
    suspend fun upsertAll(commits: List<CommitEntity>)

    @Query("SELECT COUNT(*) FROM commits")
    suspend fun count(): Int

    @Query("DELETE FROM commits")
    suspend fun clear()
}
