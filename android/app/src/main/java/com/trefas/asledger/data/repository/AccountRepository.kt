package com.trefas.asledger.data.repository

import com.trefas.asledger.data.db.AccountDao
import com.trefas.asledger.data.db.AccountEntity
import com.trefas.asledger.data.model.Account
import com.trefas.asledger.data.model.AccountNode
import com.trefas.asledger.util.normalizeSearch
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.map

/** Репозиторий плана счетов: дерево, листовые счета и поиск. */
class AccountRepository(private val dao: AccountDao) {
    /** Все счета или только активные, в порядке полного пути. */
    fun observeAccounts(includeArchived: Boolean = false): Flow<List<Account>> {
        val source = if (includeArchived) dao.observeAll() else dao.observeActive()
        return source.map { entities -> entities.map { it.toDomain() } }
    }

    /** Дерево счетов, отсортированное по именам, как в отчёте сервера. */
    fun observeTree(includeArchived: Boolean = false): Flow<List<AccountNode>> =
        observeAccounts(includeArchived).map { buildTree(it) }

    /** Листовые счета, допустимые в проводках. */
    fun observeLeaves(): Flow<List<Account>> =
        dao.observeLeaves().map { entities -> entities.map { it.toDomain() } }

    /**
     * Поиск по имени и полному пути без учёта регистра.
     *
     * Нормализация выполняется в Kotlin: SQLite `NOCASE` не работает с
     * кириллицей, а индекс `search_key` хранит уже нижний регистр.
     */
    fun search(query: String): Flow<List<Account>> {
        val normalized = normalizeSearch(query)
        val source = if (normalized.isEmpty()) dao.observeActive() else dao.search(normalized)
        return source.map { entities -> entities.map { it.toDomain() } }
    }
}

/** Строит дерево по `parent_id`; счета с потерянным родителем становятся корнями. */
internal fun buildTree(accounts: List<Account>): List<AccountNode> {
    val ids = accounts.mapTo(HashSet()) { it.id }
    val childrenByParent = accounts
        .filter { it.parentId != null && it.parentId in ids }
        .groupBy { it.parentId }
    val roots = accounts.filter { it.parentId == null || it.parentId !in ids }

    fun node(account: Account): AccountNode = AccountNode(
        account = account,
        children = childrenByParent[account.id]
            .orEmpty()
            .sortedBy { it.name }
            .map(::node)
    )

    return roots.sortedBy { it.name }.map(::node)
}

/** Преобразует плоский список счетов в дерево (для тестов и повторного использования). */
internal fun List<AccountEntity>.toDomainTree(): List<AccountNode> =
    buildTree(map { it.toDomain() })
