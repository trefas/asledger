package com.trefas.asledger.ui.screens

import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import com.trefas.asledger.data.network.ConnectionError
import com.trefas.asledger.data.network.toProblem
import com.trefas.asledger.data.settings.ServerConfig
import com.trefas.asledger.data.sync.LibraryProgress
import com.trefas.asledger.ui.theme.AsledgerTheme
import org.junit.Assert.assertEquals
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config

/** Экран первичной загрузки: прогресс, ошибки, отмена и повторная привязка. */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [34])
class LoadingScreenTest {
    @get:Rule
    val compose = createComposeRule()

    private val config = ServerConfig(
        host = "192.168.1.10",
        port = 8778,
        clientId = "client-1",
        userName = "Анна"
    )

    @Test
    fun `прогресс показывает этап и процент`() {
        compose.setContent {
            AsledgerTheme {
                LoadingScreen(
                    config = config,
                    state = LoadingState.Running(
                        LibraryProgress("Сохраняем проводки", 60)
                    ),
                    onRetry = {},
                    onCancel = {},
                    onReenroll = {},
                    onChangeServer = {}
                )
            }
        }

        compose.onNodeWithTag("loading-progress").assertIsDisplayed()
        compose.onNodeWithText("Сохраняем проводки").assertIsDisplayed()
        compose.onNodeWithText("60 %").assertIsDisplayed()
        compose.onNodeWithText("192.168.1.10:8778").assertIsDisplayed()
        compose.onNodeWithTag("loading-cancel").assertIsDisplayed()
    }

    @Test
    fun `ошибка соединения объясняется и повторяется`() {
        var retried = 0
        val error = ConnectionError.ConnectionRefused
        compose.setContent {
            AsledgerTheme {
                LoadingScreen(
                    config = config,
                    state = LoadingState.Failure(error, error.toProblem()),
                    onRetry = { retried += 1 },
                    onCancel = {},
                    onReenroll = {},
                    onChangeServer = {}
                )
            }
        }

        compose.onNodeWithText("Сервер не отвечает").assertIsDisplayed()
        compose.onNodeWithText(
            "Запущен ли asledgerd? Проверьте порт API — по умолчанию 8778."
        ).assertIsDisplayed()
        compose.onNodeWithTag("loading-retry").performClick()
        assertEquals(1, retried)
    }

    @Test
    fun `401 предлагает привязать устройство заново`() {
        var reenrolled = 0
        val error = ConnectionError.Unauthorized
        compose.setContent {
            AsledgerTheme {
                LoadingScreen(
                    config = config,
                    state = LoadingState.Failure(error, error.toProblem()),
                    onRetry = {},
                    onCancel = {},
                    onReenroll = { reenrolled += 1 },
                    onChangeServer = {}
                )
            }
        }

        compose.onNodeWithText("Устройство отвязано").assertIsDisplayed()
        compose.onNodeWithTag("loading-reenroll").performClick()
        assertEquals(1, reenrolled)
    }

    @Test
    fun `отмена не теряет адрес и предлагает продолжить`() {
        var retried = 0
        compose.setContent {
            AsledgerTheme {
                LoadingScreen(
                    config = config,
                    state = LoadingState.Cancelled,
                    onRetry = { retried += 1 },
                    onCancel = {},
                    onReenroll = {},
                    onChangeServer = {}
                )
            }
        }

        compose.onNodeWithText("Загрузка отменена").assertIsDisplayed()
        compose.onNodeWithText("Сохранённые адрес и пользователь не изменены.")
            .assertIsDisplayed()
        compose.onNodeWithTag("loading-retry").performClick()
        assertEquals(1, retried)
    }
}
