package com.trefas.asledger.ui.screens

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Add
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import com.trefas.asledger.ui.components.BrandMark
import com.trefas.asledger.ui.components.ScreenScaffold

/** Журнал проводок. В S9 — пустое состояние; данные появятся после S11–S12. */
@Composable
fun JournalScreen(
    onNewEntry: () -> Unit,
    modifier: Modifier = Modifier
) {
    ScreenScaffold(
        title = "Журнал",
        modifier = modifier,
        actions = {
            IconButton(onClick = onNewEntry) {
                Icon(Icons.Filled.Add, contentDescription = "Новая проводка")
            }
        }
    ) {
        Column(
            modifier = Modifier
                .fillMaxSize()
                .padding(24.dp),
            horizontalAlignment = Alignment.CenterHorizontally,
            verticalArrangement = Arrangement.Center
        ) {
            BrandMark(size = 64.dp)
            Spacer(Modifier.height(16.dp))
            Text(
                text = "Проводок пока нет",
                style = MaterialTheme.typography.titleLarge
            )
            Spacer(Modifier.height(8.dp))
            Text(
                text = "После подключения приложение загрузит план счетов и проводки " +
                    "с сервера. Здесь появится журнал с фильтрами и возможностью " +
                    "создать новую проводку.",
                style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                textAlign = TextAlign.Center
            )
        }
    }
}
