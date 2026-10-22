package com.trefas.asledger.ui.screens

import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.hasTestTag
import androidx.compose.ui.test.hasText
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onAllNodesWithContentDescription
import androidx.compose.ui.test.onAllNodesWithText
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollToNode
import androidx.compose.ui.test.performTextReplacement
import com.trefas.asledger.data.model.Account
import com.trefas.asledger.data.model.AccountType
import com.trefas.asledger.data.model.OperationRoles
import com.trefas.asledger.data.model.RoleRule
import com.trefas.asledger.ui.theme.AsledgerTheme
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config

/** Форма проводки: строки, суммы, ошибки и вызовы сохранения. */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [34])
class EntryScreenTest {
    @get:Rule
    val compose = createComposeRule()

    private val expense = Account(
        id = 8,
        name = "Продукты",
        fullPath = "Расходы:Продукты",
        type = AccountType.EXPENSE,
        parentId = 7,
        isLeaf = true,
        minBalanceKop = 0,
        isArchived = false,
        isSystem = false,
        createdAt = 0
    )

    private val cash = Account(
        id = 4,
        name = "Наличные",
        fullPath = "Активы:Наличные",
        type = AccountType.ASSET,
        parentId = 3,
        isLeaf = true,
        minBalanceKop = 0,
        isArchived = false,
        isSystem = false,
        createdAt = 0
    )

    private val matrix = OperationRoles(
        opType = "трата",
        title = "Трата",
        roles = listOf(
            RoleRule("расход", "DEBIT", listOf(AccountType.EXPENSE), false, false),
            RoleRule("источник", "CREDIT", listOf(AccountType.ASSET), false, false)
        ),
        minDebit = 1,
        minCredit = 1
    )

    private fun state(
        debit: String = "1000",
        credit: String = "1000",
        message: String? = null
    ) = EntryUiState(
        opType = "трата",
        date = 1_700_000_000_000,
        description = "Обед",
        roles = matrix,
        accounts = listOf(expense, cash),
        loadingRoles = false,
        message = message,
        lines = listOf(
            EntryLineUi(
                key = 1,
                accountId = 8,
                amount = debit,
                role = "расход",
                side = "DEBIT"
            ),
            EntryLineUi(
                key = 2,
                accountId = 4,
                amount = credit,
                role = "источник",
                side = "CREDIT"
            )
        )
    )

    private fun content(
        state: EntryUiState,
        onSave: () -> Unit = {},
        onDescription: (String) -> Unit = {},
        onAddLine: (String) -> Unit = {},
        onRemoveLine: (Long) -> Unit = {}
    ) {
        compose.setContent {
            AsledgerTheme {
                EntryScreen(
                    state = state,
                    onOpType = {},
                    onDescription = onDescription,
                    onDate = {},
                    onAddLine = onAddLine,
                    onRemoveLine = onRemoveLine,
                    onAccount = { _, _ -> },
                    onAmount = { _, _ -> },
                    onQuickAmount = { _, _ -> },
                    onRole = { _, _ -> },
                    onSave = onSave,
                    onBack = {},
                    onSaved = {}
                )
            }
        }
    }

    @Test
    fun `сбалансированная форма показывает итоги и сохраняется`() {
        var saves = 0
        content(state(), onSave = { saves += 1 })

        compose.onNodeWithTag("entry-form").performScrollToNode(hasTestTag("entry-balance"))
        compose.onNodeWithTag("entry-balance").assertIsDisplayed()
        compose.onAllNodesWithText("1 000,00 ₽")[0].assertIsDisplayed()
        compose.onNodeWithTag("entry-form").performScrollToNode(hasTestTag("entry-save"))
        compose.onNodeWithTag("entry-save").performClick()

        assertEquals(1, saves)
    }

    @Test
    fun `ошибка баланса видна и строка помечена`() {
        content(
            state(debit = "1000", credit = "900", message = "Проверьте строки"),
            onSave = {}
        )

        val balanceError = "Суммы дебета и кредита должны совпадать."
        compose.onNodeWithTag("entry-form").performScrollToNode(hasText(balanceError))
        compose.onNodeWithText(balanceError).assertIsDisplayed()
        compose.onNodeWithTag("entry-message").assertIsDisplayed()
    }

    @Test
    fun `описание и удаление строки передаются наружу`() {
        var description = ""
        var removed: Long? = null
        content(
            state(),
            onDescription = { description = it },
            onRemoveLine = { removed = it }
        )

        compose.onNodeWithTag("entry-description").performTextReplacement("Обед и чай")
        assertEquals("Обед и чай", description)
        compose.onNodeWithText("Продукты").assertIsDisplayed()
        compose.onAllNodesWithContentDescription("Удалить строку")[0].performClick()
        assertEquals(1L, removed)
        assertTrue(removed != null)
    }

    @Test
    fun `выбор счёта скрывает счета других типов`() {
        val source = state()
        content(
            source.copy(
                lines = source.lines.map { line ->
                    if (line.key == 1L) line.copy(accountId = null) else line
                }
            )
        )

        compose.onNodeWithTag("entry-account-1").performClick()

        compose.onNodeWithText("Расходы:Продукты").assertIsDisplayed()
        compose.onNodeWithText("Активы:Наличные").assertDoesNotExist()
    }

    @Test
    fun `добавление строки вызывает обработчик стороны`() {
        var side: String? = null
        content(state(), onAddLine = { side = it })

        compose.onAllNodesWithText("Строка")[0].performClick()
        assertEquals("DEBIT", side)
    }
}
