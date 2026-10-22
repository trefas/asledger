package com.trefas.asledger.ui.screens

import androidx.annotation.StringRes
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import com.trefas.asledger.R
import com.trefas.asledger.ui.components.ScreenScaffold

private data class HelpTopic(
    @StringRes val titleRes: Int,
    @StringRes val textRes: Int
)

private val topics = listOf(
    HelpTopic(R.string.help_password_title, R.string.help_password_text),
    HelpTopic(R.string.help_create_entry_title, R.string.help_create_entry_text),
    HelpTopic(R.string.help_debit_title, R.string.hint_entry),
    HelpTopic(R.string.help_role_title, R.string.hint_role),
    HelpTopic(R.string.help_balance_title, R.string.hint_balance),
    HelpTopic(R.string.help_status_title, R.string.hint_status),
    HelpTopic(R.string.help_duplicate_title, R.string.help_duplicate_text),
    HelpTopic(R.string.help_storage_title, R.string.help_storage_text),
    HelpTopic(R.string.help_accessibility_title, R.string.help_accessibility_text)
)

/** Справка и повторный онбординг. */
@Composable
fun HelpScreen(
    onReplayTour: () -> Unit,
    onBack: () -> Unit,
    modifier: Modifier = Modifier
) {
    ScreenScaffold(title = stringResource(R.string.help_title), modifier = modifier, onBack = onBack) {
        Column(
            modifier = Modifier
                .fillMaxSize()
                .verticalScroll(rememberScrollState())
                .padding(vertical = 12.dp),
            verticalArrangement = Arrangement.spacedBy(10.dp)
        ) {
            topics.forEach { topic ->
                Card(
                    modifier = Modifier
                        .fillMaxWidth()
                        .testTag("help-topic"),
                    colors = CardDefaults.cardColors(
                        containerColor = MaterialTheme.colorScheme.surfaceVariant
                    )
                ) {
                    Column(Modifier.padding(14.dp)) {
                        Text(stringResource(topic.titleRes), style = MaterialTheme.typography.titleMedium)
                        Text(
                            stringResource(topic.textRes),
                            style = MaterialTheme.typography.bodyMedium,
                            color = MaterialTheme.colorScheme.onSurfaceVariant,
                            modifier = Modifier.padding(top = 4.dp)
                        )
                    }
                }
            }
            Button(
                onClick = onReplayTour,
                modifier = Modifier.fillMaxWidth()
            ) { Text(stringResource(R.string.action_show_tour)) }
        }
    }
}
