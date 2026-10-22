package com.trefas.asledger.ui.screens

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import com.trefas.asledger.R
import com.trefas.asledger.data.network.ApiResult
import com.trefas.asledger.data.network.ConnectionError
import com.trefas.asledger.data.network.ConnectionProblem
import com.trefas.asledger.data.network.HealthInfo
import com.trefas.asledger.data.network.toProblem
import com.trefas.asledger.data.settings.ServerConfig
import com.trefas.asledger.ui.components.ScreenScaffold
import com.trefas.asledger.ui.components.hintText
import com.trefas.asledger.ui.components.titleText
import com.trefas.asledger.util.formatDateTime
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.launch

/**
 * Диагностика подключения: адрес, версия сервера, формат, пользователь,
 * последняя синхронизация. Токен не показывается никогда.
 */
@Composable
fun DiagnosticsScreen(
    config: ServerConfig?,
    onCheck: suspend () -> ApiResult<HealthInfo>,
    onHealth: suspend (HealthInfo) -> Unit,
    modifier: Modifier = Modifier
) {
    var checking by remember { mutableStateOf(false) }
    var problem by remember { mutableStateOf<ConnectionProblem?>(null) }
    var checkedHealth by remember { mutableStateOf<HealthInfo?>(null) }
    val scope = rememberCoroutineScope()

    fun check() {
        if (checking) return
        checking = true
        problem = null
        checkedHealth = null
        scope.launch {
            try {
                when (val result = onCheck()) {
                    is ApiResult.Ok -> {
                        onHealth(result.value)
                        checkedHealth = result.value
                    }

                    is ApiResult.Failure -> problem = result.error.toProblem()
                }
            } catch (cancelled: CancellationException) {
                throw cancelled
            } catch (failure: Exception) {
                problem = ConnectionError.Unexpected(
                    detail = failure.message,
                    detailRes = R.string.connection_detail_check_failed
                ).toProblem()
            } finally {
                checking = false
            }
        }
    }

    ScreenScaffold(title = stringResource(R.string.diagnostics_title), modifier = modifier) {
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
                    verticalArrangement = Arrangement.spacedBy(10.dp)
                ) {
                    Text(stringResource(R.string.diagnostics_connection), style = MaterialTheme.typography.titleMedium)
                    DiagnosticRow(
                        stringResource(R.string.diagnostics_address),
                        config?.baseUrl ?: stringResource(R.string.diagnostics_not_configured)
                    )
                    DiagnosticRow(
                        stringResource(R.string.diagnostics_server_version),
                        config?.serverVersion?.takeIf { it.isNotBlank() }
                            ?: stringResource(R.string.diagnostics_unknown)
                    )
                    DiagnosticRow(
                        stringResource(R.string.diagnostics_format),
                        config?.formatVersion?.takeIf { it > 0 }?.toString() ?: "—"
                    )
                    DiagnosticRow(
                        stringResource(R.string.diagnostics_user),
                        config?.userName?.takeIf { it.isNotBlank() }
                            ?: stringResource(R.string.diagnostics_not_selected)
                    )
                    DiagnosticRow(
                        stringResource(R.string.diagnostics_device),
                        config?.clientId?.takeIf { it.isNotBlank() } ?: "—"
                    )
                    DiagnosticRow(
                        stringResource(R.string.diagnostics_last_sync),
                        (config?.lastSyncAt ?: 0).takeIf { it > 0 }?.let { formatDateTime(it) }
                            ?: stringResource(R.string.diagnostics_never)
                    )
                    DiagnosticRow(
                        stringResource(R.string.diagnostics_token),
                        stringResource(R.string.diagnostics_token_value)
                    )
                }
            }

            if (problem != null) {
                Card(
                    modifier = Modifier.fillMaxWidth(),
                    colors = CardDefaults.cardColors(
                        containerColor = MaterialTheme.colorScheme.errorContainer
                    )
                ) {
                    Column(
                        modifier = Modifier.padding(16.dp),
                        verticalArrangement = Arrangement.spacedBy(4.dp)
                    ) {
                        Text(
                            text = problem?.titleText().orEmpty(),
                            style = MaterialTheme.typography.titleSmall,
                            color = MaterialTheme.colorScheme.onErrorContainer
                        )
                        Text(
                            text = problem?.hintText().orEmpty(),
                            style = MaterialTheme.typography.bodyMedium,
                            color = MaterialTheme.colorScheme.onErrorContainer
                        )
                    }
                }
            }

            checkedHealth?.let { health ->
                Text(
                    text = stringResource(
                        R.string.diagnostics_success,
                        health.version,
                        health.formatVersion
                    ),
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.primary
                )
            }

            Button(
                onClick = { check() },
                enabled = !checking && config != null,
                modifier = Modifier.fillMaxWidth()
            ) {
                if (checking) {
                    CircularProgressIndicator(
                        modifier = Modifier
                            .padding(end = 8.dp)
                            .size(18.dp),
                        strokeWidth = 2.dp
                    )
                }
                Text(
                    stringResource(
                        if (checking) R.string.diagnostics_checking else R.string.diagnostics_check
                    )
                )
            }
        }
    }
}

@Composable
private fun DiagnosticRow(label: String, value: String) {
    Row(modifier = Modifier.fillMaxWidth()) {
        Text(
            text = label,
            style = MaterialTheme.typography.bodyMedium,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
            modifier = Modifier.padding(end = 12.dp)
        )
        Text(
            text = value,
            style = MaterialTheme.typography.bodyMedium,
            modifier = Modifier.weight(1f)
        )
    }
}
