package com.trefas.asledger.ui.navigation

import androidx.annotation.StringRes
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
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.res.stringResource
import com.trefas.asledger.R

/** Пункт нижней навигации. */
internal data class TopLevelDestination(
    val route: String,
    @StringRes val labelRes: Int,
    val icon: ImageVector
)

internal val topLevelDestinations = listOf(
    TopLevelDestination(Routes.JOURNAL, R.string.nav_journal, Icons.Filled.Home),
    TopLevelDestination(Routes.ENTRY, R.string.nav_entry, Icons.Filled.Add),
    TopLevelDestination(Routes.ACCOUNTS, R.string.nav_accounts, Icons.AutoMirrored.Filled.List),
    TopLevelDestination(Routes.SETTINGS, R.string.nav_settings, Icons.Filled.Settings)
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
    // При крупном системном шрифте подписи не помещаются в четыре колонки.
    // Оставляем только иконки, а название раздела переносим в описание для
    // TalkBack: интерфейс не ломается, доступность не страдает.
    val labelsVisible = LocalDensity.current.fontScale < 1.5f
    NavigationBar {
        topLevelDestinations.forEach { item ->
            val label = stringResource(item.labelRes)
            NavigationBarItem(
                selected = currentRoute == item.route,
                onClick = { onNavigate(item.route) },
                icon = {
                    Icon(
                        imageVector = item.icon,
                        contentDescription = if (labelsVisible) null else label
                    )
                },
                label = if (labelsVisible) {
                    { Text(label, maxLines = 1) }
                } else {
                    null
                }
            )
        }
    }
}
