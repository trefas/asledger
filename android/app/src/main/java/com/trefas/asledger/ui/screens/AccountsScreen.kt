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
import androidx.compose.material.icons.filled.KeyboardArrowDown
import androidx.compose.material.icons.filled.KeyboardArrowUp
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.FilterChip
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
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
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.trefas.asledger.R
import com.trefas.asledger.data.model.Account
import com.trefas.asledger.data.model.AccountBalanceRow
import com.trefas.asledger.data.model.AccountNode
import com.trefas.asledger.data.repository.AccountRepository
import com.trefas.asledger.data.repository.BalanceRepository
import com.trefas.asledger.ui.components.EmptyState
import com.trefas.asledger.ui.components.ScreenScaffold
import com.trefas.asledger.util.formatKop

/** План счетов: дерево, поиск, раскрытие и выбор листового счёта. */
@Composable
fun AccountsScreen(
    accounts: AccountRepository,
    balances: BalanceRepository,
    onSelectAccount: (Long) -> Unit = {},
    modifier: Modifier = Modifier
) {
    var query by rememberSaveable { mutableStateOf("") }
    var includeArchived by rememberSaveable { mutableStateOf(false) }
    var collapsed by remember { mutableStateOf<Set<Long>>(emptySet()) }

    val tree by accounts.observeTree(includeArchived = includeArchived)
        .collectAsStateWithLifecycle(initialValue = emptyList())
    val search by accounts.search(query)
        .collectAsStateWithLifecycle(initialValue = emptyList())
    val report by balances.observeReport()
        .collectAsStateWithLifecycle(initialValue = null)
    val balancesById = report?.rows.orEmpty().associateBy { it.account.id }

    ScreenScaffold(title = stringResource(R.string.accounts_title), modifier = modifier) {
        Column(Modifier.fillMaxSize()) {
            Text(
                text = stringResource(R.string.accounts_description),
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                modifier = Modifier.padding(bottom = 8.dp)
            )
            OutlinedTextField(
                value = query,
                onValueChange = { query = it },
                label = { Text(stringResource(R.string.accounts_search_label)) },
                singleLine = true,
                modifier = Modifier
                    .fillMaxWidth()
                    .testTag("accounts-search")
            )
            Row(
                modifier = Modifier.padding(vertical = 6.dp),
                horizontalArrangement = Arrangement.spacedBy(8.dp)
            ) {
                FilterChip(
                    selected = includeArchived,
                    onClick = { includeArchived = !includeArchived },
                    label = { Text(stringResource(R.string.accounts_show_archived)) }
                )
            }

            val searching = query.isNotBlank()
            val rows = if (searching) {
                search.map { FlatAccount(it, 0, pathOnly = true) }
            } else {
                flatten(tree, collapsed)
            }
            if (rows.isEmpty()) {
                EmptyState(
                    title = stringResource(
                        if (searching) {
                            R.string.accounts_empty_filtered_title
                        } else {
                            R.string.accounts_empty_title
                        }
                    ),
                    text = stringResource(
                        if (searching) {
                            R.string.accounts_empty_filtered_text
                        } else {
                            R.string.accounts_empty_text
                        }
                    )
                )
            } else {
                LazyColumn(
                    modifier = Modifier
                        .fillMaxSize()
                        .testTag("accounts-tree"),
                    verticalArrangement = Arrangement.spacedBy(6.dp)
                ) {
                    items(rows, key = { it.account.id }) { row ->
                        AccountRow(
                            account = row.account,
                            depth = row.depth,
                            balance = balancesById[row.account.id],
                            hasChildren = row.hasChildren,
                            expanded = !collapsed.contains(row.account.id),
                            pathOnly = row.pathOnly,
                            onClick = {
                                if (row.account.isLeaf) {
                                    onSelectAccount(row.account.id)
                                } else {
                                    collapsed = if (collapsed.contains(row.account.id)) {
                                        collapsed - row.account.id
                                    } else {
                                        collapsed + row.account.id
                                    }
                                }
                            }
                        )
                    }
                    item { Spacer(Modifier.height(16.dp)) }
                }
            }
        }
    }
}

@Composable
private fun AccountRow(
    account: Account,
    depth: Int,
    balance: AccountBalanceRow?,
    hasChildren: Boolean,
    expanded: Boolean,
    pathOnly: Boolean,
    onClick: () -> Unit
) {
    Card(
        onClick = onClick,
        modifier = Modifier
            .fillMaxWidth()
            .padding(start = (depth * 14).dp)
            .testTag("account-${account.id}"),
        colors = CardDefaults.cardColors(
            containerColor = if (account.isLeaf) {
                MaterialTheme.colorScheme.surface
            } else {
                MaterialTheme.colorScheme.surfaceVariant
            }
        )
    ) {
        Row(
            modifier = Modifier
                .fillMaxWidth()
                .padding(horizontal = 12.dp, vertical = 10.dp),
            horizontalArrangement = Arrangement.SpaceBetween,
            verticalAlignment = Alignment.CenterVertically
        ) {
            Row(
                modifier = Modifier.weight(1f),
                verticalAlignment = Alignment.CenterVertically
            ) {
                if (hasChildren) {
                    Icon(
                        imageVector = if (expanded) {
                            Icons.Filled.KeyboardArrowUp
                        } else {
                            Icons.Filled.KeyboardArrowDown
                        },
                        contentDescription = stringResource(
                            if (expanded) R.string.accounts_collapse else R.string.accounts_expand
                        )
                    )
                }
                Column(Modifier.padding(start = if (hasChildren) 6.dp else 0.dp)) {
                    Text(
                        text = if (pathOnly) account.fullPath else account.name,
                        style = if (account.isLeaf) {
                            MaterialTheme.typography.bodyLarge
                        } else {
                            MaterialTheme.typography.titleSmall
                        },
                        color = if (account.isArchived) {
                            MaterialTheme.colorScheme.onSurfaceVariant
                        } else {
                            MaterialTheme.colorScheme.onSurface
                        },
                        maxLines = 1,
                        overflow = TextOverflow.Ellipsis
                    )
                    val typeTitle = stringResource(account.type.titleRes)
                    val archivedSuffix = if (account.isArchived) {
                        stringResource(R.string.accounts_archived_suffix)
                    } else {
                        ""
                    }
                    val leafSuffix = if (account.isLeaf) {
                        stringResource(R.string.accounts_leaf_suffix)
                    } else {
                        ""
                    }
                    Text(
                        text = typeTitle + archivedSuffix + leafSuffix,
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant
                    )
                }
            }
            Column(
                modifier = Modifier.padding(start = 12.dp),
                horizontalAlignment = Alignment.End
            ) {
                Text(
                    text = formatKop(balance?.totalKop ?: 0),
                    style = MaterialTheme.typography.bodyLarge,
                    maxLines = 1
                )
                val pending = balance?.pendingTotalKop ?: 0
                if (pending != 0L) {
                    Text(
                        text = stringResource(R.string.accounts_pending, formatKop(pending)),
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.tertiary
                    )
                }
            }
        }
    }
}

private data class FlatAccount(
    val account: Account,
    val depth: Int,
    val pathOnly: Boolean = false,
    val hasChildren: Boolean = false
)

private fun flatten(
    nodes: List<AccountNode>,
    collapsed: Set<Long>,
    depth: Int = 0
): List<FlatAccount> = nodes.flatMap { node ->
    val current = FlatAccount(
        account = node.account,
        depth = depth,
        hasChildren = node.children.isNotEmpty()
    )
    if (collapsed.contains(node.account.id)) {
        listOf(current)
    } else {
        listOf(current) + flatten(node.children, collapsed, depth + 1)
    }
}
