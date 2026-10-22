package com.trefas.asledger.ui.screens

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
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Add
import androidx.compose.material.icons.filled.Delete
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.DatePicker
import androidx.compose.material3.DatePickerDialog
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
import androidx.compose.material3.TextButton
import androidx.compose.material3.rememberDatePickerState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.unit.dp
import com.trefas.asledger.data.model.Account
import com.trefas.asledger.data.model.RoleRule
import com.trefas.asledger.ui.components.ScreenScaffold
import com.trefas.asledger.util.formatDate
import com.trefas.asledger.util.formatKop
import java.time.Instant
import java.time.ZoneId
import java.time.ZoneOffset

/** Форма проводки: роли, строки дебета и кредита, офлайн-сохранение. */
@OptIn(ExperimentalMaterial3Api::class, ExperimentalLayoutApi::class)
@Composable
fun EntryScreen(
    state: EntryUiState,
    onOpType: (String) -> Unit,
    onDescription: (String) -> Unit,
    onDate: (Long) -> Unit,
    onAddLine: (String) -> Unit,
    onRemoveLine: (Long) -> Unit,
    onAccount: (Long, Long) -> Unit,
    onAmount: (Long, String) -> Unit,
    onQuickAmount: (Long, Long) -> Unit,
    onRole: (Long, String) -> Unit,
    onSave: () -> Unit,
    onBack: () -> Unit,
    onSaved: () -> Unit,
    modifier: Modifier = Modifier
) {
    var showDatePicker by remember { mutableStateOf(false) }
    LaunchedEffect(state.saved) {
        if (state.saved) onSaved()
    }

    ScreenScaffold(
        title = if (state.isEditing) "Изменить проводку" else "Новая проводка",
        modifier = modifier,
        onBack = onBack,
        actions = {
            TextButton(
                onClick = onSave,
                enabled = !state.saving && !state.loadingRoles
            ) {
                Text(if (state.saving) "Сохраняем…" else "Сохранить")
            }
        }
    ) {
        LazyColumn(
            modifier = Modifier
                .fillMaxSize()
                .testTag("entry-form"),
            verticalArrangement = Arrangement.spacedBy(12.dp)
        ) {
            item {
                Text(
                    text = "Тип операции",
                    style = MaterialTheme.typography.labelLarge,
                    modifier = Modifier.padding(top = 8.dp)
                )
                FlowRow(
                    modifier = Modifier
                        .fillMaxWidth()
                        .padding(top = 6.dp),
                    horizontalArrangement = Arrangement.spacedBy(8.dp),
                    verticalArrangement = Arrangement.spacedBy(4.dp)
                ) {
                    state.opTypes.forEach { opType ->
                        FilterChip(
                            selected = state.opType == opType,
                            onClick = { onOpType(opType) },
                            label = { Text(opType.replaceFirstChar { it.uppercase() }) },
                            modifier = Modifier.testTag("entry-op-$opType")
                        )
                    }
                }
            }

            item {
                OutlinedTextField(
                    value = state.description,
                    onValueChange = onDescription,
                    label = { Text("Описание") },
                    placeholder = { Text("Например, продукты в магазине") },
                    singleLine = true,
                    modifier = Modifier
                        .fillMaxWidth()
                        .testTag("entry-description")
                )
            }

            item {
                OutlinedButton(
                    onClick = { showDatePicker = true },
                    modifier = Modifier.fillMaxWidth()
                ) {
                    Text("Дата: ${formatDate(state.date)}")
                }
            }

            if (state.loadingRoles) {
                item {
                    Text(
                        "Загружаем роли операции…",
                        style = MaterialTheme.typography.bodyMedium,
                        color = MaterialTheme.colorScheme.onSurfaceVariant
                    )
                }
            }
            state.rolesError?.let { error ->
                item {
                    Card(
                        colors = CardDefaults.cardColors(
                            containerColor = MaterialTheme.colorScheme.errorContainer
                        )
                    ) {
                        Text(
                            text = error,
                            modifier = Modifier.padding(12.dp),
                            color = MaterialTheme.colorScheme.onErrorContainer,
                            style = MaterialTheme.typography.bodyMedium
                        )
                    }
                }
            }

            item {
                SideHeader(
                    title = "Дебет",
                    total = formatKop(state.debitKop),
                    onAdd = { onAddLine("DEBIT") }
                )
            }
            items(state.lines.filter { it.side == "DEBIT" }, key = { it.key }) { line ->
                EntryLineCard(
                    line = line,
                    accounts = state.accounts.sortedBy { account ->
                        state.frequentAccountIds.indexOf(account.id)
                            .let { if (it < 0) Int.MAX_VALUE else it }
                    },
                    roles = rolesForLine(state, line),
                    onAccount = { onAccount(line.key, it) },
                    onAmount = { onAmount(line.key, it) },
                    onQuickAmount = { onQuickAmount(line.key, it) },
                    onRole = { onRole(line.key, it) },
                    onRemove = { onRemoveLine(line.key) }
                )
            }

            item {
                SideHeader(
                    title = "Кредит",
                    total = formatKop(state.creditKop),
                    onAdd = { onAddLine("CREDIT") }
                )
            }
            items(state.lines.filter { it.side == "CREDIT" }, key = { it.key }) { line ->
                EntryLineCard(
                    line = line,
                    accounts = state.accounts.sortedBy { account ->
                        state.frequentAccountIds.indexOf(account.id)
                            .let { if (it < 0) Int.MAX_VALUE else it }
                    },
                    roles = rolesForLine(state, line),
                    onAccount = { onAccount(line.key, it) },
                    onAmount = { onAmount(line.key, it) },
                    onQuickAmount = { onQuickAmount(line.key, it) },
                    onRole = { onRole(line.key, it) },
                    onRemove = { onRemoveLine(line.key) }
                )
            }

            item {
                val balanced = state.balanced
                Card(
                    modifier = Modifier
                        .fillMaxWidth()
                        .testTag("entry-balance"),
                    colors = CardDefaults.cardColors(
                        containerColor = if (balanced) {
                            MaterialTheme.colorScheme.secondaryContainer
                        } else {
                            MaterialTheme.colorScheme.surfaceVariant
                        }
                    )
                ) {
                    Column(Modifier.padding(12.dp)) {
                        Row(
                            Modifier.fillMaxWidth(),
                            horizontalArrangement = Arrangement.SpaceBetween
                        ) {
                            Text("Дебет")
                            Text(formatKop(state.debitKop), fontWeight = FontWeight.SemiBold)
                        }
                        Spacer(Modifier.height(4.dp))
                        Row(
                            Modifier.fillMaxWidth(),
                            horizontalArrangement = Arrangement.SpaceBetween
                        ) {
                            Text("Кредит")
                            Text(formatKop(state.creditKop), fontWeight = FontWeight.SemiBold)
                        }
                        if (!balanced) {
                            Spacer(Modifier.height(6.dp))
                            Text(
                                text = "Суммы дебета и кредита должны совпадать.",
                                style = MaterialTheme.typography.bodySmall,
                                color = MaterialTheme.colorScheme.error
                            )
                        }
                    }
                }
            }

            state.message?.let { message ->
                item {
                    Text(
                        text = message,
                        color = MaterialTheme.colorScheme.error,
                        style = MaterialTheme.typography.bodyMedium,
                        modifier = Modifier.testTag("entry-message")
                    )
                }
            }

            item {
                Button(
                    onClick = onSave,
                    enabled = !state.saving && !state.loadingRoles,
                    modifier = Modifier
                        .fillMaxWidth()
                        .testTag("entry-save")
                ) {
                    Text(if (state.saving) "Сохраняем…" else "Сохранить проводку")
                }
            }
            item {
                Text(
                    text = if (state.isEditing) {
                        "Изменения попадут в очередь и отправятся при связи с сервером."
                    } else {
                        "Проводка сохранится на устройстве и отправится, когда появится сеть."
                    },
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    modifier = Modifier.padding(bottom = 24.dp)
                )
            }
        }
    }

    if (showDatePicker) {
        val pickerState = rememberDatePickerState(
            initialSelectedDateMillis = state.date
        )
        DatePickerDialog(
            onDismissRequest = { showDatePicker = false },
            confirmButton = {
                TextButton(
                    onClick = {
                        val selected = pickerState.selectedDateMillis
                        if (selected != null) {
                            onDate(localNoon(selected))
                        }
                        showDatePicker = false
                    }
                ) { Text("Выбрать") }
            },
            dismissButton = {
                TextButton(onClick = { showDatePicker = false }) { Text("Отмена") }
            }
        ) {
            DatePicker(state = pickerState)
        }
    }
}

@Composable
private fun SideHeader(
    title: String,
    total: String,
    onAdd: () -> Unit
) {
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .padding(top = 8.dp),
        horizontalArrangement = Arrangement.SpaceBetween,
        verticalAlignment = Alignment.CenterVertically
    ) {
        Column {
            Text(title, style = MaterialTheme.typography.titleMedium)
            Text(
                total,
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant
            )
        }
        OutlinedButton(onClick = onAdd) {
            Icon(Icons.Filled.Add, contentDescription = null, modifier = Modifier.size(18.dp))
            Spacer(Modifier.size(4.dp))
            Text("Строка")
        }
    }
}

@Composable
private fun EntryLineCard(
    line: EntryLineUi,
    accounts: List<Account>,
    roles: List<RoleRule>,
    onAccount: (Long) -> Unit,
    onAmount: (String) -> Unit,
    onQuickAmount: (Long) -> Unit,
    onRole: (String) -> Unit,
    onRemove: () -> Unit
) {
    var accountMenu by remember { mutableStateOf(false) }
    var roleMenu by remember { mutableStateOf(false) }
    val account = accounts.firstOrNull { it.id == line.accountId }
    // В списке счетов показываем только типы, допустимые текущей ролью.
    // Иначе пользователь может выбрать счёт, который сервер отклонит.
    val allowedTypes = roles.firstOrNull { it.role == line.role }?.accountTypes.orEmpty()
    val allowedAccounts = if (allowedTypes.isEmpty()) {
        accounts
    } else {
        accounts.filter { it.type in allowedTypes }
    }

    Card(
        modifier = Modifier.fillMaxWidth(),
        colors = CardDefaults.cardColors(
            containerColor = MaterialTheme.colorScheme.surface
        )
    ) {
        Column(Modifier.padding(12.dp)) {
            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.SpaceBetween,
                verticalAlignment = Alignment.CenterVertically
            ) {
                Box {
                    OutlinedButton(
                        onClick = { accountMenu = true },
                        modifier = Modifier.testTag("entry-account-${line.key}")
                    ) {
                        Text(account?.name ?: "Выбрать счёт", maxLines = 1)
                    }
                    DropdownMenu(
                        expanded = accountMenu,
                        onDismissRequest = { accountMenu = false }
                    ) {
                        allowedAccounts.forEach { candidate ->
                            DropdownMenuItem(
                                text = { Text(candidate.fullPath) },
                                onClick = {
                                    accountMenu = false
                                    onAccount(candidate.id)
                                }
                            )
                        }
                    }
                }
                IconButton(onClick = onRemove) {
                    Icon(Icons.Filled.Delete, contentDescription = "Удалить строку")
                }
            }

            Spacer(Modifier.height(8.dp))
            Box {
                OutlinedButton(
                    onClick = { roleMenu = true },
                    modifier = Modifier.testTag("entry-role-${line.key}")
                ) {
                    Text(line.role.ifBlank { "Выбрать роль" })
                }
                DropdownMenu(
                    expanded = roleMenu,
                    onDismissRequest = { roleMenu = false }
                ) {
                    roles.forEach { rule ->
                        DropdownMenuItem(
                            text = {
                                Text(
                                    rule.role + if (rule.accountTypes.isEmpty()) {
                                        ""
                                    } else {
                                        " · " + rule.accountTypes.joinToString(", ") {
                                            it.title
                                        }
                                    }
                                )
                            },
                            onClick = {
                                roleMenu = false
                                onRole(rule.role)
                            }
                        )
                    }
                }
            }

            Spacer(Modifier.height(8.dp))
            OutlinedTextField(
                value = line.amount,
                onValueChange = onAmount,
                label = { Text("Сумма, ₽") },
                singleLine = true,
                isError = line.error != null,
                supportingText = line.error?.let { error -> { Text(error) } },
                keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Decimal),
                modifier = Modifier
                    .fillMaxWidth()
                    .testTag("entry-amount-${line.key}")
            )
            FlowRow(horizontalArrangement = Arrangement.spacedBy(6.dp)) {
                listOf(100L, 500L, 1_000L).forEach { rubles ->
                    TextButton(onClick = { onQuickAmount(rubles * 100) }) {
                        Text("+$rubles", maxLines = 1)
                    }
                }
                TextButton(onClick = { onAmount("") }) {
                    Text("Очистить", maxLines = 1)
                }
            }
        }
    }
}

private fun rolesForLine(state: EntryUiState, line: EntryLineUi): List<RoleRule> {
    val roles = state.roles ?: return emptyList()
    val type = state.accounts.firstOrNull { it.id == line.accountId }?.type
    return roles.roles.filter { rule ->
        rule.side == line.side &&
            (type == null || rule.accountTypes.isEmpty() || rule.accountTypes.contains(type))
    }.ifEmpty {
        roles.roles.filter { it.side == line.side }
    }
}

private fun localNoon(utcMillis: Long): Long {
    val date = Instant.ofEpochMilli(utcMillis).atZone(ZoneOffset.UTC).toLocalDate()
    return date.atStartOfDay(ZoneId.systemDefault()).plusHours(12)
        .toInstant().toEpochMilli()
}
