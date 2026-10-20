package com.trefas.asledger.ui.screens

import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import com.trefas.asledger.ui.components.EmptyState
import com.trefas.asledger.ui.components.ScreenScaffold

/** Состояние синхронизации: очередь и последний обмен появятся в S12. */
@Composable
fun SyncStatusScreen(modifier: Modifier = Modifier) {
    ScreenScaffold(title = "Синхронизация", modifier = modifier) {
        EmptyState(
            title = "Обмен с сервером",
            text = "Здесь будет очередь локальных проводок, время последнего обмена " +
                "и предупреждения о расхождениях. Пока очередь пуста: данные " +
                "появятся после подключения книги в вехах S11–S12."
        )
    }
}
