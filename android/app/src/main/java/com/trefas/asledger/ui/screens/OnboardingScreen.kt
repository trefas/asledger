package com.trefas.asledger.ui.screens

import androidx.annotation.StringRes
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.material3.Button
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import com.trefas.asledger.R
import com.trefas.asledger.ui.components.BrandMark

private data class OnboardingStep(
    @StringRes val titleRes: Int,
    @StringRes val textRes: Int,
    @StringRes val hintRes: Int
)

private val steps = listOf(
    OnboardingStep(
        titleRes = R.string.onboarding_book_title,
        textRes = R.string.onboarding_book_text,
        hintRes = R.string.onboarding_book_hint
    ),
    OnboardingStep(
        titleRes = R.string.onboarding_accounts_title,
        textRes = R.string.onboarding_accounts_text,
        hintRes = R.string.onboarding_accounts_hint
    ),
    OnboardingStep(
        titleRes = R.string.onboarding_entry_title,
        textRes = R.string.onboarding_entry_text,
        hintRes = R.string.onboarding_entry_hint
    ),
    OnboardingStep(
        titleRes = R.string.onboarding_sync_title,
        textRes = R.string.onboarding_sync_text,
        hintRes = R.string.onboarding_sync_hint
    )
)

/** Короткий тур по приложению при первом запуске. */
@Composable
fun OnboardingScreen(
    onDone: () -> Unit,
    modifier: Modifier = Modifier
) {
    var index by rememberSaveable { mutableIntStateOf(0) }
    val step = steps[index]

    Column(
        modifier = modifier
            .fillMaxSize()
            .padding(24.dp),
        horizontalAlignment = Alignment.CenterHorizontally
    ) {
        Row(
            modifier = Modifier.fillMaxWidth(),
            horizontalArrangement = Arrangement.End
        ) {
            TextButton(
                onClick = onDone,
                modifier = Modifier.testTag("onboarding-skip")
            ) { Text(stringResource(R.string.action_skip)) }
        }
        Spacer(Modifier.weight(1f))
        BrandMark(size = 88.dp)
        Spacer(Modifier.height(28.dp))
        Text(
            text = stringResource(step.titleRes),
            style = MaterialTheme.typography.headlineSmall,
            textAlign = TextAlign.Center,
            modifier = Modifier.testTag("onboarding-title")
        )
        Spacer(Modifier.height(12.dp))
        Text(
            text = stringResource(step.textRes),
            style = MaterialTheme.typography.bodyLarge,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
            textAlign = TextAlign.Center
        )
        Spacer(Modifier.height(16.dp))
        Text(
            text = stringResource(step.hintRes),
            style = MaterialTheme.typography.bodyMedium,
            color = MaterialTheme.colorScheme.primary,
            textAlign = TextAlign.Center
        )
        Spacer(Modifier.weight(1f))
        Row(
            horizontalArrangement = Arrangement.spacedBy(8.dp),
            modifier = Modifier.padding(bottom = 20.dp)
        ) {
            steps.indices.forEach { dot ->
                Box(
                    modifier = Modifier
                        .size(if (dot == index) 10.dp else 8.dp)
                        .background(
                            color = if (dot == index) {
                                MaterialTheme.colorScheme.primary
                            } else {
                                MaterialTheme.colorScheme.outlineVariant
                            },
                            shape = CircleShape
                        )
                )
            }
        }
        Button(
            onClick = {
                if (index == steps.lastIndex) {
                    onDone()
                } else {
                    index += 1
                }
            },
            modifier = Modifier
                .fillMaxWidth()
                .testTag("onboarding-next")
        ) {
            Text(
                stringResource(
                    if (index == steps.lastIndex) R.string.action_start else R.string.action_next
                )
            )
        }
    }
}
