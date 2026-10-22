package com.trefas.asledger.ui.theme

import androidx.compose.material3.ColorScheme
import androidx.compose.ui.graphics.Color
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * Контраст текста и акцентов по WCAG AA.
 *
 * Для обычного текста требуется 4,5:1, для крупного текста и элементов
 * управления — 3:1. Палитра не зависит от динамических цветов Android, поэтому
 * проверка детерминирована.
 */
class ContrastTest {
    @Test
    fun `светлая тема проходит WCAG AA`() {
        assertScheme(LightColors)
    }

    @Test
    fun `тёмная тема проходит WCAG AA`() {
        assertScheme(DarkColors)
    }

    private fun assertScheme(scheme: ColorScheme) {
        val textPairs = listOf(
            "onBackground/background" to (scheme.onBackground to scheme.background),
            "onSurface/surface" to (scheme.onSurface to scheme.surface),
            "onSurfaceVariant/surface" to (scheme.onSurfaceVariant to scheme.surface),
            "onSurfaceVariant/surfaceVariant" to
                (scheme.onSurfaceVariant to scheme.surfaceVariant),
            "onPrimary/primary" to (scheme.onPrimary to scheme.primary),
            "onPrimaryContainer/primaryContainer" to
                (scheme.onPrimaryContainer to scheme.primaryContainer),
            "onSecondary/secondary" to (scheme.onSecondary to scheme.secondary),
            "onSecondaryContainer/secondaryContainer" to
                (scheme.onSecondaryContainer to scheme.secondaryContainer),
            "onTertiary/tertiary" to (scheme.onTertiary to scheme.tertiary),
            "onTertiaryContainer/tertiaryContainer" to
                (scheme.onTertiaryContainer to scheme.tertiaryContainer),
            "tertiary/background" to (scheme.tertiary to scheme.background),
            "tertiary/surface" to (scheme.tertiary to scheme.surface),
            "onError/error" to (scheme.onError to scheme.error)
        )
        textPairs.forEach { (name, pair) ->
            assertContrast(name, pair.first, pair.second, 4.5)
        }

        val accentPairs = listOf(
            "primary/background" to (scheme.primary to scheme.background),
            "secondary/background" to (scheme.secondary to scheme.background),
            "tertiary/background" to (scheme.tertiary to scheme.background),
            "error/background" to (scheme.error to scheme.background),
            "primary/surface" to (scheme.primary to scheme.surface),
            "secondary/surface" to (scheme.secondary to scheme.surface)
        )
        accentPairs.forEach { (name, pair) ->
            assertContrast(name, pair.first, pair.second, 3.0)
        }
    }

    private fun assertContrast(
        name: String,
        foreground: Color,
        background: Color,
        minimum: Double
    ) {
        val actual = contrast(foreground, background)
        assertTrue(
            "$name: контраст ${"%.2f".format(actual)}, требуется не ниже $minimum",
            actual >= minimum
        )
    }

    private fun contrast(first: Color, second: Color): Double {
        val a = luminance(first)
        val b = luminance(second)
        return (maxOf(a, b) + 0.05) / (minOf(a, b) + 0.05)
    }

    private fun luminance(color: Color): Double {
        fun channel(value: Float): Double {
            val c = value.toDouble()
            return if (c <= 0.04045) {
                c / 12.92
            } else {
                Math.pow((c + 0.055) / 1.055, 2.4)
            }
        }
        return 0.2126 * channel(color.red) +
            0.7152 * channel(color.green) +
            0.0722 * channel(color.blue)
    }
}
