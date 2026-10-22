package com.trefas.asledger.ui.theme

import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Shapes
import androidx.compose.material3.darkColorScheme
import androidx.compose.material3.lightColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.ui.unit.dp

internal val LightColors = lightColorScheme(
    primary = Green700,
    onPrimary = Card,
    primaryContainer = Green100,
    onPrimaryContainer = Green900,
    secondary = Green500,
    onSecondary = Card,
    secondaryContainer = Green100,
    onSecondaryContainer = Green900,
    tertiary = Olive,
    onTertiary = Card,
    tertiaryContainer = Green100,
    onTertiaryContainer = Green900,
    background = Background,
    onBackground = Ink,
    surface = Card,
    onSurface = Ink,
    surfaceVariant = Green100,
    onSurfaceVariant = Muted,
    outline = Line,
    outlineVariant = Line,
    error = Danger,
    onError = Card,
    errorContainer = DangerBg,
    onErrorContainer = Danger
)

internal val DarkColors = darkColorScheme(
    primary = Lime,
    onPrimary = Green900,
    primaryContainer = Green700,
    onPrimaryContainer = Green100,
    secondary = Green500,
    onSecondary = Card,
    secondaryContainer = DarkSurfaceVariant,
    onSecondaryContainer = DarkOnSurface,
    tertiary = LimeDark,
    onTertiary = Green900,
    tertiaryContainer = Green700,
    onTertiaryContainer = Green100,
    background = DarkBackground,
    onBackground = DarkOnSurface,
    surface = DarkSurface,
    onSurface = DarkOnSurface,
    surfaceVariant = DarkSurfaceVariant,
    onSurfaceVariant = DarkOnSurfaceVariant,
    outline = DarkOutline,
    outlineVariant = DarkOutline,
    error = DarkError,
    onError = DarkOnError
)

private val AsledgerShapes = Shapes(
    extraSmall = RoundedCornerShape(6.dp),
    small = RoundedCornerShape(8.dp),
    medium = RoundedCornerShape(10.dp),
    large = RoundedCornerShape(14.dp),
    extraLarge = RoundedCornerShape(20.dp)
)

/**
 * Тема приложения.
 *
 * Динамические цвета отключены: бренд должен совпадать с ПК-версией на всех
 * устройствах.
 */
@Composable
fun AsledgerTheme(
    darkTheme: Boolean = isSystemInDarkTheme(),
    content: @Composable () -> Unit
) {
    MaterialTheme(
        colorScheme = if (darkTheme) DarkColors else LightColors,
        typography = AsledgerTypography,
        shapes = AsledgerShapes,
        content = content
    )
}
