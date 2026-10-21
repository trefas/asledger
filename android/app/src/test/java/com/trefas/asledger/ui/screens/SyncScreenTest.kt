package com.trefas.asledger.ui.screens

import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollTo
import com.trefas.asledger.data.model.OutboxItem
import com.trefas.asledger.data.model.SyncStatus
import com.trefas.asledger.data.sync.SyncPhase
import com.trefas.asledger.data.sync.SyncState
import com.trefas.asledger.ui.theme.AsledgerTheme
import org.junit.Assert.assertEquals
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config

/** Экран очереди: отклонение, дубли, предупреждения и действия. */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [34])
class SyncScreenTest {
    @get:Rule
    val compose = createComposeRule()

    private val rejected = OutboxItem(
        transactionUuid = "uuid-1",
        commitId = "client:7",
        status = SyncStatus.REJECTED,
        attempts = 2,
        lastError = "роль не подходит к счёту",
        errorCode = "ERR_MATRIX_CELL",
        warnings = listOf("Пакет применён поверх актуальной книги"),
        duplicateUuids = listOf("uuid-old"),
        description = "Продукты",
        amountKop = 125_050,
        date = 1_700_000_000_000
    )

    @Test
    fun `отклонённая проводка показывает код и кнопку исправления`() {
        var edited: String? = null
        compose.setContent {
            AsledgerTheme {
                SyncScreen(
                    state = SyncScreenState(
                        items = listOf(rejected),
                        sync = SyncState(phase = SyncPhase.REJECTED)
                    ),
                    onSync = {},
                    onOpen = {},
                    onEdit = { edited = it },
                    onDelete = {},
                    onBack = {}
                )
            }
        }

        compose.onNodeWithText("ERR_MATRIX_CELL: роль не подходит к счёту")
            .assertIsDisplayed()
        compose.onNodeWithText("Возможные дубли: uuid-old").assertIsDisplayed()
        compose.onNodeWithText("Пакет применён поверх актуальной книги").assertIsDisplayed()
        compose.onNodeWithTag("outbox-uuid-1").assertIsDisplayed()
        compose.onNodeWithText("Исправить").performScrollTo().performClick()

        assertEquals("uuid-1", edited)
    }

    @Test
    fun `пустая очередь предлагает синхронизацию`() {
        var synced = 0
        compose.setContent {
            AsledgerTheme {
                SyncScreen(
                    state = SyncScreenState(
                        sync = SyncState(
                            phase = SyncPhase.SUCCESS,
                            lastSyncAt = 1_700_000_000_000
                        )
                    ),
                    onSync = { synced += 1 },
                    onOpen = {},
                    onEdit = {},
                    onDelete = {},
                    onBack = {}
                )
            }
        }

        compose.onNodeWithText("Очередь пуста. Все проводки отправлены.")
            .assertIsDisplayed()
        compose.onNodeWithText("Книга синхронизирована").assertIsDisplayed()
        compose.onNodeWithText("Отправить и обновить").performClick()
        assertEquals(1, synced)
    }
}
