package com.trefas.asledger.util

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

/** Деньги: копейки без float, русский формат и устойчивый разбор. */
class MoneyFormatTest {
    @Test
    fun `формат сумм с пробелами и знаком минус`() {
        assertEquals("0,00 ₽", formatKop(0))
        assertEquals("0,05 ₽", formatKop(5))
        assertEquals("12 345,67 ₽", formatKop(1_234_567))
        assertEquals("−1,00 ₽", formatKop(-100))
        assertEquals("−1 000 000,00 ₽", formatKop(-100_000_000))
    }

    @Test
    fun `разбор принимает запятую точку и пробелы`() {
        assertEquals(1_234_567L, parseRubles("12 345,67"))
        assertEquals(1_234_567L, parseRubles("12345.67"))
        assertEquals(100L, parseRubles("1"))
        assertEquals(150L, parseRubles("1,5"))
        assertEquals(0L, parseRubles("0"))
        assertEquals(50L, parseRubles(",5"))
    }

    @Test
    fun `некорректные и отрицательные суммы отвергаются`() {
        assertNull(parseRubles(""))
        assertNull(parseRubles("  "))
        assertNull(parseRubles("-1"))
        assertNull(parseRubles("1,234"))
        assertNull(parseRubles("12,3,4"))
        assertNull(parseRubles("рубль"))
        assertNull(parseRubles("999999999999999999999"))
    }

    @Test
    fun `переполнение при разборе не превращается в отрицательное число`() {
        assertNull(parseRubles("92233720368547758,08"))
    }
}
