package com.trefas.asledger.ui.screens

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.trefas.asledger.data.network.ConnectionError
import com.trefas.asledger.data.network.ConnectionProblem
import com.trefas.asledger.data.network.toProblem
import com.trefas.asledger.data.sync.LibraryLoadResult
import com.trefas.asledger.data.sync.LibraryProgress
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.Job
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.launch

/** Состояние экрана первичной загрузки. */
sealed interface LoadingState {
    data object Starting : LoadingState
    data class Running(val progress: LibraryProgress) : LoadingState
    data object Success : LoadingState
    data class Failure(
        val error: ConnectionError,
        val problem: ConnectionProblem
    ) : LoadingState

    data object Cancelled : LoadingState
}

/**
 * Управляет первичной загрузкой книги.
 *
 * Загрузчик передаётся лямбдой, поэтому ViewModel не знает про NavController
 * и легко проверяется с поддельным источником.
 */
class LoadingViewModel(
    private val load: suspend (onProgress: (LibraryProgress) -> Unit) -> LibraryLoadResult
) : ViewModel() {
    private val _state = MutableStateFlow<LoadingState>(LoadingState.Starting)
    val state: StateFlow<LoadingState> = _state.asStateFlow()

    private var job: Job? = null

    /** Запускает загрузку; повторный вызов во время работы игнорируется. */
    fun start() {
        if (job?.isActive == true || _state.value is LoadingState.Success) {
            return
        }
        job = viewModelScope.launch {
            _state.value = LoadingState.Running(LibraryProgress("Готовимся", 0))
            try {
                val result = load { progress ->
                    _state.value = LoadingState.Running(progress)
                }
                when (result) {
                    LibraryLoadResult.Success -> {
                        _state.value = LoadingState.Success
                    }

                    is LibraryLoadResult.Failure -> {
                        _state.value = LoadingState.Failure(
                            error = result.error,
                            problem = result.error.toProblem()
                        )
                    }
                }
            } catch (cancelled: CancellationException) {
                throw cancelled
            }
        }
    }

    /** Повторяет загрузку после ошибки. */
    fun retry() {
        if (job?.isActive == true) return
        _state.value = LoadingState.Starting
        start()
    }

    /** Отменяет загрузку, не меняя сохранённые адрес и сессию. */
    fun cancel() {
        job?.cancel()
        _state.value = LoadingState.Cancelled
    }
}
