package com.trefas.asledger.ui.navigation

import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.Scaffold
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.produceState
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import androidx.lifecycle.viewmodel.initializer
import androidx.lifecycle.viewmodel.viewModelFactory
import androidx.navigation.NavGraph.Companion.findStartDestination
import androidx.navigation.compose.NavHost
import androidx.navigation.compose.composable
import androidx.navigation.compose.currentBackStackEntryAsState
import androidx.navigation.compose.rememberNavController
import com.trefas.asledger.data.AppContainer
import com.trefas.asledger.data.network.ApiResult
import com.trefas.asledger.data.network.ConnectionError
import com.trefas.asledger.data.settings.ServerConfig
import com.trefas.asledger.ui.components.BrandMark
import com.trefas.asledger.ui.screens.AccountsScreen
import com.trefas.asledger.ui.screens.DiagnosticsScreen
import com.trefas.asledger.ui.screens.EnrollScreen
import com.trefas.asledger.ui.screens.EntryScreen
import com.trefas.asledger.ui.screens.JournalScreen
import com.trefas.asledger.ui.screens.LoadingScreen
import com.trefas.asledger.ui.screens.LoadingState
import com.trefas.asledger.ui.screens.LoadingViewModel
import com.trefas.asledger.ui.screens.SettingsScreen
import com.trefas.asledger.ui.screens.SetupScreen
import com.trefas.asledger.util.Log
import com.trefas.asledger.util.deviceName
import kotlinx.coroutines.flow.collect
import kotlinx.coroutines.launch

/** Маршруты приложения. */
object Routes {
    const val SETUP = "setup"
    const val ENROLL = "enroll"
    const val LOADING = "loading"
    const val JOURNAL = "journal"
    const val ENTRY = "entry"
    const val ACCOUNTS = "accounts"
    const val SETTINGS = "settings"
    const val DIAGNOSTICS = "diagnostics"
}

/** Состояние загрузки адреса сервера и привязки. */
private sealed interface ConfigState {
    data object Loading : ConfigState
    data object Missing : ConfigState
    data class EnrollRequired(val config: ServerConfig) : ConfigState
    data class Ready(val config: ServerConfig) : ConfigState
}

/**
 * Корневая навигация.
 *
 * Пока DataStore не прочитан, показывается заставка. Затем: нет адреса —
 * подключение, нет пользователя — привязка, иначе журнал.
 */
@Composable
fun AsledgerNavHost(
    container: AppContainer,
    modifier: Modifier = Modifier
) {
    val state by produceState<ConfigState>(
        initialValue = ConfigState.Loading,
        key1 = container
    ) {
        container.serverConfig.config.collect { saved ->
            value = when {
                saved == null -> ConfigState.Missing
                saved.hasSession -> ConfigState.Ready(saved)
                else -> ConfigState.EnrollRequired(saved)
            }
        }
    }
    val libraryLoaded by produceState<Boolean?>(
        initialValue = null,
        key1 = container
    ) {
        container.libraryLoaded.collect { value = it }
    }

    when {
        state is ConfigState.Loading -> SplashScreen(modifier)
        // Базу читаем параллельно с DataStore. Пустой ответ до первого
        // импорта — это не «нет данных», а ещё не завершённая загрузка.
        state is ConfigState.Ready && libraryLoaded == null -> SplashScreen(modifier)
        else -> MainNavigation(
            container = container,
            state = state,
            libraryLoaded = libraryLoaded == true,
            modifier = modifier
        )
    }
}

@Composable
private fun SplashScreen(modifier: Modifier = Modifier) {
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
    libraryLoaded: Boolean,
    modifier: Modifier = Modifier
) {
    val navController = rememberNavController()
    val backStack by navController.currentBackStackEntryAsState()
    val currentRoute = backStack?.destination?.route
    // Диагностика — вложенный экран настроек, поэтому подсвечиваем «Настройки».
    val selectedRoute = if (currentRoute == Routes.DIAGNOSTICS) {
        Routes.SETTINGS
    } else {
        currentRoute
    }
    val scope = rememberCoroutineScope()

    // Стартовый маршрут фиксируем один раз: после сохранения адреса или
    // привязки NavHost не пересоздаётся, переход выполняет экран.
    val startDestination = remember {
        when (state) {
            is ConfigState.Ready -> if (libraryLoaded) Routes.JOURNAL else Routes.LOADING
            is ConfigState.EnrollRequired -> Routes.ENROLL
            else -> Routes.SETUP
        }
    }
    val config = when (state) {
        is ConfigState.Ready -> state.config
        is ConfigState.EnrollRequired -> state.config
        else -> null
    }

    // 401 от сервера: сессия уже очищена контейнером, но локальные черновики
    // (в S11 — база Room) не трогаем. Переводим на повторную привязку.
    LaunchedEffect(navController, container) {
        container.sessionExpired.collect {
            if (navController.currentDestination?.route != Routes.ENROLL) {
                navController.navigate(Routes.ENROLL) {
                    popUpTo(navController.graph.findStartDestination().id) {
                        inclusive = true
                    }
                    launchSingleTop = true
                }
            }
        }
    }

    Scaffold(
        modifier = modifier.fillMaxSize(),
        contentWindowInsets = WindowInsets(0, 0, 0, 0),
        bottomBar = {
            if (currentRoute != null &&
                currentRoute != Routes.SETUP &&
                currentRoute != Routes.ENROLL &&
                currentRoute != Routes.LOADING
            ) {
                AsledgerBottomBar(
                    currentRoute = selectedRoute,
                    onNavigate = { route ->
                        navController.navigate(route) {
                            popUpTo(navController.graph.findStartDestination().id) {
                                saveState = true
                            }
                            launchSingleTop = true
                            restoreState = true
                        }
                    }
                )
            }
        }
    ) { padding ->
        NavHost(
            navController = navController,
            startDestination = startDestination,
            modifier = Modifier.padding(padding)
        ) {
            composable(Routes.SETUP) {
                SetupScreen(
                    initialHost = config?.host.orEmpty(),
                    initialPort = config?.port ?: ServerConfig.DEFAULT_PORT,
                    onSubmit = { candidate -> container.api.health(candidate) },
                    onSuccess = { candidate, health ->
                        container.serverConfig.saveServer(
                            host = candidate.host,
                            port = candidate.port,
                            serverVersion = health.version,
                            formatVersion = health.formatVersion
                        )
                        navController.navigate(Routes.ENROLL) {
                            popUpTo(Routes.SETUP) { inclusive = true }
                            launchSingleTop = true
                        }
                    }
                )
            }
            composable(Routes.ENROLL) {
                val enrollConfig = config
                if (enrollConfig != null) {
                    EnrollScreen(
                        config = enrollConfig,
                        onEnroll = { userId ->
                            container.api.enroll(
                                config = enrollConfig,
                                clientId = enrollConfig.clientId,
                                deviceName = deviceName(),
                                userId = userId
                            )
                        },
                        onEnrolled = { success ->
                            try {
                                container.tokenStore.save(success.token)
                                container.serverConfig.saveSession(
                                    userId = success.userId,
                                    userName = success.userName,
                                    // Снапшот ещё не загружен: S11 отметит время после pull.
                                    lastSyncAt = 0
                                )
                                null
                            } catch (failure: Exception) {
                                ConnectionError.Unexpected(
                                    failure.message ?: "не удалось сохранить токен"
                                )
                            }
                        },
                        onDone = {
                            navController.navigate(Routes.LOADING) {
                                popUpTo(navController.graph.findStartDestination().id) {
                                    inclusive = true
                                }
                                launchSingleTop = true
                            }
                        },
                        onChangeServer = {
                            navController.navigate(Routes.SETUP) {
                                popUpTo(Routes.ENROLL) { inclusive = true }
                                launchSingleTop = true
                            }
                        }
                    )
                } else {
                    LaunchedEffect(Unit) {
                        navController.navigate(Routes.SETUP) {
                            popUpTo(Routes.ENROLL) { inclusive = true }
                            launchSingleTop = true
                        }
                    }
                }
            }
            composable(Routes.LOADING) {
                val loadingConfig = config
                if (loadingConfig != null) {
                    val loadingViewModel: LoadingViewModel = viewModel(
                        key = "loading-${loadingConfig.clientId}",
                        factory = viewModelFactory {
                            initializer {
                                LoadingViewModel { onProgress ->
                                    container.libraryLoader.load(loadingConfig, onProgress)
                                }
                            }
                        }
                    )
                    val loadingState by loadingViewModel.state.collectAsStateWithLifecycle()
                    LaunchedEffect(loadingViewModel) { loadingViewModel.start() }
                    LaunchedEffect(loadingState) {
                        if (loadingState is LoadingState.Success) {
                            navController.navigate(Routes.JOURNAL) {
                                popUpTo(navController.graph.findStartDestination().id) {
                                    inclusive = true
                                }
                                launchSingleTop = true
                            }
                        }
                    }
                    LoadingScreen(
                        config = loadingConfig,
                        state = loadingState,
                        onRetry = loadingViewModel::retry,
                        onCancel = loadingViewModel::cancel,
                        onReenroll = {
                            scope.launch {
                                container.forgetSession()
                                navController.navigate(Routes.ENROLL) {
                                    popUpTo(navController.graph.findStartDestination().id) {
                                        inclusive = true
                                    }
                                    launchSingleTop = true
                                }
                            }
                        },
                        onChangeServer = {
                            scope.launch {
                                container.forgetSession()
                                navController.navigate(Routes.SETUP) {
                                    popUpTo(navController.graph.findStartDestination().id) {
                                        inclusive = true
                                    }
                                    launchSingleTop = true
                                }
                            }
                        }
                    )
                } else {
                    LaunchedEffect(Unit) {
                        navController.navigate(Routes.SETUP) {
                            popUpTo(Routes.LOADING) { inclusive = true }
                            launchSingleTop = true
                        }
                    }
                }
            }
            composable(Routes.JOURNAL) {
                JournalScreen(
                    onNewEntry = { navController.navigate(Routes.ENTRY) },
                    transactions = container.transactions,
                    userName = config?.userName.orEmpty()
                )
            }
            composable(Routes.ENTRY) {
                EntryScreen(userName = config?.userName.orEmpty())
            }
            composable(Routes.ACCOUNTS) {
                AccountsScreen(
                    accounts = container.accounts,
                    balances = container.balances
                )
            }
            composable(Routes.SETTINGS) {
                SettingsScreen(
                    config = config,
                    onDiagnostics = { navController.navigate(Routes.DIAGNOSTICS) },
                    onChangeUser = {
                        scope.launch {
                            val token = runCatching { container.tokenStore.load() }.getOrNull()
                            if (token != null && config != null) {
                                val result = container.api.revokeToken(config, token)
                                if (result is ApiResult.Failure) {
                                    Log.w(
                                        "session",
                                        "не удалось отозвать токен: ${result.error}"
                                    )
                                }
                            }
                            container.forgetSession()
                            navController.navigate(Routes.ENROLL) {
                                popUpTo(navController.graph.findStartDestination().id) {
                                    inclusive = true
                                }
                                launchSingleTop = true
                            }
                        }
                    },
                    onChangeServer = {
                        scope.launch {
                            container.forgetSession()
                            navController.navigate(Routes.SETUP) {
                                popUpTo(navController.graph.findStartDestination().id) {
                                    inclusive = true
                                }
                                launchSingleTop = true
                            }
                        }
                    },
                    onForgetServer = {
                        scope.launch {
                            container.forgetServer()
                            navController.navigate(Routes.SETUP) {
                                popUpTo(navController.graph.findStartDestination().id) {
                                    inclusive = true
                                }
                                launchSingleTop = true
                            }
                        }
                    }
                )
            }
            composable(Routes.DIAGNOSTICS) {
                DiagnosticsScreen(
                    config = config,
                    onCheck = {
                        if (config == null) {
                            ApiResult.Failure(
                                ConnectionError.InvalidAddress("сервер не настроен")
                            )
                        } else {
                            container.api.health(config)
                        }
                    },
                    onHealth = { health ->
                        container.serverConfig.saveServerInfo(
                            serverVersion = health.version,
                            formatVersion = health.formatVersion
                        )
                    }
                )
            }
        }
    }
}
