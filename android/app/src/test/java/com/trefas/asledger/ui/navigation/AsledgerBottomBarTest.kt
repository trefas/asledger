package com.trefas.asledger.ui.navigation

import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import com.trefas.asledger.ui.theme.AsledgerTheme
import org.junit.Assert.assertEquals
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config

/** Compose-тест нижней навигации; выполняется на JVM через Robolectric. */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [34])
class AsledgerBottomBarTest {
    @get:Rule
    val compose = createComposeRule()

    @Test
    fun `нажатие на раздел передаёт маршрут`() {
        var selected: String? = null
        compose.setContent {
            AsledgerTheme {
                AsledgerBottomBar(
                    currentRoute = Routes.JOURNAL,
                    onNavigate = { route -> selected = route }
                )
            }
        }

        compose.onNodeWithText("Настройки").performClick()
        assertEquals(Routes.SETTINGS, selected)
    }
}
