package com.trefas.asledger.ui.screens

import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import com.trefas.asledger.ui.components.EmptyState
import com.trefas.asledger.ui.components.ScreenScaffold

/** План счетов доступен только для чтения: редактируется на сервере. */
@Composable
fun AccountsScreen(modifier: Modifier = Modifier) {
    ScreenScaffold(title = "План счетов", modifier = modifier) {
        EmptyState(
            title = "План счетов",
            text = "После первой синхронизации здесь появится дерево счетов с типами " +
                "и остатками. План счетов меняется на сервере, в приложении доступен " +
                "только просмотр и выбор счёта при вводе проводки.",
        )
    }
}
