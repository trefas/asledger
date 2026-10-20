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
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import com.trefas.asledger.data.network.ApiResult
import com.trefas.asledger.data.network.ConnectionProblem
import com.trefas.asledger.data.network.HealthInfo
import com.trefas.asledger.data.network.toProblem
import com.trefas.asledger.data.settings.ServerConfig
import com.trefas.asledger.ui.components.BrandHeader
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
    var validationError by rememberSaveable { mutableStateOf<String?>(null) }
    var problem by remember { mutableStateOf<ConnectionProblem?>(null) }
    var checking by remember { mutableStateOf(false) }
    val scope = rememberCoroutineScope()

    fun submit() {
        if (checking) return
        val normalized = ServerConfig.normalizeHost(host)
        val portValue = port.trim().toIntOrNull()
        when {
            normalized.isEmpty() -> {
                validationError = "Укажите IP-адрес или имя компьютера"
                return
            }

            portValue == null || portValue !in 1..65_535 -> {
                validationError = "Порт должен быть числом от 1 до 65535"
                return
            }
        }
        validationError = null
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
                    title = "Не удалось сохранить адрес",
                    hint = failure.message ?: "Попробуйте ещё раз."
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
                    text = "Подключение к книге",
                    style = MaterialTheme.typography.titleLarge
                )
                Text(
                    text = "Укажите адрес компьютера, на котором запущен asledgerd. " +
                        "Приложение проверит связь и привяжется к книге автоматически.",
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant
                )

                OutlinedTextField(
                    value = host,
                    onValueChange = {
                        host = it
                        validationError = null
                        problem = null
                    },
                    modifier = Modifier
                        .fillMaxWidth()
                        .testTag("setup-host"),
                    label = { Text("IP-адрес или имя") },
                    placeholder = { Text("например, 192.168.1.10") },
                    singleLine = true,
                    enabled = !checking,
                    isError = validationError != null,
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
                            validationError = null
                            problem = null
                        }
                    },
                    modifier = Modifier
                        .fillMaxWidth()
                        .testTag("setup-port"),
                    label = { Text("Порт") },
                    supportingText = { Text("По умолчанию 8778") },
                    singleLine = true,
                    enabled = !checking,
                    isError = validationError != null,
                    keyboardOptions = KeyboardOptions(
                        keyboardType = KeyboardType.Number,
                        imeAction = ImeAction.Done
                    ),
                    keyboardActions = KeyboardActions(onDone = { submit() })
                )

                if (validationError != null) {
                    Text(
                        text = validationError.orEmpty(),
                        style = MaterialTheme.typography.bodyMedium,
                        color = MaterialTheme.colorScheme.error
                    )
                }

                if (problem != null) {
                    Text(
                        text = problem?.title.orEmpty(),
                        style = MaterialTheme.typography.titleSmall,
                        color = MaterialTheme.colorScheme.error
                    )
                    Text(
                        text = problem?.hint.orEmpty(),
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
                        Text(if (problem != null) "Проверить ещё раз" else "Подключиться")
                    }
                }
            }
        }

        Spacer(Modifier.height(16.dp))
        Text(
            text = "Пароль не нужен. Привязка разрешена только в локальной сети; " +
                "отозвать устройство можно в web-интерфейсе asledger.",
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
            textAlign = TextAlign.Center
        )
    }
}
