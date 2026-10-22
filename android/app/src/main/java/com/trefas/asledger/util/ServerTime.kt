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
        throw IllegalArgumentException("empty date in snapshot")
    }
    val instant = runCatching { OffsetDateTime.parse(value).toInstant() }
        .getOrElse {
            runCatching { Instant.parse(value) }
                .getOrElse { throw IllegalArgumentException("invalid date: $value") }
        }
    return instant.toEpochMilli()
}

/** Дата проводки для журнала. */
fun formatDate(epochMillis: Long): String =
    if (epochMillis <= 0) "" else DateFormatter.format(Instant.ofEpochMilli(epochMillis))

/** Относительная дата для списка: сегодня, вчера или обычная дата. */
sealed interface FriendlyDate {
    data object Today : FriendlyDate
    data object Yesterday : FriendlyDate
    data class Absolute(val text: String) : FriendlyDate
}

/**
 * Дата для списка: «сегодня», «вчера» или `дд.мм.гггг`.
 *
 * [nowMillis] и [zone] передаются в тестах; в приложении — текущее время и
 * часовой пояс устройства.
 */
fun friendlyDate(
    epochMillis: Long,
    nowMillis: Long = System.currentTimeMillis(),
    zone: ZoneId = ZoneId.systemDefault()
): FriendlyDate {
    if (epochMillis <= 0) return FriendlyDate.Absolute("")
    val date = Instant.ofEpochMilli(epochMillis).atZone(zone).toLocalDate()
    val today = Instant.ofEpochMilli(nowMillis).atZone(zone).toLocalDate()
    return when (date) {
        today -> FriendlyDate.Today
        today.minusDays(1) -> FriendlyDate.Yesterday
        else -> FriendlyDate.Absolute(
            DateFormatter.withZone(zone).format(Instant.ofEpochMilli(epochMillis))
        )
    }
}

/** Дата и время для деталей. */
fun formatDateTime(epochMillis: Long): String =
    if (epochMillis <= 0) "" else DateTimeFormatterFull.format(Instant.ofEpochMilli(epochMillis))
