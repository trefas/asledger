package com.trefas.asledger.ui.screens

import androidx.annotation.StringRes
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.trefas.asledger.R
import com.trefas.asledger.data.model.OutboxItem
import com.trefas.asledger.data.network.ConnectionProblem
import com.trefas.asledger.data.network.toProblem
import com.trefas.asledger.data.repository.LocalEntryRepository
import com.trefas.asledger.data.settings.ServerConfig
import com.trefas.asledger.data.sync.SyncManager
import com.trefas.asledger.data.sync.SyncOutcome
import com.trefas.asledger.data.sync.SyncPhase
import com.trefas.asledger.data.sync.SyncState
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.stateIn
import kotlinx.coroutines.launch

/** Состояние экрана синхронизации. */
data class SyncScreenState(
    val items: List<OutboxItem> = emptyList(),
    val sync: SyncState = SyncState(),
    val working: Boolean = false,
    val message: String? = null,
    @StringRes val messageRes: Int? = null,
    val problem: ConnectionProblem? = null
)

/** Экран очереди: отправка, предупреждения, дубли и отклонённые проводки. */
class SyncViewModel(
    private val config: ServerConfig?,
    private val manager: SyncManager,
    private val localEntries: LocalEntryRepository
) : ViewModel() {
    private val _screen = MutableStateFlow(SyncScreenState())
    val screen: StateFlow<SyncScreenState> = _screen.asStateFlow()

    val syncState: StateFlow<SyncState> = manager.state
    val items: StateFlow<List<OutboxItem>> = localEntries.observeItems()
        .stateIn(viewModelScope, SharingStarted.WhileSubscribed(5_000), emptyList())

    init {
        viewModelScope.launch {
            items.collect { rows -> _screen.value = _screen.value.copy(items = rows) }
        }
        viewModelScope.launch {
            manager.state.collect { state ->
                _screen.value = _screen.value.copy(
                    sync = state,
                    working = state.phase == SyncPhase.SYNCING
                )
            }
        }
    }

    fun syncNow() {
        val current = config ?: return
        viewModelScope.launch {
            _screen.value = _screen.value.copy(
                working = true,
                message = null,
                messageRes = null,
                problem = null
            )
            val result = manager.sync(current)
            _screen.value = when (result) {
                is SyncOutcome.Success -> _screen.value.copy(
                    working = false,
                    message = result.warnings.firstOrNull()
                )
                is SyncOutcome.Rejected -> _screen.value.copy(
                    working = false,
                    message = if (result.message.isBlank()) {
                        result.code
                    } else {
                        "${result.code}: ${result.message}"
                    },
                    messageRes = if (result.code.isBlank() && result.message.isBlank()) {
                        R.string.sync_rejected_fallback
                    } else {
                        null
                    }
                )
                is SyncOutcome.Failure -> _screen.value.copy(
                    working = false,
                    problem = result.error.toProblem()
                )
                SyncOutcome.NothingToDo -> _screen.value.copy(working = false)
            }
        }
    }

    fun deleteDraft(uuid: String) {
        viewModelScope.launch {
            runCatching { localEntries.delete(uuid) }
                .onFailure {
                    _screen.value = _screen.value.copy(
                        messageRes = R.string.sync_delete_failed
                    )
                }
        }
    }
}
