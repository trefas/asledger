package com.trefas.asledger.ui.screens

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
import androidx.compose.ui.unit.dp
import com.trefas.asledger.ui.components.ScreenScaffold

private data class HelpTopic(val title: String, val text: String)

private val topics = listOf(
    HelpTopic(
        "Почему не нужен пароль?",
        "Телефон привязывается к пользователю книги один раз, а сервер выдаёт " +
            "устройству отдельный токен. Токен хранится в защищённом хранилище " +
            "Android и не показывается в интерфейсе. Пароль книги вводится только " +
            "в web-интерфейсе сервера."
    ),
    HelpTopic(
        "Как создать проводку?",
        "Нажмите «Ввод», выберите тип операции и счета. Сторону строки задаёт " +
            "роль из матрицы ASL-2.2: приложению не нужно угадывать дебет и кредит. " +
            "Суммы должны совпадать; черновик сохранится даже без сети."
    ),
    HelpTopic(
        "Что означают статусы?",
        "«К отправке» — проводка ждёт сеть. «Отправляется» — пакет уже ушёл. " +
            "«Отклонено» — сервер вернул правило, которое нужно исправить. " +
            "«Синхронизировано» — проводка есть в книге сервера."
    ),
    HelpTopic(
        "Почему появился дубль?",
        "Если повторная отправка совпала по смыслу с уже существующей проводкой, " +
            "сервер помечает возможный дубль, но не удаляет данные. Проверьте " +
            "предупреждение в очереди и отмените лишнюю проводку сторно."
    ),
    HelpTopic(
        "Где хранятся данные?",
        "Основная книга — на вашем сервере asledgerd. Телефон хранит локальную " +
            "копию для чтения и офлайн-работы. Полная перезагрузка книги сохраняет " +
            "неотправленные черновики."
    ),
    HelpTopic(
        "Доступность",
        "Все кнопки имеют текстовые подписи, суммы озвучиваются с валютой, " +
            "а размер шрифта следует системным настройкам Android. Если что-то " +
            "неудобно, опишите проблему в GitHub-репозитории проекта."
    )
)

/** Справка и повторный онбординг. */
@Composable
fun HelpScreen(
    onReplayTour: () -> Unit,
    onBack: () -> Unit,
    modifier: Modifier = Modifier
) {
    ScreenScaffold(title = "Справка", modifier = modifier, onBack = onBack) {
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
                        Text(topic.title, style = MaterialTheme.typography.titleMedium)
                        Text(
                            topic.text,
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
            ) { Text("Показать тур ещё раз") }
        }
    }
}
