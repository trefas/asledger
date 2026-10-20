package com.trefas.asledger.ui.navigation

import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.List
import androidx.compose.material.icons.filled.Add
import androidx.compose.material.icons.filled.Home
import androidx.compose.material.icons.filled.Settings
import androidx.compose.material3.Icon
import androidx.compose.material3.NavigationBar
import androidx.compose.material3.NavigationBarItem
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.graphics.vector.ImageVector

/** Пункт нижней навигации. */
internal data class TopLevelDestination(
    val route: String,
    val label: String,
    val icon: ImageVector
)

internal val topLevelDestinations = listOf(
    TopLevelDestination(Routes.JOURNAL, "Журнал", Icons.Filled.Home),
    TopLevelDestination(Routes.ENTRY, "Ввод", Icons.Filled.Add),
    TopLevelDestination(Routes.ACCOUNTS, "Счета", Icons.AutoMirrored.Filled.List),
    TopLevelDestination(Routes.SETTINGS, "Настройки", Icons.Filled.Settings)
)

/**
 * Нижняя навигация по основным разделам.
 *
 * Вынесена отдельно, чтобы её можно было проверить Compose-тестом без запуска
 * всего NavHost и DataStore.
 */
@Composable
fun AsledgerBottomBar(
    currentRoute: String?,
    onNavigate: (String) -> Unit
) {
    NavigationBar {
        topLevelDestinations.forEach { item ->
            NavigationBarItem(
                selected = currentRoute == item.route,
                onClick = { onNavigate(item.route) },
                icon = { Icon(item.icon, contentDescription = null) },
                label = { Text(item.label) }
            )
        }
    }
}
