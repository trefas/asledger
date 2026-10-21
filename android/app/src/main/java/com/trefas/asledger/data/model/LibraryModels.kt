package com.trefas.asledger.data.model

/** Тип счёта из матрицы ASL-2.2. */
enum class AccountType(val serverValue: String, val title: String) {
    ASSET("актив", "Актив"),
    LIABILITY("пассив", "Пассив"),
    CAPITAL("капитал", "Капитал"),
    INCOME("доход", "Доход"),
    EXPENSE("расход", "Расход");

    companion object {
        fun fromServer(value: String): AccountType? =
            entries.firstOrNull { it.serverValue == value }
    }
}

/** Счёт плана счетов в локальной реплике. */
data class Account(
    val id: Long,
    val name: String,
    val fullPath: String,
    val type: AccountType,
    val parentId: Long?,
    val isLeaf: Boolean,
    val minBalanceKop: Long,
    val isArchived: Boolean,
    val isSystem: Boolean,
    val createdAt: Long
)

/** Узел дерева счетов. */
data class AccountNode(
    val account: Account,
    val children: List<AccountNode> = emptyList()
)

/** Пользователь книги. */
data class LibraryUser(
    val id: Long,
    val name: String
)

/** Настройка книги. */
data class LibrarySetting(
    val key: String,
    val value: String
)

/** Метаданные синхронизации. */
data class SyncMeta(
    val headSeq: Long,
    val chainHash: String,
    val formatVersion: Int,
    val initialLoaded: Boolean,
    val lastSyncAt: Long
)

/** Статус проводки в локальной очереди. */
enum class SyncStatus(val serverValue: String, val title: String) {
    SYNCED("synced", "Синхронизировано"),
    PENDING("pending", "К отправке"),
    SENDING("sending", "Отправляется"),
    REJECTED("rejected", "Отклонено"),
    DRAFT("draft", "Черновик");

    companion object {
        fun fromServer(value: String?): SyncStatus =
            entries.firstOrNull { it.serverValue == value } ?: SYNCED
    }
}

/** Краткая запись журнала. */
data class TransactionSummary(
    val uuid: String,
    val serverId: Long?,
    val date: Long,
    val opType: String,
    val description: String,
    val actorName: String,
    val amountKop: Long,
    val pending: Boolean,
    val syncStatus: SyncStatus = SyncStatus.SYNCED
)

/** Строка проводки в деталях. */
data class TransactionLine(
    val accountId: Long,
    val amountKop: Long,
    val side: String
)

/** Проводка с строками. */
data class TransactionDetails(
    val uuid: String,
    val opType: String,
    val date: Long,
    val description: String,
    val actorId: Long,
    val lines: List<TransactionLine>,
    val pending: Boolean,
    val syncStatus: SyncStatus = SyncStatus.SYNCED,
    val errorCode: String? = null,
    val lastError: String? = null,
    val warnings: List<String> = emptyList(),
    val duplicateUuids: List<String> = emptyList()
)

/** Фильтр журнала. */
data class JournalFilter(
    val query: String = "",
    val fromDate: Long? = null,
    val toDate: Long? = null,
    val accountId: Long? = null,
    val opType: String? = null,
    val pendingOnly: Boolean = false,
    val syncStatus: SyncStatus? = null
)

/** Строка новой проводки: роль и сумма; сторону назначает матрица сервера. */
data class DraftLine(
    val accountId: Long,
    val amountKop: Long,
    val role: String,
    val side: String
)

/** Локальный черновик проводки. */
data class EntryDraft(
    val uuid: String? = null,
    val opType: String,
    val date: Long,
    val description: String = "",
    val lines: List<DraftLine>
)

/** Запись очереди для экрана синхронизации. */
data class OutboxItem(
    val transactionUuid: String,
    val commitId: String,
    val status: SyncStatus,
    val attempts: Int,
    val lastError: String?,
    val errorCode: String?,
    val warnings: List<String>,
    val duplicateUuids: List<String>,
    val description: String,
    val amountKop: Long,
    val date: Long
)

/** Коммит синхронизации для проверки цепочки и диагностики. */
data class SyncCommit(
    val id: Long,
    val clientId: String,
    val actorId: Long,
    val baseSeq: Long,
    val opsJson: String,
    val resultSeq: Long,
    val hash: String,
    val warnings: List<String>,
    val createdAt: Long
)

/** Роль матрицы ASL-2.2 в доменном виде. */
data class RoleRule(
    val role: String,
    val side: String,
    val accountTypes: List<AccountType>,
    val directionRequired: Boolean,
    val reserved: Boolean
)

/** Матрица ролей для типа операции. */
data class OperationRoles(
    val opType: String,
    val title: String,
    val roles: List<RoleRule>,
    val minDebit: Int,
    val minCredit: Int
)

/** Строка отчёта об остатках. */
data class AccountBalanceRow(
    val account: Account,
    val ownKop: Long,
    val totalKop: Long,
    val pendingOwnKop: Long,
    val pendingTotalKop: Long,
    val openingKop: Long,
    val openingTotalKop: Long,
    val periodDebitKop: Long,
    val periodCreditKop: Long,
    val periodDebitTotalKop: Long,
    val periodCreditTotalKop: Long
)

/** Локальный отчёт об остатках и оборотах. */
data class BalanceReport(
    val rows: List<AccountBalanceRow>,
    val assetsKop: Long,
    val liabilitiesKop: Long,
    val netWorthKop: Long,
    val pendingAssetsKop: Long,
    val pendingLiabilitiesKop: Long,
    val pendingNetWorthKop: Long
)

/** Остаток по природе счёта: актив/расход — дебет минус кредит, остальные — наоборот. */
fun balanceFor(type: AccountType, debitKop: Long, creditKop: Long): Long =
    when (type) {
        AccountType.ASSET, AccountType.EXPENSE -> debitKop - creditKop
        AccountType.LIABILITY, AccountType.CAPITAL, AccountType.INCOME -> creditKop - debitKop
    }
