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
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import com.trefas.asledger.data.settings.ServerConfig
import com.trefas.asledger.ui.components.BrandHeader
import kotlinx.coroutines.launch

/**
 * Первый экран: только адрес и порт сервера.
 *
 * Пароль не запрашивается: привязка устройства выполняется через enrollment
 * (веха S10). Здесь адрес сохраняется локально, чтобы следующий шаг мог
 * обратиться к `/api/v1/health`.
 */
@Composable
fun SetupScreen(
    onSave: suspend (ServerConfig) -> Unit,
    onConnected: () -> Unit,
    modifier: Modifier = Modifier,
) {
    var host by rememberSaveable { mutableStateOf("") }
    var port by rememberSaveable { mutableStateOf(ServerConfig.DEFAULT_PORT.toString()) }
    var error by rememberSaveable { mutableStateOf<String?>(null) }
    var saving by remember { mutableStateOf(false) }
    val scope = rememberCoroutineScope()

    fun submit() {
        if (saving) return
        val normalized = ServerConfig.normalizeHost(host)
        val portValue = port.trim().toIntOrNull()
        when {
            normalized.isEmpty() -> {
                error = "Укажите IP-адрес или имя компьютера"
                return
            }
            portValue == null || portValue !in 1..65_535 -> {
                error = "Порт должен быть числом от 1 до 65535"
                return
            }
        }
        error = null
        saving = true
        scope.launch {
            try {
                onSave(ServerConfig(host = normalized, port = portValue))
                onConnected()
            } catch (failure: Exception) {
                error = "Не удалось сохранить адрес: ${failure.message ?: "неизвестная ошибка"}"
            } finally {
                saving = false
            }
        }
    }

    Column(
        modifier = modifier
            .fillMaxSize()
            .windowInsetsPadding(WindowInsets.safeDrawing)
            .verticalScroll(rememberScrollState())
            .padding(horizontal = 24.dp, vertical = 32.dp),
        horizontalAlignment = Alignment.CenterHorizontally,
    ) {
        BrandHeader()
        Spacer(Modifier.height(32.dp))

        Card(
            modifier = Modifier.fillMaxWidth(),
            colors = CardDefaults.cardColors(
                containerColor = MaterialTheme.colorScheme.surface,
            ),
        ) {
            Column(
                modifier = Modifier.padding(20.dp),
                verticalArrangement = Arrangement.spacedBy(12.dp),
            ) {
                Text(
                    text = "Подключение к книге",
                    style = MaterialTheme.typography.titleLarge,
                )
                Text(
                    text = "Укажите адрес компьютера, на котором запущен asledgerd. " +
                        "Приложение найдёт книгу и привяжется к ней автоматически.",
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )

                OutlinedTextField(
                    value = host,
                    onValueChange = {
                        host = it
                        error = null
                    },
                    modifier = Modifier.fillMaxWidth(),
                    label = { Text("IP-адрес или имя") },
                    placeholder = { Text("например, 192.168.1.10") },
                    singleLine = true,
                    isError = error != null,
                    keyboardOptions = KeyboardOptions(
                        keyboardType = KeyboardType.Uri,
                        imeAction = ImeAction.Next,
                    ),
                )

                OutlinedTextField(
                    value = port,
                    onValueChange = { value ->
                        if (value.length <= 5 && value.all { it.isDigit() }) {
                            port = value
                            error = null
                        }
                    },
                    modifier = Modifier.fillMaxWidth(),
                    label = { Text("Порт") },
                    supportingText = { Text("По умолчанию 8778") },
                    singleLine = true,
                    isError = error != null,
                    keyboardOptions = KeyboardOptions(
                        keyboardType = KeyboardType.Number,
                        imeAction = ImeAction.Done,
                    ),
                    keyboardActions = KeyboardActions(onDone = { submit() }),
                )

                if (error != null) {
                    Text(
                        text = error.orEmpty(),
                        style = MaterialTheme.typography.bodyMedium,
                        color = MaterialTheme.colorScheme.error,
                    )
                }

                Button(
                    onClick = { submit() },
                    enabled = !saving,
                    modifier = Modifier.fillMaxWidth(),
                ) {
                    if (saving) {
                        CircularProgressIndicator(
                            modifier = Modifier.size(20.dp),
                            strokeWidth = 2.dp,
                            color = MaterialTheme.colorScheme.onPrimary,
                        )
                    } else {
                        Text("Подключиться")
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
            textAlign = TextAlign.Center,
        )
    }
}
