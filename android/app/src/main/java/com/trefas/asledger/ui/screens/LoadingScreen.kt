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
import androidx.compose.foundation.layout.windowInsetsPadding
import androidx.compose.material3.Button
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import com.trefas.asledger.R
import com.trefas.asledger.data.network.ConnectionError
import com.trefas.asledger.data.settings.ServerConfig
import com.trefas.asledger.ui.components.BrandMark
import com.trefas.asledger.ui.components.hintText
import com.trefas.asledger.ui.components.titleText

/**
 * Экран первичной загрузки снапшота.
 *
 * Пока книга не импортирована, журнал не показывается: иначе пользователь
 * увидел бы пустую книгу и решил, что данные потеряны.
 */
@Composable
fun LoadingScreen(
    config: ServerConfig,
    state: LoadingState,
    onRetry: () -> Unit,
    onCancel: () -> Unit,
    onReenroll: () -> Unit,
    onChangeServer: () -> Unit,
    modifier: Modifier = Modifier
) {
    val progress = (state as? LoadingState.Running)?.progress
    Column(
        modifier = modifier
            .fillMaxSize()
            .windowInsetsPadding(WindowInsets.safeDrawing)
            .padding(horizontal = 24.dp, vertical = 32.dp),
        horizontalAlignment = Alignment.CenterHorizontally,
        verticalArrangement = Arrangement.Center
    ) {
        BrandMark(size = 96.dp)
        Spacer(Modifier.height(24.dp))
        Text(
            text = stringResource(R.string.loading_title),
            style = MaterialTheme.typography.headlineSmall,
            textAlign = TextAlign.Center
        )
        Text(
            text = "${config.host}:${config.port}",
            style = MaterialTheme.typography.bodyMedium,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
            modifier = Modifier.padding(top = 4.dp)
        )
        Spacer(Modifier.height(24.dp))

        when (state) {
            LoadingState.Starting, is LoadingState.Running -> {
                LinearProgressIndicator(
                    progress = { (progress?.percent ?: 0) / 100f },
                    modifier = Modifier
                        .fillMaxWidth()
                        .testTag("loading-progress")
                )
                Text(
                    text = progress?.let { stringResource(it.titleRes) }
                        ?: stringResource(R.string.progress_preparing),
                    style = MaterialTheme.typography.bodyLarge,
                    modifier = Modifier.padding(top = 12.dp)
                )
                Text(
                    text = stringResource(R.string.loading_percent, progress?.percent ?: 0),
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    modifier = Modifier.padding(top = 4.dp)
                )
                Spacer(Modifier.height(20.dp))
                OutlinedButton(
                    onClick = onCancel,
                    modifier = Modifier.testTag("loading-cancel")
                ) {
                    Text(stringResource(R.string.loading_cancel))
                }
            }

            LoadingState.Success -> {
                Text(
                    text = stringResource(R.string.loading_success),
                    style = MaterialTheme.typography.titleMedium
                )
            }

            is LoadingState.Failure -> {
                Text(
                    text = state.problem.titleText(),
                    style = MaterialTheme.typography.titleMedium,
                    color = MaterialTheme.colorScheme.error,
                    textAlign = TextAlign.Center
                )
                Text(
                    text = state.problem.hintText(),
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    textAlign = TextAlign.Center,
                    modifier = Modifier.padding(top = 8.dp)
                )
                Spacer(Modifier.height(20.dp))
                if (state.error == ConnectionError.Unauthorized) {
                    Button(
                        onClick = onReenroll,
                        modifier = Modifier
                            .fillMaxWidth()
                            .testTag("loading-reenroll")
                    ) {
                        Text(stringResource(R.string.loading_reenroll))
                    }
                } else {
                    Button(
                        onClick = onRetry,
                        modifier = Modifier
                            .fillMaxWidth()
                            .testTag("loading-retry")
                    ) {
                        Text(stringResource(R.string.action_retry))
                    }
                }
                OutlinedButton(
                    onClick = onChangeServer,
                    modifier = Modifier
                        .fillMaxWidth()
                        .padding(top = 8.dp)
                ) {
                    Text(stringResource(R.string.action_change_address))
                }
            }

            LoadingState.Cancelled -> {
                Text(
                    text = stringResource(R.string.loading_cancelled),
                    style = MaterialTheme.typography.titleMedium
                )
                Text(
                    text = stringResource(R.string.loading_cancelled_text),
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    textAlign = TextAlign.Center,
                    modifier = Modifier.padding(top = 8.dp)
                )
                Spacer(Modifier.height(20.dp))
                Button(
                    onClick = onRetry,
                    modifier = Modifier
                        .fillMaxWidth()
                        .testTag("loading-retry")
                ) {
                    Text(stringResource(R.string.loading_continue))
                }
                OutlinedButton(
                    onClick = onChangeServer,
                    modifier = Modifier
                        .fillMaxWidth()
                        .padding(top = 8.dp)
                ) {
                    Text(stringResource(R.string.action_change_address))
                }
            }
        }
    }
}
