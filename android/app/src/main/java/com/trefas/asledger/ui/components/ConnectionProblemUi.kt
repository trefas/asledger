package com.trefas.asledger.ui.components

import androidx.compose.runtime.Composable
import androidx.compose.ui.res.stringResource
import com.trefas.asledger.data.network.ConnectionDetail
import com.trefas.asledger.data.network.ConnectionProblem

/** Заголовок ошибки подключения из ресурсов. */
@Composable
fun ConnectionProblem.titleText(): String = stringResource(titleRes)

/** Подсказка ошибки подключения: ресурс с аргументами или динамический текст. */
@Composable
fun ConnectionProblem.hintText(): String = when (val value = hint) {
    is ConnectionDetail.Resource -> stringResource(value.id, *value.args.toTypedArray())
    is ConnectionDetail.Text -> value.value
}
