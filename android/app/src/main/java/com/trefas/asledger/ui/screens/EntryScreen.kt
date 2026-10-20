package com.trefas.asledger.ui.screens

import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import com.trefas.asledger.ui.components.EmptyState
import com.trefas.asledger.ui.components.ScreenScaffold

/** Ввод проводки: строки дебета и кредита появятся в вехе S13. */
@Composable
fun EntryScreen(
    userName: String = "",
    modifier: Modifier = Modifier
) {
    ScreenScaffold(title = "Новая проводка", modifier = modifier) {
        EmptyState(
            title = "Ввод проводки",
            text = (if (userName.isNotBlank()) "Автор проводки: $userName.\n\n" else "") +
                "Форма с ролями, суммами и подсказками по плану счетов появится " +
                "после загрузки книги. Стороны дебета и кредита назначит движок — " +
                "дублировать матрицу в приложении не нужно."
        )
    }
}
