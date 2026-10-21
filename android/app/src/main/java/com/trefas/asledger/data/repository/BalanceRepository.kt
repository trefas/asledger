package com.trefas.asledger.data.repository

import com.trefas.asledger.data.db.AccountDao
import com.trefas.asledger.data.db.AccountMovement
import com.trefas.asledger.data.db.TransactionDao
import com.trefas.asledger.data.model.Account
import com.trefas.asledger.data.model.AccountBalanceRow
import com.trefas.asledger.data.model.AccountType
import com.trefas.asledger.data.model.BalanceReport
import com.trefas.asledger.data.model.balanceFor
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.combine

/**
 * Считает локальные остатки и обороты без обращения к серверу.
 *
 * Формула совпадает с `store.Balance`: для активов и расходов дебет минус
 * кредит, для остальных типов — наоборот. Итог по счёту включает поддерево,
 * как в отчёте сервера. Отдельно считается очередь: она не меняет
 * подтверждённый остаток, но показывается как «ожидает отправки».
 */
class BalanceRepository(
    private val accountDao: AccountDao,
    private val transactionDao: TransactionDao
) {
    fun observeReport(fromDate: Long? = null, toDate: Long? = null): Flow<BalanceReport> =
        combine(
            accountDao.observeAll(),
            transactionDao.observeMovements(fromDate, toDate, pendingFlag = 0),
            transactionDao.observeMovements(fromDate, toDate, pendingFlag = 1)
        ) { accountEntities, confirmed, pending ->
            buildReport(
                accounts = accountEntities.map { it.toDomain() },
                confirmed = confirmed,
                pending = pending
            )
        }
}

/** Строит отчёт по счетам и движениям; вынесено для тестов. */
internal fun buildReport(
    accounts: List<Account>,
    confirmed: List<AccountMovement>,
    pending: List<AccountMovement>
): BalanceReport {
    val accountsById = accounts.associateBy { it.id }
    val ids = accountsById.keys
    val childrenByParent = accounts
        .filter { it.parentId != null && it.parentId in ids }
        .groupBy { it.parentId }
    val roots = accounts
        .filter { it.parentId == null || it.parentId !in ids }
        .sortedBy { it.name }

    val confirmedById = confirmed.associateBy { it.accountId }
    val pendingById = pending.associateBy { it.accountId }

    val closing = mutableMapOf<Long, Long>()
    val opening = mutableMapOf<Long, Long>()
    val pendingClosing = mutableMapOf<Long, Long>()
    val periodDebit = mutableMapOf<Long, Long>()
    val periodCredit = mutableMapOf<Long, Long>()

    accounts.forEach { account ->
        val movement = confirmedById[account.id]
        val debit = movement?.periodDebit ?: 0
        val credit = movement?.periodCredit ?: 0
        val openingDebit = movement?.openingDebit ?: 0
        val openingCredit = movement?.openingCredit ?: 0
        opening[account.id] = balanceFor(account.type, openingDebit, openingCredit)
        closing[account.id] = balanceFor(
            account.type,
            openingDebit + debit,
            openingCredit + credit
        )
        periodDebit[account.id] = debit
        periodCredit[account.id] = credit

        val pendingMovement = pendingById[account.id]
        pendingClosing[account.id] = balanceFor(
            account.type,
            (pendingMovement?.openingDebit ?: 0) + (pendingMovement?.periodDebit ?: 0),
            (pendingMovement?.openingCredit ?: 0) + (pendingMovement?.periodCredit ?: 0)
        )
    }

    fun subtreeTotal(
        account: Account,
        own: Map<Long, Long>,
        children: Map<Long?, List<Account>>
    ): Long {
        var sum = own[account.id] ?: 0
        children[account.id].orEmpty().forEach { child ->
            sum += subtreeTotal(child, own, children)
        }
        return sum
    }

    fun subtreePeriod(account: Account, values: Map<Long, Long>): Long {
        var sum = values[account.id] ?: 0
        childrenByParent[account.id].orEmpty().forEach { child ->
            sum += subtreePeriod(child, values)
        }
        return sum
    }

    val rows = mutableListOf<AccountBalanceRow>()

    fun walk(account: Account) {
        rows += AccountBalanceRow(
            account = account,
            ownKop = closing[account.id] ?: 0,
            totalKop = subtreeTotal(account, closing, childrenByParent),
            pendingOwnKop = pendingClosing[account.id] ?: 0,
            pendingTotalKop = subtreeTotal(account, pendingClosing, childrenByParent),
            openingKop = opening[account.id] ?: 0,
            openingTotalKop = subtreeTotal(account, opening, childrenByParent),
            periodDebitKop = periodDebit[account.id] ?: 0,
            periodCreditKop = periodCredit[account.id] ?: 0,
            periodDebitTotalKop = subtreePeriod(account, periodDebit),
            periodCreditTotalKop = subtreePeriod(account, periodCredit)
        )
        childrenByParent[account.id].orEmpty()
            .sortedBy { it.name }
            .forEach(::walk)
    }

    roots.forEach(::walk)

    fun rootTotal(type: AccountType, values: Map<Long, Long>): Long =
        roots.filter { it.type == type }
            .sumOf { subtreeTotal(it, values, childrenByParent) }

    val assets = rootTotal(AccountType.ASSET, closing)
    val liabilities = rootTotal(AccountType.LIABILITY, closing)
    val pendingAssets = rootTotal(AccountType.ASSET, pendingClosing)
    val pendingLiabilities = rootTotal(AccountType.LIABILITY, pendingClosing)

    return BalanceReport(
        rows = rows,
        assetsKop = assets,
        liabilitiesKop = liabilities,
        netWorthKop = assets - liabilities,
        pendingAssetsKop = pendingAssets,
        pendingLiabilitiesKop = pendingLiabilities,
        pendingNetWorthKop = pendingAssets - pendingLiabilities
    )
}
