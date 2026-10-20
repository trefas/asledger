package com.trefas.asledger.ui.navigation

import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.List
import androidx.compose.material.icons.filled.Add
import androidx.compose.material.icons.filled.Home
import androidx.compose.material.icons.filled.Settings
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.Icon
import androidx.compose.material3.NavigationBar
import androidx.compose.material3.NavigationBarItem
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.produceState
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.unit.dp
import androidx.navigation.NavGraph.Companion.findStartDestination
import androidx.navigation.compose.NavHost
import androidx.navigation.compose.composable
import androidx.navigation.compose.currentBackStackEntryAsState
import androidx.navigation.compose.rememberNavController
import com.trefas.asledger.data.AppContainer
import com.trefas.asledger.data.settings.ServerConfig
import com.trefas.asledger.ui.components.BrandMark
import com.trefas.asledger.ui.screens.AccountsScreen
import com.trefas.asledger.ui.screens.EntryScreen
import com.trefas.asledger.ui.screens.JournalScreen
import com.trefas.asledger.ui.screens.SettingsScreen
import com.trefas.asledger.ui.screens.SetupScreen
import kotlinx.coroutines.flow.collect
import kotlinx.coroutines.launch

/** Маршруты приложения. */
object Routes {
    const val SETUP = "setup"
    const val JOURNAL = "journal"
    const val ENTRY = "entry"
    const val ACCOUNTS = "accounts"
    const val SETTINGS = "settings"
}

/** Состояние загрузки адреса сервера. */
private sealed interface ConfigState {
    data object Loading : ConfigState
    data object Missing : ConfigState
    data class Ready(val config: ServerConfig) : ConfigState
}

private data class TopLevelDestination(
    val route: String,
    val label: String,
    val icon: ImageVector,
)

private val topLevelDestinations = listOf(
    TopLevelDestination(Routes.JOURNAL, "Журнал", Icons.Filled.Home),
    TopLevelDestination(Routes.ENTRY, "Ввод", Icons.Filled.Add),
    TopLevelDestination(Routes.ACCOUNTS, "Счета", Icons.AutoMirrored.Filled.List),
    TopLevelDestination(Routes.SETTINGS, "Настройки", Icons.Filled.Settings),
)

/**
 * Корневая навигация.
 *
 * Пока DataStore не прочитан, показывается заставка. Если адрес не сохранён,
 * стартовый экран — подключение; иначе журнал.
 */
@Composable
fun AsledgerNavHost(
    container: AppContainer,
    modifier: Modifier = Modifier,
) {
    val state by produceState<ConfigState>(
        initialValue = ConfigState.Loading,
        key1 = container,
    ) {
        container.serverConfig.config.collect { saved ->
            value = if (saved == null) ConfigState.Missing else ConfigState.Ready(saved)
        }
    }

    when (val current = state) {
        ConfigState.Loading -> LoadingScreen(modifier)
        else -> MainNavigation(container = container, state = current, modifier = modifier)
    }
}

@Composable
private fun LoadingScreen(modifier: Modifier = Modifier) {
    Box(modifier = modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
        Column(horizontalAlignment = Alignment.CenterHorizontally) {
            BrandMark(size = 96.dp)
            Spacer(Modifier.height(24.dp))
            CircularProgressIndicator()
        }
    }
}

@Composable
private fun MainNavigation(
    container: AppContainer,
    state: ConfigState,
    modifier: Modifier = Modifier,
) {
    val navController = rememberNavController()
    val backStack by navController.currentBackStackEntryAsState()
    val currentRoute = backStack?.destination?.route
    val scope = rememberCoroutineScope()

    // Стартовый маршрут фиксируем один раз: после сохранения адреса NavHost
    // не пересоздаётся, а переход выполняет экран подключения.
    val startDestination = remember {
        if (state is ConfigState.Ready) Routes.JOURNAL else Routes.SETUP
    }
    val config = (state as? ConfigState.Ready)?.config

    Scaffold(
        modifier = modifier.fillMaxSize(),
        contentWindowInsets = WindowInsets(0, 0, 0, 0),
        bottomBar = {
            if (currentRoute != null && currentRoute != Routes.SETUP) {
                NavigationBar {
                    topLevelDestinations.forEach { item ->
                        NavigationBarItem(
                            selected = currentRoute == item.route,
                            onClick = {
                                navController.navigate(item.route) {
                                    popUpTo(navController.graph.findStartDestination().id) {
                                        saveState = true
                                    }
                                    launchSingleTop = true
                                    restoreState = true
                                }
                            },
                            icon = { Icon(item.icon, contentDescription = null) },
                            label = { Text(item.label) },
                        )
                    }
                }
            }
        },
    ) { padding ->
        NavHost(
            navController = navController,
            startDestination = startDestination,
            modifier = Modifier.padding(padding),
        ) {
            composable(Routes.SETUP) {
                SetupScreen(
                    onSave = { saved -> container.serverConfig.save(saved) },
                    onConnected = {
                        navController.navigate(Routes.JOURNAL) {
                            popUpTo(Routes.SETUP) { inclusive = true }
                            launchSingleTop = true
                        }
                    },
                )
            }
            composable(Routes.JOURNAL) {
                JournalScreen()
            }
            composable(Routes.ENTRY) {
                EntryScreen()
            }
            composable(Routes.ACCOUNTS) {
                AccountsScreen()
            }
            composable(Routes.SETTINGS) {
                SettingsScreen(
                    config = config,
                    onChangeServer = {
                        scope.launch {
                            container.serverConfig.clear()
                            navController.navigate(Routes.SETUP) {
                                popUpTo(navController.graph.findStartDestination().id) {
                                    inclusive = true
                                }
                                launchSingleTop = true
                            }
                        }
                    },
                )
            }
        }
    }
}
