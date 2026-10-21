package com.trefas.asledger.util

import java.time.Instant
import java.time.OffsetDateTime
import java.time.ZoneId
import java.time.format.DateTimeFormatter

private val DateFormatter = DateTimeFormatter
    .ofPattern("dd.MM.yyyy")
    .withZone(ZoneId.systemDefault())

private val DateTimeFormatterFull = DateTimeFormatter
    .ofPattern("dd.MM.yyyy HH:mm")
    .withZone(ZoneId.systemDefault())

/**
 * Разбирает время сервера в миллисекунды Unix.
 *
 * Go отдаёт RFC 3339 с `Z` или смещением; `OffsetDateTime` покрывает оба
 * варианта. Некорректная дата — ошибка снапшота, а не тихо подставленный ноль.
 */
fun parseServerTime(value: String): Long {
    if (value.isBlank()) {
        throw IllegalArgumentException("пустая дата в снапшоте")
    }
    val instant = runCatching { OffsetDateTime.parse(value).toInstant() }
        .getOrElse {
            runCatching { Instant.parse(value) }
                .getOrElse { throw IllegalArgumentException("некорректная дата: $value") }
        }
    return instant.toEpochMilli()
}

/** Дата проводки для журнала. */
fun formatDate(epochMillis: Long): String =
    if (epochMillis <= 0) "" else DateFormatter.format(Instant.ofEpochMilli(epochMillis))

/** Дата и время для деталей. */
fun formatDateTime(epochMillis: Long): String =
    if (epochMillis <= 0) "" else DateTimeFormatterFull.format(Instant.ofEpochMilli(epochMillis))
