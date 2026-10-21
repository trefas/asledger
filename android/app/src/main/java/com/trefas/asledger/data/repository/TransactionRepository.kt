package com.trefas.asledger.data.repository

import com.trefas.asledger.data.db.TransactionDao
import com.trefas.asledger.data.model.JournalFilter
import com.trefas.asledger.data.model.TransactionDetails
import com.trefas.asledger.data.model.TransactionLine
import com.trefas.asledger.data.model.TransactionSummary
import com.trefas.asledger.util.normalizeSearch
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.combine
import kotlinx.coroutines.flow.map

/** Репозиторий журнала: страницы, фильтры и детали проводки. */
class TransactionRepository(private val dao: TransactionDao) {
    /**
     * Одна страница журнала.
     *
     * Пагинация ручная (`LIMIT`/`OFFSET`): для семейной книги этого достаточно,
     * а Paging 3 добавил бы зависимость и сложность без выигрыша.
     */
    fun observePage(
        filter: JournalFilter = JournalFilter(),
        limit: Int = DEFAULT_PAGE_SIZE,
        offset: Int = 0
    ): Flow<List<TransactionSummary>> {
        require(limit > 0) { "limit должен быть положительным" }
        require(offset >= 0) { "offset не может быть отрицательным" }
        return dao.observePage(
            fromDate = filter.fromDate,
            toDate = filter.toDate,
            accountId = filter.accountId,
            opType = filter.opType,
            query = filter.query.normalizedOrNull(),
            pendingOnly = if (filter.pendingOnly) 1 else 0,
            limit = limit,
            offset = offset
        ).map { rows ->
            rows.map { row ->
                TransactionSummary(
                    uuid = row.transaction.uuid,
                    serverId = row.transaction.serverId,
                    date = row.transaction.date,
                    opType = row.transaction.opType,
                    description = row.transaction.description,
                    actorName = row.actorName.orEmpty(),
                    amountKop = row.amountKop,
                    pending = row.pendingCount > 0
                )
            }
        }
    }

    /** Общее число проводок под фильтром — для счётчика страниц. */
    fun observeCount(filter: JournalFilter = JournalFilter()): Flow<Int> =
        dao.observeCount(
            fromDate = filter.fromDate,
            toDate = filter.toDate,
            accountId = filter.accountId,
            opType = filter.opType,
            query = filter.query.normalizedOrNull(),
            pendingOnly = if (filter.pendingOnly) 1 else 0
        )

    /** Детали проводки с признаком локальной очереди. */
    fun observeDetails(uuid: String): Flow<TransactionDetails?> =
        combine(
            dao.observeDetails(uuid),
            dao.observePending(uuid)
        ) { withLines, pending ->
            withLines?.let { details ->
                TransactionDetails(
                    uuid = details.transaction.uuid,
                    opType = details.transaction.opType,
                    date = details.transaction.date,
                    description = details.transaction.description,
                    actorId = details.transaction.actorId,
                    lines = details.lines
                        .sortedWith(compareBy({ it.serverId ?: Long.MAX_VALUE }, { it.id }))
                        .map { line ->
                            TransactionLine(
                                accountId = line.accountId,
                                amountKop = line.amountKop,
                                side = line.side
                            )
                        },
                    pending = pending
                )
            }
        }

    companion object {
        /** Размер страницы журнала по умолчанию. */
        const val DEFAULT_PAGE_SIZE = 50
    }
}

private fun String.normalizedOrNull(): String? =
    trim().takeIf { it.isNotEmpty() }?.let { normalizeSearch(it) }
