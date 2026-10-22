package com.trefas.asledger.ui.screens

import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertTextContains
import androidx.compose.ui.test.junit4.StateRestorationTester
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onAllNodesWithText
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollTo
import androidx.compose.ui.test.performTextClearance
import androidx.compose.ui.test.performTextInput
import com.trefas.asledger.data.network.ApiResult
import com.trefas.asledger.data.network.ConnectionError
import com.trefas.asledger.data.network.HealthInfo
import com.trefas.asledger.data.settings.ServerConfig
import com.trefas.asledger.ui.theme.AsledgerTheme
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config

/** UI-тесты первого экрана: валидация адреса и ошибки сети. */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [34])
class SetupScreenTest {
    @get:Rule
    val compose = createComposeRule()

    @Test
    fun `пустой адрес не отправляется`() {
        compose.setContent {
            AsledgerTheme {
                SetupScreen(
                    onSubmit = { throw AssertionError("запрос не должен уйти") },
                    onSuccess = { _, _ -> }
                )
            }
        }

        compose.onNodeWithTag("setup-submit").performScrollTo().performClick()

        compose.onNodeWithText("Укажите IP-адрес или имя компьютера")
            .performScrollTo()
            .assertIsDisplayed()
    }

    @Test
    fun `некорректный порт объясняется`() {
        compose.setContent {
            AsledgerTheme {
                SetupScreen(
                    onSubmit = { throw AssertionError("запрос не должен уйти") },
                    onSuccess = { _, _ -> }
                )
            }
        }

        compose.onNodeWithTag("setup-host").performTextInput("192.168.1.10")
        compose.onNodeWithTag("setup-port").performTextClearance()
        compose.onNodeWithTag("setup-port").performTextInput("0")
        compose.onNodeWithTag("setup-submit").performScrollTo().performClick()

        compose.onNodeWithText("Порт должен быть числом от 1 до 65535")
            .performScrollTo()
            .assertIsDisplayed()
    }

    @Test
    fun `отказ соединения показывает подсказку и повтор`() {
        compose.setContent {
            AsledgerTheme {
                SetupScreen(
                    onSubmit = {
                        ApiResult.Failure(ConnectionError.ConnectionRefused)
                    },
                    onSuccess = { _, _ -> }
                )
            }
        }

        compose.onNodeWithTag("setup-host").performTextInput("192.168.1.10")
        compose.onNodeWithTag("setup-submit").performScrollTo().performClick()

        compose.waitUntil(timeoutMillis = 5_000) {
            compose.onAllNodesWithText("Сервер не отвечает").fetchSemanticsNodes().isNotEmpty()
        }
        compose.onNodeWithText("Сервер не отвечает")
            .performScrollTo()
            .assertIsDisplayed()
        compose.onNodeWithText("Проверить ещё раз")
            .performScrollTo()
            .assertIsDisplayed()
    }

    @Test
    fun `адрес и порт переживают восстановление состояния`() {
        val restoration = StateRestorationTester(compose)
        restoration.setContent {
            AsledgerTheme {
                SetupScreen(
                    onSubmit = { throw AssertionError("запрос не должен уйти") },
                    onSuccess = { _, _ -> }
                )
            }
        }

        compose.onNodeWithTag("setup-host").performTextInput("192.168.1.10")
        compose.onNodeWithTag("setup-port").performTextClearance()
        compose.onNodeWithTag("setup-port").performTextInput("9000")

        restoration.emulateSavedInstanceStateRestore()

        compose.onNodeWithTag("setup-host").assertTextContains("192.168.1.10")
        compose.onNodeWithTag("setup-port").assertTextContains("9000")
    }

    @Test
    fun `успешная проверка сохраняет адрес`() {
        var saved: ServerConfig? = null
        var savedHealth: HealthInfo? = null
        compose.setContent {
            AsledgerTheme {
                SetupScreen(
                    onSubmit = {
                        ApiResult.Ok(
                            HealthInfo(
                                status = "ok",
                                version = "v1.0.0",
                                formatVersion = 1
                            )
                        )
                    },
                    onSuccess = { config, health ->
                        saved = config
                        savedHealth = health
                    }
                )
            }
        }

        compose.onNodeWithTag("setup-host").performTextInput("192.168.1.10")
        compose.onNodeWithTag("setup-submit").performScrollTo().performClick()

        compose.waitUntil(timeoutMillis = 5_000) { saved != null }
        assertNotNull(saved)
        assertEquals("192.168.1.10", saved?.host)
        assertEquals(8778, saved?.port)
        assertEquals("v1.0.0", savedHealth?.version)
    }
}
