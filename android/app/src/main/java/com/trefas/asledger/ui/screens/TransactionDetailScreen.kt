package com.trefas.asledger.ui.screens

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.trefas.asledger.data.model.Account
import com.trefas.asledger.data.model.SyncStatus
import com.trefas.asledger.data.model.TransactionDetails
import com.trefas.asledger.data.model.TransactionLine
import com.trefas.asledger.data.repository.AccountRepository
import com.trefas.asledger.data.repository.TransactionRepository
import com.trefas.asledger.ui.components.ScreenScaffold
import com.trefas.asledger.util.formatDateTime
import com.trefas.asledger.util.formatKop

/** Детали проводки, статус отправки, сторно и повтор. */
@Composable
fun TransactionDetailScreen(
    uuid: String,
    transactions: TransactionRepository,
    accountsRepository: AccountRepository,
    onBack: () -> Unit,
    onEdit: () -> Unit,
    onRepeat: () -> Unit,
    onDelete: () -> Unit,
    onReverse: () -> Unit,
    onSync: () -> Unit,
    onOpenTransaction: (String) -> Unit,
    modifier: Modifier = Modifier
) {
    val details by transactions.observeDetails(uuid)
        .collectAsStateWithLifecycle(initialValue = null)
    val accounts by accountsRepository.observeAccounts(includeArchived = true)
        .collectAsStateWithLifecycle(initialValue = emptyList())
    var confirmReverse by remember { mutableStateOf(false) }
    var confirmDelete by remember { mutableStateOf(false) }
    val transaction = details

    if (transaction == null) {
        ScreenScaffold(title = "Проводка", modifier = modifier, onBack = onBack) {
            Text("Проводка не найдена", modifier = Modifier.padding(16.dp))
        }
        return
    }

    ScreenScaffold(title = "Проводка", modifier = modifier, onBack = onBack) {
        Column(
            modifier = Modifier
                .fillMaxSize()
                .verticalScroll(rememberScrollState())
                .padding(bottom = 24.dp),
            verticalArrangement = Arrangement.spacedBy(12.dp)
        ) {
            DetailHeader(transaction)
            transaction.errorCode?.let { code ->
                Card(
                    colors = CardDefaults.cardColors(
                        containerColor = MaterialTheme.colorScheme.errorContainer
                    )
                ) {
                    Text(
                        text = "$code: ${transaction.lastError.orEmpty()}",
                        modifier = Modifier.padding(12.dp),
                        color = MaterialTheme.colorScheme.onErrorContainer
                    )
                }
            }
            if (transaction.warnings.isNotEmpty()) {
                Text(
                    text = transaction.warnings.joinToString("\n"),
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.tertiary
                )
            }
            if (transaction.duplicateUuids.isNotEmpty()) {
                Text(
                    text = "Возможные дубли",
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.tertiary
                )
                transaction.duplicateUuids.forEach { duplicate ->
                    TextButton(
                        onClick = { onOpenTransaction(duplicate) },
                        modifier = Modifier.testTag("duplicate-$duplicate")
                    ) {
                        Text("Открыть существующую проводку")
                    }
                }
            }

            LinesBlock(
                title = "Дебет",
                lines = transaction.lines.filter { it.side == "DEBIT" },
                accounts = accounts
            )
            LinesBlock(
                title = "Кредит",
                lines = transaction.lines.filter { it.side == "CREDIT" },
                accounts = accounts
            )

            if (transaction.syncStatus == SyncStatus.PENDING ||
                transaction.syncStatus == SyncStatus.SENDING ||
                transaction.syncStatus == SyncStatus.REJECTED
            ) {
                Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    Button(
                        onClick = onEdit,
                        modifier = Modifier
                            .weight(1f)
                            .testTag("detail-edit")
                    ) { Text("Исправить") }
                    OutlinedButton(
                        onClick = onSync,
                        modifier = Modifier.weight(1f)
                    ) { Text("Отправить") }
                }
                TextButton(
                    onClick = { confirmDelete = true },
                    modifier = Modifier.testTag("detail-delete")
                ) { Text("Удалить черновик") }
            } else {
                Button(
                    onClick = onRepeat,
                    modifier = Modifier
                        .fillMaxWidth()
                        .testTag("detail-repeat")
                ) { Text("Повторить проводку") }
                OutlinedButton(
                    onClick = { confirmReverse = true },
                    modifier = Modifier
                        .fillMaxWidth()
                        .testTag("detail-reverse")
                ) { Text("Сторно") }
            }
        }
    }

    if (confirmReverse) {
        AlertDialog(
            onDismissRequest = { confirmReverse = false },
            title = { Text("Создать сторно?") },
            text = {
                Text(
                    "Сервер создаст обратную проводку на ту же сумму. " +
                        "Исходная проводка останется в журнале."
                )
            },
            confirmButton = {
                TextButton(
                    onClick = {
                        confirmReverse = false
                        onReverse()
                    }
                ) { Text("Создать сторно") }
            },
            dismissButton = {
                TextButton(onClick = { confirmReverse = false }) { Text("Отмена") }
            }
        )
    }
    if (confirmDelete) {
        AlertDialog(
            onDismissRequest = { confirmDelete = false },
            title = { Text("Удалить черновик?") },
            text = { Text("Проводка и её строки будут удалены с устройства.") },
            confirmButton = {
                TextButton(
                    onClick = {
                        confirmDelete = false
                        onDelete()
                    }
                ) { Text("Удалить") }
            },
            dismissButton = {
                TextButton(onClick = { confirmDelete = false }) { Text("Отмена") }
            }
        )
    }
}

@Composable
private fun DetailHeader(transaction: TransactionDetails) {
    Card(Modifier.fillMaxWidth()) {
        Column(Modifier.padding(16.dp)) {
            Text(
                text = transaction.description.ifBlank {
                    transaction.opType.replaceFirstChar { it.uppercase() }
                },
                style = MaterialTheme.typography.titleLarge
            )
            Spacer(Modifier.height(4.dp))
            Text(
                text = "${formatDateTime(transaction.date)} · ${transaction.opType}",
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant
            )
            Spacer(Modifier.height(8.dp))
            Text(
                text = transaction.syncStatus.title,
                style = MaterialTheme.typography.labelLarge,
                color = when (transaction.syncStatus) {
                    SyncStatus.REJECTED -> MaterialTheme.colorScheme.error
                    SyncStatus.PENDING, SyncStatus.SENDING ->
                        MaterialTheme.colorScheme.tertiary
                    else -> MaterialTheme.colorScheme.secondary
                }
            )
        }
    }
}

@Composable
private fun LinesBlock(
    title: String,
    lines: List<TransactionLine>,
    accounts: List<Account>
) {
    if (lines.isEmpty()) return
    Text(title, style = MaterialTheme.typography.titleMedium)
    lines.forEach { line ->
        val account = accounts.firstOrNull { it.id == line.accountId }
        Card(
            modifier = Modifier.fillMaxWidth(),
            colors = CardDefaults.cardColors(
                containerColor = MaterialTheme.colorScheme.surfaceVariant
            )
        ) {
            Row(
                modifier = Modifier
                    .fillMaxWidth()
                    .padding(12.dp),
                horizontalArrangement = Arrangement.SpaceBetween
            ) {
                Column(Modifier.weight(1f)) {
                    Text(
                        text = account?.name ?: "Счёт ${line.accountId}",
                        style = MaterialTheme.typography.bodyLarge
                    )
                    Text(
                        text = account?.fullPath.orEmpty(),
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant
                    )
                }
                Text(
                    text = formatKop(line.amountKop),
                    style = MaterialTheme.typography.titleSmall
                )
            }
        }
    }
}
