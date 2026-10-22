package com.trefas.asledger.ui.screens

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.safeDrawing
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.windowInsetsPadding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.text.KeyboardActions
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import com.trefas.asledger.R
import com.trefas.asledger.data.network.ApiResult
import com.trefas.asledger.data.network.ConnectionDetail
import com.trefas.asledger.data.network.ConnectionProblem
import com.trefas.asledger.data.network.HealthInfo
import com.trefas.asledger.data.network.toProblem
import com.trefas.asledger.data.settings.ServerConfig
import com.trefas.asledger.ui.components.BrandHeader
import com.trefas.asledger.ui.components.hintText
import com.trefas.asledger.ui.components.titleText
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.launch

/**
 * Первый экран: только адрес и порт сервера.
 *
 * Пароль не запрашивается. Перед сохранением адреса приложение проверяет
 * `/api/v1/health`, версию и формат данных; при ошибке показывает отдельное
 * объяснение и кнопку «Проверить ещё раз».
 */
@Composable
fun SetupScreen(
    initialHost: String = "",
    initialPort: Int = ServerConfig.DEFAULT_PORT,
    onSubmit: suspend (ServerConfig) -> ApiResult<HealthInfo>,
    onSuccess: suspend (ServerConfig, HealthInfo) -> Unit,
    modifier: Modifier = Modifier
) {
    var host by rememberSaveable(initialHost) { mutableStateOf(initialHost) }
    var port by rememberSaveable(initialPort) { mutableStateOf(initialPort.toString()) }
    var validationErrorRes by rememberSaveable { mutableStateOf<Int?>(null) }
    var problem by remember { mutableStateOf<ConnectionProblem?>(null) }
    var checking by remember { mutableStateOf(false) }
    val scope = rememberCoroutineScope()

    fun submit() {
        if (checking) return
        val normalized = ServerConfig.normalizeHost(host)
        val portValue = port.trim().toIntOrNull()
        when {
            normalized.isEmpty() -> {
                validationErrorRes = R.string.setup_validation_host
                return
            }

            portValue == null || portValue !in 1..65_535 -> {
                validationErrorRes = R.string.setup_validation_port
                return
            }
        }
        validationErrorRes = null
        problem = null
        checking = true
        val config = ServerConfig(host = normalized, port = portValue)
        scope.launch {
            try {
                when (val result = onSubmit(config)) {
                    is ApiResult.Ok -> onSuccess(config, result.value)
                    is ApiResult.Failure -> problem = result.error.toProblem()
                }
            } catch (cancelled: CancellationException) {
                throw cancelled
            } catch (failure: Exception) {
                problem = ConnectionProblem(
                    titleRes = R.string.setup_save_error_title,
                    hint = failure.message?.takeIf { it.isNotBlank() }?.let {
                        ConnectionDetail.Text(it)
                    } ?: ConnectionDetail.Resource(R.string.setup_try_again)
                )
            } finally {
                checking = false
            }
        }
    }

    Column(
        modifier = modifier
            .fillMaxSize()
            .windowInsetsPadding(WindowInsets.safeDrawing)
            .verticalScroll(rememberScrollState())
            .padding(horizontal = 24.dp, vertical = 32.dp),
        horizontalAlignment = Alignment.CenterHorizontally
    ) {
        BrandHeader()
        Spacer(Modifier.height(32.dp))

        Card(
            modifier = Modifier.fillMaxWidth(),
            colors = CardDefaults.cardColors(
                containerColor = MaterialTheme.colorScheme.surface
            )
        ) {
            Column(
                modifier = Modifier.padding(20.dp),
                verticalArrangement = Arrangement.spacedBy(12.dp)
            ) {
                Text(
                    text = stringResource(R.string.setup_title),
                    style = MaterialTheme.typography.titleLarge
                )
                Text(
                    text = stringResource(R.string.setup_description),
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant
                )

                OutlinedTextField(
                    value = host,
                    onValueChange = {
                        host = it
                        validationErrorRes = null
                        problem = null
                    },
                    modifier = Modifier
                        .fillMaxWidth()
                        .testTag("setup-host"),
                    label = { Text(stringResource(R.string.setup_host_label)) },
                    placeholder = { Text(stringResource(R.string.setup_host_placeholder)) },
                    singleLine = true,
                    enabled = !checking,
                    isError = validationErrorRes != null,
                    keyboardOptions = KeyboardOptions(
                        keyboardType = KeyboardType.Uri,
                        imeAction = ImeAction.Next
                    )
                )

                OutlinedTextField(
                    value = port,
                    onValueChange = { value ->
                        if (value.length <= 5 && value.all { it.isDigit() }) {
                            port = value
                            validationErrorRes = null
                            problem = null
                        }
                    },
                    modifier = Modifier
                        .fillMaxWidth()
                        .testTag("setup-port"),
                    label = { Text(stringResource(R.string.setup_port_label)) },
                    supportingText = { Text(stringResource(R.string.setup_port_supporting)) },
                    singleLine = true,
                    enabled = !checking,
                    isError = validationErrorRes != null,
                    keyboardOptions = KeyboardOptions(
                        keyboardType = KeyboardType.Number,
                        imeAction = ImeAction.Done
                    ),
                    keyboardActions = KeyboardActions(onDone = { submit() })
                )

                validationErrorRes?.let { errorRes ->
                    Text(
                        text = stringResource(errorRes),
                        style = MaterialTheme.typography.bodyMedium,
                        color = MaterialTheme.colorScheme.error
                    )
                }

                if (problem != null) {
                    Text(
                        text = problem?.titleText().orEmpty(),
                        style = MaterialTheme.typography.titleSmall,
                        color = MaterialTheme.colorScheme.error
                    )
                    Text(
                        text = problem?.hintText().orEmpty(),
                        style = MaterialTheme.typography.bodyMedium,
                        color = MaterialTheme.colorScheme.onSurfaceVariant
                    )
                }

                Button(
                    onClick = { submit() },
                    enabled = !checking,
                    modifier = Modifier
                        .fillMaxWidth()
                        .testTag("setup-submit")
                ) {
                    if (checking) {
                        CircularProgressIndicator(
                            modifier = Modifier.size(20.dp),
                            strokeWidth = 2.dp,
                            color = MaterialTheme.colorScheme.onPrimary
                        )
                    } else {
                        Text(
                            stringResource(
                                if (problem != null) {
                                    R.string.setup_check_again
                                } else {
                                    R.string.setup_connect
                                }
                            )
                        )
                    }
                }
            }
        }

        Spacer(Modifier.height(16.dp))
        Text(
            text = stringResource(R.string.setup_footer),
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
            textAlign = TextAlign.Center
        )
    }
}
