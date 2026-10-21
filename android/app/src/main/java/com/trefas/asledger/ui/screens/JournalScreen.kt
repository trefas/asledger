package com.trefas.asledger.ui.screens

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Add
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.trefas.asledger.data.model.TransactionSummary
import com.trefas.asledger.data.repository.TransactionRepository
import com.trefas.asledger.ui.components.BrandMark
import com.trefas.asledger.ui.components.ScreenScaffold
import com.trefas.asledger.util.formatDate
import com.trefas.asledger.util.formatKop

/** Журнал проводок из локальной реплики. */
@Composable
fun JournalScreen(
    onNewEntry: () -> Unit,
    transactions: TransactionRepository,
    userName: String = "",
    modifier: Modifier = Modifier
) {
    val items by transactions
        .observePage()
        .collectAsStateWithLifecycle(initialValue = emptyList())

    ScreenScaffold(
        title = "Журнал",
        modifier = modifier,
        actions = {
            IconButton(onClick = onNewEntry) {
                Icon(Icons.Filled.Add, contentDescription = "Новая проводка")
            }
        }
    ) {
        if (items.isEmpty()) {
            EmptyJournal(userName)
        } else {
            LazyColumn(
                modifier = Modifier
                    .fillMaxSize()
                    .testTag("journal-list"),
                verticalArrangement = Arrangement.spacedBy(8.dp)
            ) {
                items(items, key = { it.uuid }) { transaction ->
                    JournalRow(transaction)
                }
                item { Spacer(Modifier.height(16.dp)) }
            }
        }
    }
}

@Composable
private fun EmptyJournal(userName: String) {
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
                "создать новую проводку." +
                if (userName.isNotBlank()) "\n\nТекущий пользователь: $userName" else "",
            style = MaterialTheme.typography.bodyMedium,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
            textAlign = TextAlign.Center
        )
    }
}

@Composable
private fun JournalRow(transaction: TransactionSummary) {
    Card(
        modifier = Modifier.fillMaxWidth(),
        colors = CardDefaults.cardColors(
            containerColor = MaterialTheme.colorScheme.surfaceVariant
        )
    ) {
        Column(modifier = Modifier.padding(16.dp)) {
            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.SpaceBetween,
                verticalAlignment = Alignment.Top
            ) {
                Text(
                    text = transaction.description.ifBlank { transaction.opType },
                    style = MaterialTheme.typography.titleMedium,
                    modifier = Modifier.weight(1f)
                )
                Text(
                    text = formatKop(transaction.amountKop),
                    style = MaterialTheme.typography.titleMedium,
                    color = MaterialTheme.colorScheme.primary
                )
            }
            Spacer(Modifier.height(6.dp))
            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.SpaceBetween
            ) {
                Text(
                    text = buildString {
                        append(formatDate(transaction.date))
                        if (transaction.actorName.isNotBlank()) {
                            append(" · ")
                            append(transaction.actorName)
                        }
                    },
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant
                )
                if (transaction.pending) {
                    Text(
                        text = "Не отправлено",
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.tertiary
                    )
                }
            }
        }
    }
}
