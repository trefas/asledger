package com.trefas.asledger.ui.screens

import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onAllNodesWithText
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollTo
import com.trefas.asledger.data.network.ConnectionError
import com.trefas.asledger.data.network.EnrollResult
import com.trefas.asledger.data.network.EnrollUser
import com.trefas.asledger.data.settings.ServerConfig
import com.trefas.asledger.ui.theme.AsledgerTheme
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config

/** Проверка выбора пользователя при привязке устройства. */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [34])
class EnrollScreenTest {
    @get:Rule
    val compose = createComposeRule()

    @Test
    fun `выбор пользователя повторяет enroll с user_id`() {
        var enrolledUserId: Long? = null
        var done = false
        val config = ServerConfig(
            host = "192.168.1.10",
            port = 8778,
            clientId = "client-1",
            serverVersion = "v1.0.0",
            formatVersion = 1
        )
        compose.setContent {
            AsledgerTheme {
                EnrollScreen(
                    config = config,
                    onEnroll = { userId ->
                        if (userId == null) {
                            EnrollResult.UserRequired(
                                listOf(
                                    EnrollUser(id = 1, name = "Анна"),
                                    EnrollUser(id = 2, name = "Борис")
                                )
                            )
                        } else {
                            EnrollResult.Success(
                                token = "secret",
                                clientId = "client-1",
                                userId = userId,
                                userName = "Анна"
                            )
                        }
                    },
                    onEnrolled = { success ->
                        enrolledUserId = success.userId
                        null
                    },
                    onDone = { done = true },
                    onChangeServer = {}
                )
            }
        }

        compose.waitUntil(timeoutMillis = 5_000) {
            compose.onAllNodesWithText("Анна", useUnmergedTree = true)
                .fetchSemanticsNodes().isNotEmpty()
        }
        compose.onNodeWithText("Анна", useUnmergedTree = true)
            .performScrollTo()
            .performClick()
        compose.onNodeWithText("Продолжить")
            .performScrollTo()
            .performClick()

        compose.waitUntil(timeoutMillis = 5_000) { done }
        assertEquals(1L, enrolledUserId)
    }

    @Test
    fun `экран не просит пароль`() {
        val config = ServerConfig(
            host = "192.168.1.10",
            port = 8778,
            clientId = "client-1"
        )
        compose.setContent {
            AsledgerTheme {
                EnrollScreen(
                    config = config,
                    onEnroll = { EnrollResult.Failure(ConnectionError.Timeout) },
                    onEnrolled = { null },
                    onDone = {},
                    onChangeServer = {}
                )
            }
        }

        compose.waitUntil(timeoutMillis = 5_000) {
            compose.onAllNodesWithText("Повторить").fetchSemanticsNodes().isNotEmpty()
        }
        assertTrue(
            compose.onAllNodesWithText("Пароль не нужен", substring = true)
                .fetchSemanticsNodes().isNotEmpty()
        )
    }
}
