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

/** Счёт и число использований в проводках за период. */
data class FrequentAccountRow(
    val accountId: Long,
    val uses: Int
)

/** Краткая строка журнала: заголовок, сумма по дебету и признак очереди. */
data class TransactionSummaryRow(
    @Embedded val transaction: TransactionEntity,
    val actorName: String?,
    val amountKop: Long,
    val pendingCount: Int,
    /** synced, pending, sending или rejected. */
    val syncStatus: String
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
               ) AS pendingCount,
               COALESCE((
                   SELECT o.status FROM outbox o
                   WHERE o.transactionUuid = t.uuid
                   ORDER BY o.id DESC LIMIT 1
               ), 'synced') AS syncStatus
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
           AND (:syncStatus IS NULL OR COALESCE((
                   SELECT o3.status FROM outbox o3
                   WHERE o3.transactionUuid = t.uuid
                   ORDER BY o3.id DESC LIMIT 1
               ), 'synced') = :syncStatus)
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
        syncStatus: String?,
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
           AND (:syncStatus IS NULL OR COALESCE((
                   SELECT o3.status FROM outbox o3
                   WHERE o3.transactionUuid = t.uuid
                   ORDER BY o3.id DESC LIMIT 1
               ), 'synced') = :syncStatus)
        """
    )
    fun observeCount(
        fromDate: Long?,
        toDate: Long?,
        accountId: Long?,
        opType: String?,
        query: String?,
        pendingOnly: Int,
        syncStatus: String?
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

    @Query(
        """
        SELECT status FROM outbox
         WHERE transactionUuid = :uuid
         ORDER BY id DESC LIMIT 1
        """
    )
    fun observeSyncStatus(uuid: String): Flow<String?>

    @Query("DELETE FROM transactions WHERE uuid = :uuid")
    suspend fun delete(uuid: String)

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

    /** Частые счета за период: подсказки для быстрого ввода. */
    @Query(
        """
        SELECT l.accountId AS accountId, COUNT(*) AS uses
          FROM lines l
          JOIN transactions t ON t.uuid = l.transactionUuid
         WHERE t.date >= :since
         GROUP BY l.accountId
         ORDER BY uses DESC, l.accountId
         LIMIT :limit
        """
    )
    fun observeFrequentAccounts(since: Long, limit: Int): Flow<List<FrequentAccountRow>>

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
            SELECT transactionUuid FROM outbox
             WHERE status IN ('pending', 'sending', 'rejected')
        )
        """
    )
    suspend fun deleteConfirmedLines()

    @Query(
        """
        DELETE FROM transactions WHERE uuid NOT IN (
            SELECT transactionUuid FROM outbox
             WHERE status IN ('pending', 'sending', 'rejected')
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

    @Query("SELECT * FROM settings ORDER BY key")
    suspend fun all(): List<SettingEntity>

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

/** Запись очереди вместе с данными проводки для экрана синхронизации. */
data class OutboxRow(
    @Embedded val outbox: OutboxEntity,
    val description: String?,
    val amountKop: Long?,
    val date: Long?
)

/** Очередь отправки. */
@Dao
interface OutboxDao {
    @Query("SELECT COUNT(*) FROM outbox WHERE status IN ('pending', 'sending')")
    fun observePendingCount(): Flow<Int>

    @Query("SELECT * FROM outbox WHERE status IN ('pending', 'sending') ORDER BY createdAt")
    fun observeActive(): Flow<List<OutboxEntity>>

    @Query("SELECT * FROM outbox WHERE status IN ('pending', 'sending') ORDER BY createdAt")
    suspend fun active(): List<OutboxEntity>

    @Query("SELECT * FROM outbox WHERE status = 'synced' ORDER BY updatedAt DESC")
    fun observeSynced(): Flow<List<OutboxEntity>>

    @Query("SELECT * FROM outbox WHERE status = 'rejected' ORDER BY updatedAt DESC")
    fun observeRejected(): Flow<List<OutboxEntity>>

    @Query("SELECT * FROM outbox ORDER BY updatedAt DESC")
    fun observeAll(): Flow<List<OutboxEntity>>

    @Query(
        """
        SELECT o.*, t.description AS description,
               COALESCE((SELECT SUM(l.amountKop) FROM lines l
                         WHERE l.transactionUuid = o.transactionUuid AND l.side = 'DEBIT'), 0)
                   AS amountKop,
               t.date AS date
          FROM outbox o
          LEFT JOIN transactions t ON t.uuid = o.transactionUuid
         ORDER BY o.updatedAt DESC
        """
    )
    fun observeRows(): Flow<List<OutboxRow>>

    @Query("SELECT * FROM outbox WHERE transactionUuid = :uuid")
    suspend fun byTransaction(uuid: String): OutboxEntity?

    @Query("SELECT * FROM outbox WHERE transactionUuid = :uuid")
    fun observeByTransaction(uuid: String): Flow<OutboxEntity?>

    @Query("SELECT COUNT(*) FROM outbox WHERE status IN ('pending', 'sending')")
    suspend fun pendingCount(): Int

    @Query("SELECT transactionUuid FROM outbox WHERE status IN ('pending', 'sending')")
    suspend fun pendingUuids(): List<String>

    @Query("SELECT * FROM outbox ORDER BY createdAt")
    suspend fun all(): List<OutboxEntity>

    @Upsert
    suspend fun upsert(entry: OutboxEntity)

    @Query("DELETE FROM outbox WHERE transactionUuid = :uuid")
    suspend fun deleteByTransaction(uuid: String)

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
