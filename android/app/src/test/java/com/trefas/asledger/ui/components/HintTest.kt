package com.trefas.asledger.ui.components

import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import com.trefas.asledger.data.settings.HintStore
import com.trefas.asledger.ui.theme.AsledgerTheme
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config

/** Подсказки открываются, закрываются и запоминают просмотр. */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [34])
class HintTest {
    @get:Rule
    val compose = createComposeRule()

    private class FakeHintStore : HintStore {
        val seen = mutableSetOf<String>()

        override fun wasSeen(id: String): Boolean = id in seen

        override fun markSeen(id: String) {
            seen += id
        }
    }

    @Test
    fun `подсказка у поля открывается и закрывается`() {
        compose.setContent {
            AsledgerTheme {
                HintIcon(text = "Роль задаёт сторону строки")
            }
        }

        compose.onNodeWithContentDescription("Подсказка").performClick()
        compose.onNodeWithText("Роль задаёт сторону строки").assertIsDisplayed()

        compose.onNodeWithText("Понятно").performClick()
        compose.onNodeWithText("Роль задаёт сторону строки").assertDoesNotExist()
    }

    @Test
    fun `подсказка при первом открытии запоминается`() {
        val store = FakeHintStore()
        compose.setContent {
            AsledgerTheme {
                CompositionLocalProvider(LocalHintStore provides store) {
                    FirstOpenHint(
                        id = "entry-form",
                        title = "Как заполнить проводку",
                        text = "Суммы дебета и кредита должны совпадать."
                    )
                }
            }
        }

        compose.onNodeWithText("Как заполнить проводку").assertIsDisplayed()
        compose.onNodeWithText("Понятно").performClick()

        assertTrue(store.wasSeen("entry-form"))
        compose.onNodeWithText("Как заполнить проводку").assertDoesNotExist()
    }
}
