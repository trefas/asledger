package com.trefas.asledger.ui.screens

import androidx.annotation.StringRes
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.FlowRow
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
import androidx.compose.material.icons.filled.Warning
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FilterChip
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.material3.pulltorefresh.PullToRefreshBox
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.trefas.asledger.R
import com.trefas.asledger.data.model.JournalFilter
import com.trefas.asledger.data.model.SyncStatus
import com.trefas.asledger.data.model.TransactionSummary
import com.trefas.asledger.data.repository.AccountRepository
import com.trefas.asledger.data.repository.LocalEntryRepository
import com.trefas.asledger.data.repository.MOBILE_OP_TYPES
import com.trefas.asledger.data.repository.TransactionRepository
import com.trefas.asledger.data.sync.SyncManager
import com.trefas.asledger.data.sync.SyncPhase
import com.trefas.asledger.ui.components.BrandMark
import com.trefas.asledger.ui.components.FirstOpenHint
import com.trefas.asledger.ui.components.HintIcon
import com.trefas.asledger.ui.components.ScreenScaffold
import com.trefas.asledger.ui.components.operationTypeLabel
import com.trefas.asledger.util.FriendlyDate
import com.trefas.asledger.util.formatKop
import com.trefas.asledger.util.friendlyDate
import java.time.LocalDate
import java.time.ZoneId
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.map

/** Период журнала. */
private enum class JournalPeriod(@StringRes val titleRes: Int) {
    ALL(R.string.journal_period_all),
    MONTH(R.string.journal_period_month),
    WEEK(R.string.journal_period_week),
    TODAY(R.string.journal_period_today)
}

/** Журнал проводок с поиском, фильтрами и статусом синхронизации. */
@OptIn(ExperimentalMaterial3Api::class, ExperimentalLayoutApi::class)
@Composable
fun JournalScreen(
    onNewEntry: () -> Unit,
    onOpenTransaction: (String) -> Unit,
    onOpenSync: () -> Unit,
    onRefresh: () -> Unit,
    transactions: TransactionRepository,
    accountsRepository: AccountRepository,
    localEntries: LocalEntryRepository,
    syncManager: SyncManager,
    userName: String = "",
    modifier: Modifier = Modifier
) {
    var query by rememberSaveable { mutableStateOf("") }
    var period by rememberSaveable { mutableStateOf(JournalPeriod.ALL) }
    var status by rememberSaveable { mutableStateOf<SyncStatus?>(null) }
    var opType by rememberSaveable { mutableStateOf<String?>(null) }
    var accountId by rememberSaveable { mutableStateOf<Long?>(null) }

    val accounts by accountsRepository.observeAccounts()
        .collectAsStateWithLifecycle(initialValue = emptyList())
    val syncState by syncManager.state.collectAsStateWithLifecycle()
    val outbox by localEntries.observeItems()
        .collectAsStateWithLifecycle(initialValue = emptyList())
    val pendingCount = outbox.count {
        it.status == SyncStatus.PENDING || it.status == SyncStatus.SENDING
    }
    val rejectedCount = outbox.count { it.status == SyncStatus.REJECTED }

    val filter = remember(query, period, status, opType, accountId) {
        val (from, to) = periodBounds(period)
        JournalFilter(
            query = query,
            fromDate = from,
            toDate = to,
            accountId = accountId,
            opType = opType,
            syncStatus = status
        )
    }
    val pageFlow: Flow<List<TransactionSummary>> = remember(filter) {
        transactions.observePage(filter)
    }
    val items by pageFlow.collectAsStateWithLifecycle(initialValue = emptyList())

    FirstOpenHint(
        id = "journal",
        title = stringResource(R.string.hint_title_journal),
        text = stringResource(R.string.hint_journal)
    )

    ScreenScaffold(
        title = stringResource(R.string.journal_title),
        modifier = modifier,
        actions = {
            HintIcon(text = stringResource(R.string.hint_status))
            IconButton(onClick = onNewEntry) {
                Icon(Icons.Filled.Add, contentDescription = stringResource(R.string.journal_new_entry))
            }
        }
    ) {
        PullToRefreshBox(
            isRefreshing = syncState.phase == SyncPhase.SYNCING,
            onRefresh = onRefresh,
            modifier = Modifier.fillMaxSize()
        ) {
            Column(Modifier.fillMaxSize()) {
                if (pendingCount > 0 ||
                    rejectedCount > 0 ||
                    syncState.phase == SyncPhase.OFFLINE
                ) {
                    SyncBanner(
                        pending = pendingCount,
                        rejected = rejectedCount,
                        offline = syncState.phase == SyncPhase.OFFLINE,
                        onClick = onOpenSync
                    )
                    Spacer(Modifier.height(8.dp))
                }
                OutlinedTextField(
                    value = query,
                    onValueChange = { query = it },
                    label = { Text(stringResource(R.string.journal_search_label)) },
                    singleLine = true,
                    modifier = Modifier
                        .fillMaxWidth()
                        .testTag("journal-search")
                )
                FlowRow(
                    horizontalArrangement = Arrangement.spacedBy(6.dp),
                    verticalArrangement = Arrangement.spacedBy(4.dp),
                    modifier = Modifier.padding(vertical = 4.dp)
                ) {
                    FilterChip(
                        selected = status == null,
                        onClick = { status = null },
                        label = { Text(stringResource(R.string.journal_filter_all)) }
                    )
                    FilterChip(
                        selected = status == SyncStatus.PENDING,
                        onClick = { status = SyncStatus.PENDING },
                        label = { Text(stringResource(R.string.journal_filter_pending)) }
                    )
                    FilterChip(
                        selected = status == SyncStatus.REJECTED,
                        onClick = { status = SyncStatus.REJECTED },
                        label = { Text(stringResource(R.string.journal_filter_rejected)) }
                    )
                    JournalPeriod.entries.forEach { candidate ->
                        FilterChip(
                            selected = period == candidate,
                            onClick = { period = candidate },
                            label = { Text(stringResource(candidate.titleRes)) }
                        )
                    }
                }
                Row(
                    modifier = Modifier
                        .fillMaxWidth()
                        .padding(bottom = 6.dp),
                    horizontalArrangement = Arrangement.spacedBy(8.dp)
                ) {
                    FilterMenuText(
                        label = opType?.let { operationTypeLabel(it) }
                            ?: stringResource(R.string.journal_filter_type),
                        options = listOf(null to stringResource(R.string.journal_filter_all_types)) +
                            MOBILE_OP_TYPES.map { it to operationTypeLabel(it) },
                        onSelect = { opType = it }
                    )
                    FilterMenuLong(
                        label = accounts.firstOrNull { it.id == accountId }?.name
                            ?: stringResource(R.string.journal_filter_account),
                        options = listOf(null to stringResource(R.string.journal_filter_all_accounts)) +
                            accounts.map { it.id to it.name },
                        onSelect = { accountId = it }
                    )
                    if (query.isNotBlank() ||
                        status != null ||
                        opType != null ||
                        accountId != null ||
                        period != JournalPeriod.ALL
                    ) {
                        OutlinedButton(
                            onClick = {
                                query = ""
                                status = null
                                opType = null
                                accountId = null
                                period = JournalPeriod.ALL
                            }
                        ) { Text(stringResource(R.string.journal_reset)) }
                    }
                }

                if (items.isEmpty()) {
                    EmptyJournal(userName, query.isNotBlank())
                } else {
                    LazyColumn(
                        modifier = Modifier
                            .fillMaxSize()
                            .testTag("journal-list"),
                        verticalArrangement = Arrangement.spacedBy(8.dp)
                    ) {
                        items(items, key = { it.uuid }) { transaction ->
                            JournalRow(
                                transaction = transaction,
                                onClick = { onOpenTransaction(transaction.uuid) }
                            )
                        }
                        item { Spacer(Modifier.height(16.dp)) }
                    }
                }
            }
        }
    }
}

@Composable
private fun FilterMenuLong(
    label: String,
    options: List<Pair<Long?, String>>,
    onSelect: (Long?) -> Unit
) {
    var expanded by remember { mutableStateOf(false) }
    Box {
        OutlinedButton(onClick = { expanded = true }) { Text(label, maxLines = 1) }
        DropdownMenu(expanded = expanded, onDismissRequest = { expanded = false }) {
            options.forEach { (value, title) ->
                DropdownMenuItem(
                    text = { Text(title) },
                    onClick = {
                        expanded = false
                        onSelect(value)
                    }
                )
            }
        }
    }
}

@Composable
private fun FilterMenuText(
    label: String,
    options: List<Pair<String?, String>>,
    onSelect: (String?) -> Unit
) {
    var expanded by remember { mutableStateOf(false) }
    Box {
        OutlinedButton(onClick = { expanded = true }) { Text(label, maxLines = 1) }
        DropdownMenu(expanded = expanded, onDismissRequest = { expanded = false }) {
            options.forEach { (value, title) ->
                DropdownMenuItem(
                    text = { Text(title) },
                    onClick = {
                        expanded = false
                        onSelect(value)
                    }
                )
            }
        }
    }
}

@Composable
private fun SyncBanner(
    pending: Int,
    rejected: Int,
    offline: Boolean,
    onClick: () -> Unit
) {
    Card(
        onClick = onClick,
        modifier = Modifier
            .fillMaxWidth()
            .testTag("sync-banner"),
        colors = CardDefaults.cardColors(
            containerColor = if (offline || rejected > 0) {
                MaterialTheme.colorScheme.errorContainer
            } else {
                MaterialTheme.colorScheme.tertiaryContainer
            }
        )
    ) {
        Row(
            modifier = Modifier.padding(12.dp),
            verticalAlignment = Alignment.CenterVertically
        ) {
            Icon(Icons.Filled.Warning, contentDescription = null)
            Column(Modifier.padding(start = 10.dp)) {
                Text(
                    text = when {
                        offline -> stringResource(R.string.journal_sync_offline)
                        rejected > 0 -> stringResource(R.string.journal_sync_rejected, rejected)
                        else -> stringResource(R.string.journal_sync_pending, pending)
                    },
                    style = MaterialTheme.typography.titleSmall
                )
                Text(
                    text = stringResource(R.string.journal_sync_open),
                    style = MaterialTheme.typography.bodySmall
                )
            }
        }
    }
}

@Composable
private fun EmptyJournal(userName: String, filtered: Boolean) {
    Column(
        modifier = Modifier
            .fillMaxSize()
            .padding(24.dp),
        horizontalAlignment = Alignment.CenterHorizontally,
        verticalArrangement = Arrangement.Center
    ) {
        if (!filtered) {
            BrandMark(size = 64.dp)
            Spacer(Modifier.height(16.dp))
        }
        Text(
            text = stringResource(
                if (filtered) R.string.journal_empty_filtered_title else R.string.journal_empty_title
            ),
            style = MaterialTheme.typography.titleLarge
        )
        Spacer(Modifier.height(8.dp))
        Text(
            text = when {
                filtered -> stringResource(R.string.journal_empty_filtered_text)
                userName.isNotBlank() -> stringResource(
                    R.string.journal_empty_text_with_user,
                    stringResource(R.string.journal_empty_text),
                    userName
                )
                else -> stringResource(R.string.journal_empty_text)
            },
            style = MaterialTheme.typography.bodyMedium,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
            textAlign = TextAlign.Center
        )
    }
}

@Composable
private fun JournalRow(
    transaction: TransactionSummary,
    onClick: () -> Unit
) {
    Card(
        onClick = onClick,
        modifier = Modifier
            .fillMaxWidth()
            .testTag("journal-row-${transaction.uuid}"),
        colors = CardDefaults.cardColors(
            containerColor = MaterialTheme.colorScheme.surfaceVariant
        )
    ) {
        Column(modifier = Modifier.padding(14.dp)) {
            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.SpaceBetween,
                verticalAlignment = Alignment.Top
            ) {
                val fallbackTitle = operationTypeLabel(transaction.opType)
                Text(
                    text = transaction.description.ifBlank { fallbackTitle },
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
                val dateText = when (val value = friendlyDate(transaction.date)) {
                    FriendlyDate.Today -> stringResource(R.string.date_today)
                    FriendlyDate.Yesterday -> stringResource(R.string.date_yesterday)
                    is FriendlyDate.Absolute -> value.text
                }
                Text(
                    text = if (transaction.actorName.isNotBlank()) {
                        stringResource(
                            R.string.journal_date_actor,
                            dateText,
                            transaction.actorName
                        )
                    } else {
                        dateText
                    },
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant
                )
                if (transaction.syncStatus != SyncStatus.SYNCED) {
                    Text(
                        text = stringResource(transaction.syncStatus.titleRes),
                        style = MaterialTheme.typography.bodySmall,
                        color = when (transaction.syncStatus) {
                            SyncStatus.REJECTED -> MaterialTheme.colorScheme.error
                            else -> MaterialTheme.colorScheme.tertiary
                        }
                    )
                }
            }
        }
    }
}

private fun periodBounds(period: JournalPeriod): Pair<Long?, Long?> {
    val now = java.time.ZonedDateTime.now()
    val start = when (period) {
        JournalPeriod.ALL -> return null to null
        JournalPeriod.TODAY -> LocalDate.now()
        JournalPeriod.WEEK -> LocalDate.now().minusDays(6)
        JournalPeriod.MONTH -> LocalDate.now().withDayOfMonth(1)
    }
    val from = start.atStartOfDay(ZoneId.systemDefault()).toInstant().toEpochMilli()
    val to = now.toLocalDate().plusDays(1).atStartOfDay(ZoneId.systemDefault())
        .toInstant().toEpochMilli() - 1
    return from to to
}
