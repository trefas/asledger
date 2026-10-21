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
