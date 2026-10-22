package com.trefas.asledger.ui.screens

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
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
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.RadioButton
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import com.trefas.asledger.R
import com.trefas.asledger.data.network.ConnectionError
import com.trefas.asledger.data.network.ConnectionProblem
import com.trefas.asledger.data.network.EnrollResult
import com.trefas.asledger.data.network.EnrollUser
import com.trefas.asledger.data.network.toProblem
import com.trefas.asledger.data.settings.ServerConfig
import com.trefas.asledger.ui.components.BrandMark
import com.trefas.asledger.ui.components.hintText
import com.trefas.asledger.ui.components.titleText
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.launch

private sealed interface EnrollUiState {
    data object Loading : EnrollUiState
    data class Choice(val users: List<EnrollUser>) : EnrollUiState
    data class Error(val problem: ConnectionProblem) : EnrollUiState
}

/**
 * Привязка устройства без пароля.
 *
 * При одном пользователе токен выдаётся сразу. Если в книге несколько
 * пользователей, показывается список имён — без секретов и паролей.
 */
@Composable
fun EnrollScreen(
    config: ServerConfig,
    onEnroll: suspend (Long?) -> EnrollResult,
    onEnrolled: suspend (EnrollResult.Success) -> ConnectionError?,
    onDone: () -> Unit,
    onChangeServer: () -> Unit,
    modifier: Modifier = Modifier
) {
    var state by remember { mutableStateOf<EnrollUiState>(EnrollUiState.Loading) }
    var selectedUserId by remember { mutableStateOf<Long?>(null) }
    var busy by remember { mutableStateOf(false) }
    val scope = rememberCoroutineScope()

    fun runEnroll(userId: Long?) {
        if (busy) return
        busy = true
        selectedUserId = userId
        state = EnrollUiState.Loading
        scope.launch {
            try {
                when (val result = onEnroll(userId)) {
                    is EnrollResult.Success -> {
                        val failure = onEnrolled(result)
                        if (failure == null) {
                            onDone()
                        } else {
                            state = EnrollUiState.Error(failure.toProblem())
                        }
                    }

                    is EnrollResult.UserRequired -> {
                        selectedUserId = null
                        state = EnrollUiState.Choice(result.users)
                    }

                    is EnrollResult.Failure -> {
                        state = EnrollUiState.Error(result.error.toProblem())
                    }
                }
            } catch (cancelled: CancellationException) {
                throw cancelled
            } catch (failure: Exception) {
                state = EnrollUiState.Error(
                    ConnectionError.Unexpected(
                        detail = failure.message,
                        detailRes = R.string.connection_detail_enroll_save
                    ).toProblem()
                )
            } finally {
                busy = false
            }
        }
    }

    LaunchedEffect(config.clientId) {
        runEnroll(null)
    }

    Column(
        modifier = modifier
            .fillMaxSize()
            .windowInsetsPadding(WindowInsets.safeDrawing)
            .verticalScroll(rememberScrollState())
            .padding(horizontal = 24.dp, vertical = 32.dp),
        horizontalAlignment = Alignment.CenterHorizontally
    ) {
        BrandMark(size = 80.dp)
        Spacer(Modifier.height(16.dp))
        Text(
            text = stringResource(R.string.enroll_title),
            style = MaterialTheme.typography.headlineMedium
        )
        Spacer(Modifier.height(4.dp))
        Text(
            text = config.baseUrl,
            style = MaterialTheme.typography.bodyMedium,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
            textAlign = TextAlign.Center
        )
        if (config.serverVersion.isNotBlank()) {
            Text(
                text = stringResource(R.string.enroll_server_version, config.serverVersion),
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant
            )
        }
        Spacer(Modifier.height(24.dp))

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
                when (val current = state) {
                    EnrollUiState.Loading -> {
                        Row(
                            modifier = Modifier.fillMaxWidth(),
                            horizontalArrangement = Arrangement.Center,
                            verticalAlignment = Alignment.CenterVertically
                        ) {
                            CircularProgressIndicator(
                                modifier = Modifier.size(24.dp),
                                strokeWidth = 2.dp
                            )
                            Text(
                                text = stringResource(R.string.enroll_loading),
                                style = MaterialTheme.typography.bodyLarge,
                                modifier = Modifier.padding(start = 12.dp)
                            )
                        }
                    }

                    is EnrollUiState.Choice -> {
                        Text(
                            text = stringResource(R.string.enroll_choice_title),
                            style = MaterialTheme.typography.titleMedium
                        )
                        Text(
                            text = stringResource(R.string.enroll_choice_text),
                            style = MaterialTheme.typography.bodySmall,
                            color = MaterialTheme.colorScheme.onSurfaceVariant
                        )
                        current.users.forEach { user ->
                            Row(
                                modifier = Modifier
                                    .fillMaxWidth()
                                    .clickable { selectedUserId = user.id }
                                    .padding(vertical = 4.dp),
                                verticalAlignment = Alignment.CenterVertically
                            ) {
                                RadioButton(
                                    selected = selectedUserId == user.id,
                                    onClick = { selectedUserId = user.id }
                                )
                                Text(
                                    text = user.name,
                                    style = MaterialTheme.typography.bodyLarge
                                )
                            }
                        }
                        Button(
                            onClick = { runEnroll(selectedUserId) },
                            enabled = selectedUserId != null && !busy,
                            modifier = Modifier.fillMaxWidth()
                        ) {
                            Text(stringResource(R.string.action_continue))
                        }
                    }

                    is EnrollUiState.Error -> {
                        Text(
                            text = current.problem.titleText(),
                            style = MaterialTheme.typography.titleMedium,
                            color = MaterialTheme.colorScheme.error
                        )
                        Text(
                            text = current.problem.hintText(),
                            style = MaterialTheme.typography.bodyMedium,
                            color = MaterialTheme.colorScheme.onSurfaceVariant
                        )
                        Button(
                            onClick = { runEnroll(selectedUserId) },
                            enabled = !busy,
                            modifier = Modifier.fillMaxWidth()
                        ) {
                            Text(stringResource(R.string.action_retry))
                        }
                        OutlinedButton(
                            onClick = onChangeServer,
                            modifier = Modifier.fillMaxWidth()
                        ) {
                            Text(stringResource(R.string.action_change_address))
                        }
                    }
                }
            }
        }

        Spacer(Modifier.height(16.dp))
        Text(
            text = stringResource(R.string.enroll_footer),
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
            textAlign = TextAlign.Center
        )
    }
}
