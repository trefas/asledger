package com.trefas.asledger.ui.components

import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.widthIn
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Info
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.runtime.staticCompositionLocalOf
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import com.trefas.asledger.R
import com.trefas.asledger.data.settings.HintStore

/** Хранилище отметок о подсказках; null в тестах и превью отключает автопоказ. */
val LocalHintStore = staticCompositionLocalOf<HintStore?> { null }

/**
 * Кнопка с подсказкой. Открывается по нажатию, закрывается кнопкой «Понятно»
 * или нажатием вне меню и не блокирует ввод.
 */
@Composable
fun HintIcon(
    text: String,
    modifier: Modifier = Modifier,
    contentDescription: String? = null
) {
    var expanded by remember { mutableStateOf(false) }
    val hintDescription = contentDescription
        ?: stringResource(R.string.hint_content_description)
    Box(modifier = modifier) {
        IconButton(
            onClick = { expanded = true },
            modifier = Modifier
                .size(40.dp)
                .testTag("hint-button")
        ) {
            Icon(
                imageVector = Icons.Filled.Info,
                contentDescription = hintDescription,
                tint = MaterialTheme.colorScheme.onSurfaceVariant
            )
        }
        DropdownMenu(
            expanded = expanded,
            onDismissRequest = { expanded = false },
            modifier = Modifier.widthIn(max = 320.dp)
        ) {
            Column(Modifier.padding(horizontal = 16.dp, vertical = 8.dp)) {
                Text(
                    text = text,
                    style = MaterialTheme.typography.bodyMedium,
                    modifier = Modifier.testTag("hint-text")
                )
                TextButton(
                    onClick = { expanded = false },
                    modifier = Modifier
                        .align(Alignment.End)
                        .padding(top = 4.dp)
                ) {
                    Text(stringResource(R.string.action_understand))
                }
            }
        }
    }
}

/**
 * Показывает объяснение при первом открытии экрана и запоминает просмотр.
 * Без [LocalHintStore] ничего не показывает — так тесты не зависят от диалога.
 */
@Composable
fun FirstOpenHint(
    id: String,
    title: String,
    text: String
) {
    val store = LocalHintStore.current ?: return
    var visible by remember(id) { mutableStateOf(!store.wasSeen(id)) }
    if (!visible) return

    fun dismiss() {
        store.markSeen(id)
        visible = false
    }

    AlertDialog(
        onDismissRequest = { dismiss() },
        title = { Text(title) },
        text = {
            Text(
                text = text,
                modifier = Modifier.testTag("first-open-hint")
            )
        },
        confirmButton = {
            TextButton(onClick = { dismiss() }) {
                Text(stringResource(R.string.action_understand))
            }
        }
    )
}
