package com.trefas.asledger.ui.components

import androidx.compose.runtime.Composable
import androidx.compose.ui.res.stringResource
import com.trefas.asledger.R

/**
 * Подпись типа операции.
 *
 * Значения `трата`, `доход` и `перевод` — протокол сервера и ключи локальной
 * базы, поэтому переводится только их отображение.
 */
@Composable
fun operationTypeLabel(opType: String): String = when (opType) {
    "трата" -> stringResource(R.string.op_type_expense)
    "доход" -> stringResource(R.string.op_type_income)
    "перевод" -> stringResource(R.string.op_type_transfer)
    else -> opType.replaceFirstChar { it.uppercase() }
}
