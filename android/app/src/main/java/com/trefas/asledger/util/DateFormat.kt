package com.trefas.asledger.util

import java.time.Instant
import java.time.ZoneId
import java.time.format.DateTimeFormatter

private val SyncTimeFormatter = DateTimeFormatter
    .ofPattern("dd.MM.yyyy HH:mm")
    .withZone(ZoneId.systemDefault())

/** Время последней синхронизации для экрана диагностики. */
fun formatSyncTime(epochMillis: Long): String {
    if (epochMillis <= 0) {
        return "ещё не было"
    }
    return SyncTimeFormatter.format(Instant.ofEpochMilli(epochMillis))
}
