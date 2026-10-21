package com.trefas.asledger.ui.screens

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
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import com.trefas.asledger.ui.components.BrandMark

private data class OnboardingStep(
    val title: String,
    val text: String,
    val hint: String
)

private val steps = listOf(
    OnboardingStep(
        title = "Книга всегда под рукой",
        text = "asledger — семейная книга в локальной сети. Сервер хранит общую " +
            "историю, а телефон работает даже без связи.",
        hint = "Адрес сервера вы уже указали. Осталось познакомиться с главным."
    ),
    OnboardingStep(
        title = "План счетов",
        text = "На экране «Счета» — дерево счетов, остатки и поиск. Нажмите на " +
            "листовой счёт, чтобы сразу начать с него проводку.",
        hint = "План счетов меняется на сервере, в телефоне он доступен для просмотра."
    ),
    OnboardingStep(
        title = "Проводки без сети",
        text = "Выберите тип, счета и суммы в дебете и кредите. Сторону подскажет " +
            "матрица ролей сервера, а черновик сохранится на устройстве.",
        hint = "Проводка отправится автоматически, когда появится связь."
    ),
    OnboardingStep(
        title = "Синхронизация и порядок",
        text = "В журнале видно, что ждёт отправки, что отклонено и где возможен " +
            "дубль. Все действия можно повторить, исправить или отменить сторно.",
        hint = "Журнал фильтруется по дате, счёту, типу и статусу."
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
            ) { Text("Пропустить") }
        }
        Spacer(Modifier.weight(1f))
        BrandMark(size = 88.dp)
        Spacer(Modifier.height(28.dp))
        Text(
            text = step.title,
            style = MaterialTheme.typography.headlineSmall,
            textAlign = TextAlign.Center,
            modifier = Modifier.testTag("onboarding-title")
        )
        Spacer(Modifier.height(12.dp))
        Text(
            text = step.text,
            style = MaterialTheme.typography.bodyLarge,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
            textAlign = TextAlign.Center
        )
        Spacer(Modifier.height(16.dp))
        Text(
            text = step.hint,
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
            Text(if (index == steps.lastIndex) "Начать" else "Далее")
        }
    }
}
