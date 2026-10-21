package com.trefas.asledger.ui.screens

import androidx.lifecycle.ViewModel
import androidx.lifecycle.ViewModelProvider
import androidx.lifecycle.viewModelScope
import com.trefas.asledger.data.model.Account
import com.trefas.asledger.data.model.AccountType
import com.trefas.asledger.data.model.DraftLine
import com.trefas.asledger.data.model.EntryDraft
import com.trefas.asledger.data.model.OperationRoles
import com.trefas.asledger.data.model.RoleRule
import com.trefas.asledger.data.model.SyncStatus
import com.trefas.asledger.data.model.TransactionDetails
import com.trefas.asledger.data.repository.AccountRepository
import com.trefas.asledger.data.repository.LocalEntryRepository
import com.trefas.asledger.data.repository.MOBILE_OP_TYPES
import com.trefas.asledger.data.repository.RoleRepository
import com.trefas.asledger.data.repository.TransactionRepository
import com.trefas.asledger.data.security.TokenStore
import com.trefas.asledger.data.settings.ServerConfig
import com.trefas.asledger.util.parseRubles
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

/** Строка формы проводки. */
data class EntryLineUi(
    val key: Long,
    val accountId: Long? = null,
    val amount: String = "",
    val role: String = "",
    val side: String = "DEBIT",
    val error: String? = null
)

/** Состояние формы новой или редактируемой проводки. */
data class EntryUiState(
    val opTypes: List<String> = MOBILE_OP_TYPES,
    val opType: String = "трата",
    val date: Long = System.currentTimeMillis(),
    val description: String = "",
    val lines: List<EntryLineUi> = emptyList(),
    val roles: OperationRoles? = null,
    val accounts: List<Account> = emptyList(),
    val frequentAccountIds: List<Long> = emptyList(),
    val loadingRoles: Boolean = true,
    val rolesError: String? = null,
    val editingUuid: String? = null,
    val syncedSource: Boolean = false,
    val saving: Boolean = false,
    val saved: Boolean = false,
    val message: String? = null
) {
    val debitKop: Long
        get() = lines.filter { it.side == "DEBIT" }
            .sumOf { parseRubles(it.amount) ?: 0 }

    val creditKop: Long
        get() = lines.filter { it.side == "CREDIT" }
            .sumOf { parseRubles(it.amount) ?: 0 }

    val balanced: Boolean
        get() = debitKop > 0 && debitKop == creditKop

    val isEditing: Boolean get() = editingUuid != null
}

/**
 * Форма проводки.
 *
 * Сторону каждой строки задаёт роль из матрицы сервера: приложение не
 * дублирует правила ASL-2.2. При офлайне используется кэш ролей, загруженный
 * при первом подключении или последней синхронизации.
 */
class EntryViewModel(
    private val editUuid: String?,
    private val copySource: Boolean,
    private val preselectedAccountId: Long?,
    private val configFlow: Flow<ServerConfig?>,
    private val tokenStore: TokenStore,
    private val rolesRepository: RoleRepository,
    private val accountsRepository: AccountRepository,
    private val transactionsRepository: TransactionRepository,
    private val localEntries: LocalEntryRepository,
    private val onSaved: () -> Unit = {}
) : ViewModel() {
    private val _state = MutableStateFlow(EntryUiState())
    val state: StateFlow<EntryUiState> = _state.asStateFlow()
    private var nextLineKey = 1L

    init {
        viewModelScope.launch {
            accountsRepository.observeLeaves().collect { accounts ->
                _state.update { it.copy(accounts = accounts) }
                _state.update { current ->
                    if (current.lines.isEmpty() && accounts.isNotEmpty()) {
                        current.withDefaultLines()
                    } else {
                        current
                    }
                }
            }
        }
        viewModelScope.launch {
            transactionsRepository.observeFrequentAccountIds(
                System.currentTimeMillis() - 90L * 24 * 60 * 60 * 1000
            ).collect { ids ->
                _state.update { it.copy(frequentAccountIds = ids) }
            }
        }
        viewModelScope.launch { load() }
    }

    private suspend fun load() {
        val details = if (editUuid != null) {
            transactionsRepository.observeDetails(editUuid).first()
        } else {
            null
        }
        if (details != null) {
            val editing = !copySource && details.syncStatus != SyncStatus.SYNCED
            _state.update {
                it.copy(
                    opType = details.opType,
                    // Повтор — новая проводка сегодняшним днём, а не копия даты.
                    date = if (copySource) System.currentTimeMillis() else details.date,
                    description = details.description,
                    editingUuid = if (editing) details.uuid else null,
                    syncedSource = !copySource && details.syncStatus == SyncStatus.SYNCED
                )
            }
        }
        loadRoles(_state.value.opType, details)
    }

    private suspend fun loadRoles(opType: String, details: TransactionDetails?) {
        _state.update { it.copy(loadingRoles = true, rolesError = null, roles = null) }
        val cached = rolesRepository.cached(opType)
        if (cached != null) {
            applyRoles(cached, details)
        }
        val config = configFlow.first()
        val token = tokenStore.load()
        if (config != null && config.hasSession && token != null) {
            when (val result = rolesRepository.fetch(config, token, opType)) {
                is com.trefas.asledger.data.network.ApiResult.Ok ->
                    applyRoles(result.value, details)
                is com.trefas.asledger.data.network.ApiResult.Failure -> {
                    if (cached == null) {
                        _state.update {
                            it.copy(
                                loadingRoles = false,
                                rolesError = "Матрица ролей недоступна офлайн. " +
                                    "Подключитесь к серверу один раз, чтобы заполнить кэш."
                            )
                        }
                    }
                }
            }
        } else if (cached == null) {
            _state.update {
                it.copy(
                    loadingRoles = false,
                    rolesError = "Нет подключения к серверу, а матрица ролей ещё не загружена."
                )
            }
        }
    }

    private fun applyRoles(roles: OperationRoles, details: TransactionDetails?) {
        _state.update { current ->
            val lines = if (details != null && current.lines.isEmpty()) {
                details.lines.map { line ->
                    EntryLineUi(
                        key = nextLineKey++,
                        accountId = line.accountId,
                        amount = kopToInput(line.amountKop),
                        side = line.side,
                        role = roleFor(roles, line.side, line.accountId)?.role.orEmpty()
                    )
                }
            } else if (current.lines.isEmpty()) {
                listOfNotNull(
                    firstRole(roles, "DEBIT")?.let { role ->
                        EntryLineUi(
                            key = nextLineKey++,
                            role = role.role,
                            side = role.side
                        )
                    },
                    firstRole(roles, "CREDIT")?.let { role ->
                        EntryLineUi(
                            key = nextLineKey++,
                            role = role.role,
                            side = role.side
                        )
                    }
                )
            } else {
                current.lines.map { line ->
                    val rule = roles.roles.firstOrNull { it.role == line.role }
                    if (rule == null) {
                        val fallback = firstRole(roles, line.side)
                        line.copy(role = fallback?.role.orEmpty(), side = fallback?.side ?: line.side)
                    } else {
                        line.copy(side = rule.side)
                    }
                }
            }
            val withPreselected = if (preselectedAccountId != null && !current.syncedSource) {
                val target = lines.indexOfFirst { it.side == "DEBIT" }
                if (target >= 0) {
                    lines.toMutableList().also {
                        it[target] = it[target].copy(accountId = preselectedAccountId)
                    }
                } else {
                    lines
                }
            } else {
                lines
            }
            current.copy(
                roles = roles,
                lines = withPreselected,
                loadingRoles = false,
                rolesError = null
            )
        }
    }

    fun selectOpType(opType: String) {
        if (opType == _state.value.opType) return
        _state.update {
            it.copy(
                opType = opType,
                lines = emptyList(),
                roles = null,
                rolesError = null
            )
        }
        viewModelScope.launch { loadRoles(opType, null) }
    }

    fun setDescription(value: String) = _state.update { it.copy(description = value) }

    fun setDate(value: Long) = _state.update { it.copy(date = value) }

    fun addLine(side: String) {
        val current = _state.value
        val role = current.roles?.let { firstRole(it, side) } ?: return
        _state.update {
            it.copy(
                lines = it.lines + EntryLineUi(
                    key = nextLineKey++,
                    role = role.role,
                    side = role.side
                )
            )
        }
    }

    fun removeLine(key: Long) {
        _state.update { current ->
            if (current.lines.size <= 1) {
                current
            } else {
                current.copy(lines = current.lines.filterNot { it.key == key })
            }
        }
    }

    fun setAccount(key: Long, accountId: Long) {
        _state.update { current ->
            current.copy(
                lines = current.lines.map { line ->
                    if (line.key == key) line.copy(accountId = accountId, error = null) else line
                }
            )
        }
    }

    fun setAmount(key: Long, value: String) {
        if (value.any { !it.isDigit() && it != ',' && it != '.' && it != ' ' }) return
        _state.update { current ->
            current.copy(
                lines = current.lines.map { line ->
                    if (line.key == key) line.copy(amount = value, error = null) else line
                }
            )
        }
    }

    fun addAmount(key: Long, kopecks: Long) {
        val line = _state.value.lines.firstOrNull { it.key == key } ?: return
        val current = parseRubles(line.amount) ?: 0
        setAmount(key, kopToInput(current + kopecks))
    }

    fun setRole(key: Long, roleName: String) {
        val rule = _state.value.roles?.roles?.firstOrNull { it.role == roleName } ?: return
        _state.update { current ->
            current.copy(
                lines = current.lines.map { line ->
                    if (line.key == key) {
                        line.copy(role = rule.role, side = rule.side, error = null)
                    } else {
                        line
                    }
                }
            )
        }
    }

    fun rolesFor(line: EntryLineUi): List<RoleRule> {
        val roles = _state.value.roles ?: return emptyList()
        val accountType = _state.value.accounts.firstOrNull { it.id == line.accountId }?.type
        return roles.roles.filter { rule ->
            rule.side == line.side &&
                (
                    accountType == null ||
                        rule.accountTypes.isEmpty() ||
                        rule.accountTypes.contains(accountType)
                    )
        }
    }

    fun save() {
        val current = _state.value
        val errors = validate(current)
        if (errors.isNotEmpty()) {
            _state.update {
                it.copy(
                    lines = it.lines.map { line ->
                        line.copy(error = errors[line.key])
                    },
                    message = "Проверьте строки: дебет и кредит должны совпадать."
                )
            }
            return
        }
        _state.update { it.copy(saving = true, message = null) }
        viewModelScope.launch {
            try {
                val draft = EntryDraft(
                    opType = current.opType,
                    date = current.date,
                    description = current.description.trim(),
                    lines = current.lines.map { line ->
                        DraftLine(
                            accountId = requireNotNull(line.accountId),
                            amountKop = requireNotNull(parseRubles(line.amount)),
                            role = line.role,
                            side = line.side
                        )
                    }
                )
                val config = configFlow.first()
                val clientId = config?.clientId ?: "android"
                val actorId = config?.userId ?: 0
                if (current.editingUuid != null) {
                    localEntries.update(current.editingUuid, draft, clientId, actorId)
                } else {
                    localEntries.save(draft, clientId, actorId)
                }
                onSaved()
                _state.update { it.copy(saving = false, saved = true) }
            } catch (failure: Exception) {
                _state.update {
                    it.copy(
                        saving = false,
                        message = failure.message ?: "Не удалось сохранить проводку"
                    )
                }
            }
        }
    }

    private fun validate(current: EntryUiState): Map<Long, String> {
        val errors = LinkedHashMap<Long, String>()
        current.lines.forEach { line ->
            when {
                line.accountId == null -> errors[line.key] = "Выберите счёт"
                parseRubles(line.amount)?.let { it > 0 } != true ->
                    errors[line.key] = "Введите сумму больше нуля"
                line.role.isBlank() -> errors[line.key] = "Выберите роль"
                else -> {
                    val rule = current.roles?.roles?.firstOrNull { it.role == line.role }
                    val account = current.accounts.firstOrNull { it.id == line.accountId }
                    if (rule != null &&
                        account != null &&
                        rule.accountTypes.isNotEmpty() &&
                        !rule.accountTypes.contains(account.type)
                    ) {
                        errors[line.key] = "Роль не подходит к типу счёта"
                    }
                }
            }
        }
        if (errors.isEmpty() && current.debitKop != current.creditKop) {
            current.lines.forEach { line -> errors[line.key] = "Дебет и кредит не совпадают" }
        }
        val minDebit = current.roles?.minDebit ?: 1
        val minCredit = current.roles?.minCredit ?: 1
        if (current.lines.count { it.side == "DEBIT" } < minDebit) {
            errors[current.lines.firstOrNull()?.key ?: 0] = "Добавьте строку дебета"
        }
        if (current.lines.count { it.side == "CREDIT" } < minCredit) {
            errors[current.lines.firstOrNull()?.key ?: 0] = "Добавьте строку кредита"
        }
        return errors
    }

    private fun EntryUiState.withDefaultLines(): EntryUiState {
        val matrix = roles ?: return this
        return copy(
            lines = listOfNotNull(
                firstRole(matrix, "DEBIT")?.let {
                    EntryLineUi(
                        key = nextLineKey++,
                        role = it.role,
                        side = it.side,
                        accountId = preselectedAccountId
                    )
                },
                firstRole(matrix, "CREDIT")?.let {
                    EntryLineUi(key = nextLineKey++, role = it.role, side = it.side)
                }
            )
        )
    }

    private fun firstRole(roles: OperationRoles, side: String): RoleRule? =
        roles.roles.firstOrNull { it.side == side }

    private fun roleFor(roles: OperationRoles, side: String, accountId: Long): RoleRule? {
        val account = _state.value.accounts.firstOrNull { it.id == accountId }
        val type: AccountType? = account?.type
        return roles.roles.firstOrNull { rule ->
            rule.side == side &&
                (type == null || rule.accountTypes.isEmpty() || rule.accountTypes.contains(type))
        } ?: roles.roles.firstOrNull { it.side == side }
    }

    private fun kopToInput(kopecks: Long): String {
        val rubles = kopecks / 100
        val kop = kopecks % 100
        return if (kop == 0L) rubles.toString() else "$rubles,${kop.toString().padStart(2, '0')}"
    }

    class Factory(
        private val editUuid: String?,
        private val copySource: Boolean,
        private val preselectedAccountId: Long?,
        private val configFlow: Flow<ServerConfig?>,
        private val tokenStore: TokenStore,
        private val rolesRepository: RoleRepository,
        private val accountsRepository: AccountRepository,
        private val transactionsRepository: TransactionRepository,
        private val localEntries: LocalEntryRepository,
        private val onSaved: () -> Unit
    ) : ViewModelProvider.Factory {
        @Suppress("UNCHECKED_CAST")
        override fun <T : ViewModel> create(modelClass: Class<T>): T = EntryViewModel(
            editUuid = editUuid,
            copySource = copySource,
            preselectedAccountId = preselectedAccountId,
            configFlow = configFlow,
            tokenStore = tokenStore,
            rolesRepository = rolesRepository,
            accountsRepository = accountsRepository,
            transactionsRepository = transactionsRepository,
            localEntries = localEntries,
            onSaved = onSaved
        ) as T
    }
}
