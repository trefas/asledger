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
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.trefas.asledger.data.model.AccountBalanceRow
import com.trefas.asledger.data.model.AccountNode
import com.trefas.asledger.data.repository.AccountRepository
import com.trefas.asledger.data.repository.BalanceRepository
import com.trefas.asledger.ui.components.EmptyState
import com.trefas.asledger.ui.components.ScreenScaffold
import com.trefas.asledger.util.formatKop

/** План счетов доступен только для чтения: редактируется на сервере. */
@Composable
fun AccountsScreen(
    accounts: AccountRepository,
    balances: BalanceRepository,
    modifier: Modifier = Modifier
) {
    val tree by accounts.observeTree()
        .collectAsStateWithLifecycle(initialValue = emptyList())
    val report by balances.observeReport()
        .collectAsStateWithLifecycle(initialValue = null)
    val rows = flatten(tree)
    val balancesById = report?.rows.orEmpty().associateBy { it.account.id }

    ScreenScaffold(title = "План счетов", modifier = modifier) {
        if (rows.isEmpty()) {
            EmptyState(
                title = "План счетов",
                text = "После первой синхронизации здесь появится дерево счетов с типами " +
                    "и остатками. План счетов меняется на сервере, в приложении доступен " +
                    "только просмотр и выбор счёта при вводе проводки."
            )
        } else {
            LazyColumn(
                modifier = Modifier
                    .fillMaxSize()
                    .testTag("accounts-tree"),
                verticalArrangement = Arrangement.spacedBy(6.dp)
            ) {
                items(rows, key = { it.node.account.id }) { row ->
                    AccountRow(
                        node = row.node,
                        depth = row.depth,
                        balance = balancesById[row.node.account.id]
                    )
                }
                item { Spacer(Modifier.height(16.dp)) }
            }
        }
    }
}

@Composable
private fun AccountRow(
    node: AccountNode,
    depth: Int,
    balance: AccountBalanceRow?
) {
    val account = node.account
    Card(
        modifier = Modifier
            .fillMaxWidth()
            .padding(start = (depth * 16).dp),
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
            Column(modifier = Modifier.weight(1f)) {
                Text(
                    text = account.name,
                    style = if (account.isLeaf) {
                        MaterialTheme.typography.bodyLarge
                    } else {
                        MaterialTheme.typography.titleSmall
                    },
                    color = if (account.isArchived) {
                        MaterialTheme.colorScheme.onSurfaceVariant
                    } else {
                        MaterialTheme.colorScheme.onSurface
                    }
                )
                Text(
                    text = account.type.title + if (account.isArchived) " · архив" else "",
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant
                )
            }
            Column(horizontalAlignment = Alignment.End) {
                Text(
                    text = formatKop(balance?.totalKop ?: 0),
                    style = MaterialTheme.typography.bodyLarge
                )
                val pending = balance?.pendingTotalKop ?: 0
                if (pending != 0L) {
                    Text(
                        text = "ожидает ${formatKop(pending)}",
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.tertiary
                    )
                }
            }
        }
    }
}

private data class FlatAccount(
    val node: AccountNode,
    val depth: Int
)

private fun flatten(nodes: List<AccountNode>, depth: Int = 0): List<FlatAccount> =
    nodes.flatMap { node ->
        listOf(FlatAccount(node, depth)) + flatten(node.children, depth + 1)
    }
