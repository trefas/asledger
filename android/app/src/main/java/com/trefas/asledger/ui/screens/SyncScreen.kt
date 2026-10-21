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
import androidx.compose.material.icons.filled.Refresh
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.unit.dp
import com.trefas.asledger.data.model.OutboxItem
import com.trefas.asledger.data.model.SyncStatus
import com.trefas.asledger.data.sync.SyncPhase
import com.trefas.asledger.ui.components.ScreenScaffold
import com.trefas.asledger.util.formatDateTime
import com.trefas.asledger.util.formatKop

/** Очередь отправки: статусы, ошибки, предупреждения и дубли. */
@Composable
fun SyncScreen(
    state: SyncScreenState,
    onSync: () -> Unit,
    onOpen: (String) -> Unit,
    onEdit: (String) -> Unit,
    onDelete: (String) -> Unit,
    onBack: () -> Unit,
    modifier: Modifier = Modifier
) {
    ScreenScaffold(
        title = "Синхронизация",
        modifier = modifier,
        onBack = onBack,
        actions = {
            if (state.working) {
                CircularProgressIndicator(
                    modifier = Modifier
                        .padding(end = 12.dp)
                        .testTag("sync-progress")
                )
            } else {
                IconButton(onClick = onSync) {
                    Icon(Icons.Filled.Refresh, contentDescription = "Синхронизировать")
                }
            }
        }
    ) {
        LazyColumn(
            modifier = Modifier
                .fillMaxSize()
                .testTag("sync-list"),
            verticalArrangement = Arrangement.spacedBy(10.dp)
        ) {
            item {
                SyncSummary(state, onSync)
            }
            state.message?.let { message ->
                item {
                    Text(
                        text = message,
                        color = MaterialTheme.colorScheme.error,
                        style = MaterialTheme.typography.bodyMedium,
                        modifier = Modifier.testTag("sync-message")
                    )
                }
            }
            if (state.items.isEmpty()) {
                item {
                    Text(
                        text = "Очередь пуста. Все проводки отправлены.",
                        style = MaterialTheme.typography.bodyLarge,
                        modifier = Modifier.padding(vertical = 24.dp)
                    )
                }
            }
            items(state.items, key = { it.transactionUuid }) { item ->
                OutboxCard(
                    item = item,
                    onOpen = { onOpen(item.transactionUuid) },
                    onEdit = { onEdit(item.transactionUuid) },
                    onDelete = { onDelete(item.transactionUuid) }
                )
            }
            item { Spacer(Modifier.height(24.dp)) }
        }
    }
}

@Composable
private fun SyncSummary(state: SyncScreenState, onSync: () -> Unit) {
    val pending = state.items.count {
        it.status == SyncStatus.PENDING || it.status == SyncStatus.SENDING
    }
    val rejected = state.items.count { it.status == SyncStatus.REJECTED }
    Card(
        modifier = Modifier.fillMaxWidth(),
        colors = CardDefaults.cardColors(
            containerColor = when (state.sync.phase) {
                SyncPhase.OFFLINE, SyncPhase.REJECTED ->
                    MaterialTheme.colorScheme.errorContainer
                SyncPhase.SUCCESS -> MaterialTheme.colorScheme.secondaryContainer
                else -> MaterialTheme.colorScheme.surfaceVariant
            }
        )
    ) {
        Column(Modifier.padding(16.dp)) {
            Text(
                text = when (state.sync.phase) {
                    SyncPhase.SYNCING -> "Синхронизируем…"
                    SyncPhase.SUCCESS -> "Книга синхронизирована"
                    SyncPhase.OFFLINE -> "Нет связи с сервером"
                    SyncPhase.REJECTED -> "Есть отклонённые проводки"
                    SyncPhase.IDLE -> "Готово к синхронизации"
                },
                style = MaterialTheme.typography.titleMedium
            )
            Spacer(Modifier.height(4.dp))
            Text(
                text = buildString {
                    append("К отправке: $pending")
                    if (rejected > 0) append(" · отклонено: $rejected")
                    if (state.sync.lastSyncAt > 0) {
                        append(" · последняя: ")
                        append(formatDateTime(state.sync.lastSyncAt))
                    }
                },
                style = MaterialTheme.typography.bodySmall
            )
            Spacer(Modifier.height(10.dp))
            Button(onClick = onSync, enabled = !state.working) {
                Text(if (state.working) "Отправляем…" else "Отправить и обновить")
            }
        }
    }
}

@Composable
private fun OutboxCard(
    item: OutboxItem,
    onOpen: () -> Unit,
    onEdit: () -> Unit,
    onDelete: () -> Unit
) {
    Card(
        modifier = Modifier
            .fillMaxWidth()
            .testTag("outbox-${item.transactionUuid}")
    ) {
        Column(Modifier.padding(14.dp)) {
            Row(
                Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.SpaceBetween,
                verticalAlignment = Alignment.Top
            ) {
                Column(Modifier.weight(1f)) {
                    Text(
                        text = item.description.ifBlank { "Проводка" },
                        style = MaterialTheme.typography.titleMedium
                    )
                    Text(
                        text = formatKop(item.amountKop),
                        style = MaterialTheme.typography.bodyMedium
                    )
                }
                Text(
                    text = item.status.title,
                    style = MaterialTheme.typography.labelMedium,
                    color = statusColor(item.status)
                )
            }
            Text(
                text = "Коммит ${item.commitId} · попыток ${item.attempts}",
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                modifier = Modifier.padding(top = 4.dp)
            )
            item.errorCode?.let { code ->
                Text(
                    text = "$code: ${item.lastError.orEmpty()}",
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.error,
                    modifier = Modifier.padding(top = 6.dp)
                )
            }
            if (item.warnings.isNotEmpty()) {
                Text(
                    text = item.warnings.joinToString("\n"),
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.tertiary,
                    modifier = Modifier.padding(top = 6.dp)
                )
            }
            if (item.duplicateUuids.isNotEmpty()) {
                Text(
                    text = "Возможные дубли: ${item.duplicateUuids.joinToString()}",
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.tertiary,
                    modifier = Modifier.padding(top = 6.dp)
                )
            }
            Row(
                modifier = Modifier
                    .fillMaxWidth()
                    .padding(top = 6.dp),
                horizontalArrangement = Arrangement.End
            ) {
                if (item.status == SyncStatus.REJECTED) {
                    TextButton(onClick = onEdit) { Text("Исправить") }
                    TextButton(onClick = onDelete) { Text("Удалить") }
                } else {
                    TextButton(onClick = onOpen) { Text("Открыть") }
                }
            }
        }
    }
}

@Composable
private fun statusColor(status: SyncStatus) = when (status) {
    SyncStatus.REJECTED -> MaterialTheme.colorScheme.error
    SyncStatus.PENDING, SyncStatus.SENDING -> MaterialTheme.colorScheme.tertiary
    SyncStatus.SYNCED -> MaterialTheme.colorScheme.secondary
    SyncStatus.DRAFT -> MaterialTheme.colorScheme.onSurfaceVariant
}
