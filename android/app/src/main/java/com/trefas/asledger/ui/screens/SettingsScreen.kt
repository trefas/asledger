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
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import com.trefas.asledger.BuildConfig
import com.trefas.asledger.R
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

    ScreenScaffold(title = stringResource(R.string.settings_title), modifier = modifier) {
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
                    Text(stringResource(R.string.settings_server), style = MaterialTheme.typography.titleMedium)
                    Text(
                        text = config?.baseUrl ?: stringResource(R.string.settings_not_connected),
                        style = MaterialTheme.typography.bodyLarge,
                        color = MaterialTheme.colorScheme.primary
                    )
                    if (config != null && config.serverVersion.isNotBlank()) {
                        Text(
                            text = stringResource(
                                R.string.settings_version_format,
                                config.serverVersion,
                                config.formatVersion
                            ),
                            style = MaterialTheme.typography.bodySmall,
                            color = MaterialTheme.colorScheme.onSurfaceVariant
                        )
                    }
                    if (config != null && config.hasSession) {
                        Text(
                            text = stringResource(R.string.settings_user, config.userName),
                            style = MaterialTheme.typography.bodyMedium
                        )
                    }
                    Text(
                        text = stringResource(R.string.settings_port_hint),
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant
                    )
                    OutlinedButton(onClick = onChangeServer) {
                        Text(stringResource(R.string.action_change_address))
                    }
                    if (config?.hasSession == true) {
                        OutlinedButton(onClick = onChangeUser) {
                            Text(stringResource(R.string.settings_change_user))
                        }
                    }
                    TextButton(onClick = { confirmForget = true }) {
                        Text(
                            text = stringResource(R.string.settings_forget_server),
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
                    Text(stringResource(R.string.settings_diagnostics), style = MaterialTheme.typography.titleMedium)
                    Text(
                        text = stringResource(R.string.settings_diagnostics_hint),
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant
                    )
                    Button(onClick = onDiagnostics) {
                        Text(stringResource(R.string.settings_open_diagnostics))
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
                    Text(stringResource(R.string.settings_help), style = MaterialTheme.typography.titleMedium)
                    Text(
                        text = stringResource(R.string.settings_help_hint),
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant
                    )
                    OutlinedButton(onClick = onHelp) { Text(stringResource(R.string.settings_open_help)) }
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
                    Text(stringResource(R.string.settings_about), style = MaterialTheme.typography.titleMedium)
                    Text(
                        text = stringResource(R.string.settings_about_version, BuildConfig.VERSION_NAME),
                        style = MaterialTheme.typography.bodyMedium
                    )
                    Text(
                        text = stringResource(R.string.settings_about_text),
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
            title = { Text(stringResource(R.string.settings_forget_title)) },
            text = {
                Text(stringResource(R.string.settings_forget_text))
            },
            confirmButton = {
                TextButton(
                    onClick = {
                        confirmForget = false
                        onForgetServer()
                    }
                ) {
                    Text(stringResource(R.string.settings_forget_confirm))
                }
            },
            dismissButton = {
                TextButton(onClick = { confirmForget = false }) {
                    Text(stringResource(R.string.action_cancel))
                }
            }
        )
    }
}
