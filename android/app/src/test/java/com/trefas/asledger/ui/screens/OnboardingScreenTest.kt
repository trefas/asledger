package com.trefas.asledger.ui.screens

import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import com.trefas.asledger.ui.theme.AsledgerTheme
import org.junit.Assert.assertEquals
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config

/** Онбординг: шаги, пропуск, повторный запуск. */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [34])
class OnboardingScreenTest {
    @get:Rule
    val compose = createComposeRule()

    @Test
    fun `тур проходит четыре шага и завершается`() {
        var finished = 0
        compose.setContent {
            AsledgerTheme {
                OnboardingScreen(onDone = { finished += 1 })
            }
        }

        compose.onNodeWithText("Книга всегда под рукой").assertIsDisplayed()
        compose.onNodeWithText("Далее").performClick()
        compose.onNodeWithText("План счетов").assertIsDisplayed()
        compose.onNodeWithText("Далее").performClick()
        compose.onNodeWithText("Проводки без сети").assertIsDisplayed()
        compose.onNodeWithText("Далее").performClick()
        compose.onNodeWithText("Синхронизация и порядок").assertIsDisplayed()
        compose.onNodeWithTag("onboarding-next").performClick()

        assertEquals(1, finished)
    }

    @Test
    fun `пропуск доступен и завершает тур`() {
        var skipped = 0
        compose.setContent {
            AsledgerTheme {
                OnboardingScreen(onDone = { skipped += 1 })
            }
        }

        compose.onNodeWithTag("onboarding-skip").performClick()

        assertEquals(1, skipped)
    }
}
