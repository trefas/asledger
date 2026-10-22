package com.trefas.asledger.util

import java.time.LocalDate
import java.time.ZoneId
import java.time.ZoneOffset
import java.time.ZonedDateTime
import java.util.TimeZone
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertThrows
import org.junit.Before
import org.junit.Test

/** Даты: серверный RFC 3339, русский формат и «сегодня»/«вчера». */
class ServerTimeTest {
    private val zone = ZoneId.of("Europe/Moscow")
    private lateinit var previousZone: TimeZone

    @Before
    fun useMoscow() {
        previousZone = TimeZone.getDefault()
        TimeZone.setDefault(TimeZone.getTimeZone(zone))
    }

    @After
    fun restoreZone() {
        TimeZone.setDefault(previousZone)
    }

    private fun millis(date: LocalDate, hour: Int = 12): Long =
        ZonedDateTime.of(date.atTime(hour, 0), zone).toInstant().toEpochMilli()

    @Test
    fun `разбор серверного времени с Z и смещением`() {
        assertEquals(
            1_700_000_000_000L,
            parseServerTime("2023-11-14T22:13:20Z")
        )
        assertEquals(
            parseServerTime("2023-11-14T22:13:20Z"),
            parseServerTime("2023-11-15T01:13:20+03:00")
        )
    }

    @Test
    fun `некорректная дата падает явно`() {
        assertThrows(IllegalArgumentException::class.java) {
            parseServerTime("")
        }
        assertThrows(IllegalArgumentException::class.java) {
            parseServerTime("вчера")
        }
    }

    @Test
    fun `формат даты и времени фиксирован`() {
        val value = ZonedDateTime.of(2026, 10, 21, 9, 5, 0, 0, zone)
            .toInstant().toEpochMilli()
        assertEquals("21.10.2026", formatDate(value))
        assertEquals("21.10.2026 09:05", formatDateTime(value))
        assertEquals("", formatDate(0))
        assertEquals("", formatDateTime(-1))
    }

    @Test
    fun `сегодня и вчера считаются по часовому поясу`() {
        val now = millis(LocalDate.of(2026, 10, 21))
        assertEquals(FriendlyDate.Today, friendlyDate(now, now, zone))
        assertEquals(
            FriendlyDate.Yesterday,
            friendlyDate(millis(LocalDate.of(2026, 10, 20)), now, zone)
        )
        assertEquals(
            FriendlyDate.Absolute("19.10.2026"),
            friendlyDate(millis(LocalDate.of(2026, 10, 19)), now, zone)
        )
        assertEquals(FriendlyDate.Absolute(""), friendlyDate(0, now, zone))
    }

    @Test
    fun `граница суток не зависит от UTC`() {
        val now = ZonedDateTime.of(2026, 10, 21, 0, 30, 0, 0, zone)
            .toInstant().toEpochMilli()
        val yesterday = ZonedDateTime.of(2026, 10, 20, 23, 50, 0, 0, zone)
            .toInstant().toEpochMilli()
        assertEquals(FriendlyDate.Yesterday, friendlyDate(yesterday, now, zone))
        // В UTC это ещё тот же календарный день, что и у «сейчас».
        assertEquals(
            FriendlyDate.Today,
            friendlyDate(yesterday, now, ZoneOffset.UTC)
        )
    }
}
