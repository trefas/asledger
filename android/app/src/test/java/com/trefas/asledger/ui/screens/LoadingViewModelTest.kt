package com.trefas.asledger.ui.screens

import com.trefas.asledger.R
import com.trefas.asledger.data.network.ConnectionError
import com.trefas.asledger.data.sync.LibraryLoadResult
import com.trefas.asledger.data.sync.LibraryProgress
import com.trefas.asledger.ui.MainDispatcherRule
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.awaitCancellation
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test

/** Состояния загрузки книги: прогресс, ошибка, отмена и повтор. */
class LoadingViewModelTest {
    @get:Rule
    val mainDispatcher = MainDispatcherRule()

    @Test
    fun `прогресс доходит до успеха`() = runTest {
        val gate = CompletableDeferred<Unit>()
        val viewModel = LoadingViewModel { onProgress ->
            onProgress(LibraryProgress(R.string.progress_transactions, 60))
            gate.await()
            LibraryLoadResult.Success
        }

        viewModel.start()

        val running = viewModel.state.value as LoadingState.Running
        assertEquals(60, running.progress.percent)
        gate.complete(Unit)
        assertEquals(LoadingState.Success, viewModel.state.value)
    }

    @Test
    fun `ошибка объясняется и повторяет загрузку`() = runTest {
        var attempt = 0
        val viewModel = LoadingViewModel {
            attempt += 1
            if (attempt == 1) {
                LibraryLoadResult.Failure(ConnectionError.ConnectionRefused)
            } else {
                LibraryLoadResult.Success
            }
        }

        viewModel.start()
        val failure = viewModel.state.value as LoadingState.Failure
        assertEquals(ConnectionError.ConnectionRefused, failure.error)
        assertEquals(R.string.connection_refused_title, failure.problem.titleRes)

        viewModel.retry()
        assertEquals(LoadingState.Success, viewModel.state.value)
        assertEquals(2, attempt)
    }

    @Test
    fun `отмена не выдаёт успех и позволяет продолжить`() = runTest {
        val viewModel = LoadingViewModel { awaitCancellation() }

        viewModel.start()
        assertTrue(viewModel.state.value is LoadingState.Running)
        viewModel.cancel()

        assertEquals(LoadingState.Cancelled, viewModel.state.value)
    }

    @Test
    fun `401 требует повторной привязки`() = runTest {
        val viewModel = LoadingViewModel {
            LibraryLoadResult.Failure(ConnectionError.Unauthorized)
        }

        viewModel.start()

        val failure = viewModel.state.value as LoadingState.Failure
        assertEquals(ConnectionError.Unauthorized, failure.error)
        assertEquals(R.string.connection_unauthorized_title, failure.problem.titleRes)
    }
}
