package com.trefas.asledger.util

import java.util.Locale
import kotlin.math.abs

/** Форматирует копейки как «1 234,56 ₽» без чисел с плавающей точкой. */
fun formatKop(kopecks: Long): String {
    if (kopecks == 0L) {
        return "0,00 ₽"
    }
    val sign = if (kopecks < 0) "−" else ""
    val value = abs(kopecks)
    val rubles = value / 100
    val kop = value % 100
    return sign + groupDigits(rubles) + "," + String.format(Locale.ROOT, "%02d", kop) + " ₽"
}

/**
 * Разбирает сумму в рублях в копейки без чисел с плавающей точкой.
 *
 * Принимает запятую или точку, одну или две цифры после разделителя.
 * Возвращает null для пустой или некорректной суммы.
 */
fun parseRubles(raw: String): Long? {
    val normalized = raw.trim()
        .replace(" ", "")
        .replace("\u00A0", "")
        .replace('\u2212', '-')
    if (normalized.isEmpty() || normalized.startsWith('-')) return null
    val parts = normalized.split(',', '.')
    if (parts.size > 2) return null
    val rubles = parts[0].ifEmpty { "0" }
    if (!rubles.all { it.isDigit() }) return null
    val kop = parts.getOrNull(1).orEmpty()
    if (kop.isNotEmpty() && (kop.length > 2 || !kop.all { it.isDigit() })) return null
    val rubleValue = rubles.toLongOrNull() ?: return null
    val kopValue = kop.padEnd(2, '0').toLongOrNull() ?: 0
    return runCatching {
        Math.addExact(Math.multiplyExact(rubleValue, 100), kopValue)
    }.getOrNull()
}

private fun groupDigits(value: Long): String {
    val text = value.toString()
    if (text.length <= 3) {
        return text
    }
    val first = text.length % 3
    val result = StringBuilder()
    if (first > 0) {
        result.append(text, 0, first)
    }
    var index = first
    while (index < text.length) {
        if (result.isNotEmpty()) {
            result.append(' ')
        }
        result.append(text, index, index + 3)
        index += 3
    }
    return result.toString()
}
