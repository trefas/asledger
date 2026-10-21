package com.trefas.asledger.ui.screens

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
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
import androidx.compose.ui.unit.dp
import com.trefas.asledger.BuildConfig
import com.trefas.asledger.data.settings.ServerConfig
import com.trefas.asledger.ui.components.ScreenScaffold

/** Настройки: сервер, пользователь, диагностика и сведения о сборке. */
@Composable
fun SettingsScreen(
    config: ServerConfig?,
    onDiagnostics: () -> Unit,
    onHelp: () -> Unit,
    onChangeUser: () -> Unit,
    onChangeServer: () -> Unit,
    onForgetServer: () -> Unit,
    modifier: Modifier = Modifier
) {
    var confirmForget by remember { mutableStateOf(false) }

    ScreenScaffold(title = "Настройки", modifier = modifier) {
        Column(
            modifier = Modifier
                .fillMaxSize()
                .verticalScroll(rememberScrollState())
                .padding(vertical = 16.dp),
            verticalArrangement = Arrangement.spacedBy(16.dp)
        ) {
            Card(
                modifier = Modifier.fillMaxWidth(),
                colors = CardDefaults.cardColors(
                    containerColor = MaterialTheme.colorScheme.surface
                )
            ) {
                Column(
                    modifier = Modifier.padding(16.dp),
                    verticalArrangement = Arrangement.spacedBy(8.dp)
                ) {
                    Text("Сервер", style = MaterialTheme.typography.titleMedium)
                    Text(
                        text = config?.baseUrl ?: "Книга не подключена",
                        style = MaterialTheme.typography.bodyLarge,
                        color = MaterialTheme.colorScheme.primary
                    )
                    if (config != null && config.serverVersion.isNotBlank()) {
                        Text(
                            text = "Версия: ${config.serverVersion}, формат ${config.formatVersion}",
                            style = MaterialTheme.typography.bodySmall,
                            color = MaterialTheme.colorScheme.onSurfaceVariant
                        )
                    }
                    if (config != null && config.hasSession) {
                        Text(
                            text = "Пользователь: ${config.userName}",
                            style = MaterialTheme.typography.bodyMedium
                        )
                    }
                    Text(
                        text = "Порт по умолчанию — 8778. Адрес можно изменить, " +
                            "если компьютер переехал в другую сеть.",
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant
                    )
                    OutlinedButton(onClick = onChangeServer) {
                        Text("Изменить адрес")
                    }
                    if (config?.hasSession == true) {
                        OutlinedButton(onClick = onChangeUser) {
                            Text("Сменить пользователя")
                        }
                    }
                    TextButton(onClick = { confirmForget = true }) {
                        Text(
                            text = "Забыть сервер",
                            color = MaterialTheme.colorScheme.error
                        )
                    }
                }
            }

            Card(
                modifier = Modifier.fillMaxWidth(),
                colors = CardDefaults.cardColors(
                    containerColor = MaterialTheme.colorScheme.surface
                )
            ) {
                Column(
                    modifier = Modifier.padding(16.dp),
                    verticalArrangement = Arrangement.spacedBy(8.dp)
                ) {
                    Text("Диагностика", style = MaterialTheme.typography.titleMedium)
                    Text(
                        text = "Проверка связи, версия сервера, время последней " +
                            "синхронизации и состояние токена. Токен не показывается.",
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant
                    )
                    Button(onClick = onDiagnostics) {
                        Text("Открыть диагностику")
                    }
                }
            }

            Card(
                modifier = Modifier.fillMaxWidth(),
                colors = CardDefaults.cardColors(
                    containerColor = MaterialTheme.colorScheme.surface
                )
            ) {
                Column(
                    modifier = Modifier.padding(16.dp),
                    verticalArrangement = Arrangement.spacedBy(8.dp)
                ) {
                    Text("Помощь", style = MaterialTheme.typography.titleMedium)
                    Text(
                        text = "Подсказки о статусах, дублях, офлайн-работе и " +
                            "привязке устройства.",
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant
                    )
                    OutlinedButton(onClick = onHelp) { Text("Открыть справку") }
                }
            }

            Card(
                modifier = Modifier.fillMaxWidth(),
                colors = CardDefaults.cardColors(
                    containerColor = MaterialTheme.colorScheme.surface
                )
            ) {
                Column(
                    modifier = Modifier.padding(16.dp),
                    verticalArrangement = Arrangement.spacedBy(4.dp)
                ) {
                    Text("О приложении", style = MaterialTheme.typography.titleMedium)
                    Text(
                        text = "asledger ${BuildConfig.VERSION_NAME}",
                        style = MaterialTheme.typography.bodyMedium
                    )
                    Text(
                        text = "Android 14 (API 34) и новее. Данные книги хранятся " +
                            "на вашем сервере; приложение работает с локальной копией " +
                            "и синхронизируется, когда сервер доступен.",
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant
                    )
                }
            }
        }
    }

    if (confirmForget) {
        AlertDialog(
            onDismissRequest = { confirmForget = false },
            title = { Text("Забыть сервер?") },
            text = {
                Text(
                    "Адрес и токен будут удалены с устройства. Локальные черновики " +
                        "проводок не удаляются. Для продолжения потребуется привязка заново."
                )
            },
            confirmButton = {
                TextButton(
                    onClick = {
                        confirmForget = false
                        onForgetServer()
                    }
                ) {
                    Text("Забыть")
                }
            },
            dismissButton = {
                TextButton(onClick = { confirmForget = false }) {
                    Text("Отмена")
                }
            }
        )
    }
}
