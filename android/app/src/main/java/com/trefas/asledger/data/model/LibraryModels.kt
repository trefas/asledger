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

/** Краткая запись журнала. */
data class TransactionSummary(
    val uuid: String,
    val serverId: Long?,
    val date: Long,
    val opType: String,
    val description: String,
    val actorName: String,
    val amountKop: Long,
    val pending: Boolean
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
    val pending: Boolean
)

/** Фильтр журнала. */
data class JournalFilter(
    val query: String = "",
    val fromDate: Long? = null,
    val toDate: Long? = null,
    val accountId: Long? = null,
    val opType: String? = null,
    val pendingOnly: Boolean = false
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
