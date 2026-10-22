package com.trefas.asledger.ui.screens

import com.trefas.asledger.R
import com.trefas.asledger.data.TestLibrary
import com.trefas.asledger.data.db.AsledgerDatabase
import com.trefas.asledger.data.db.OutboxStatus
import com.trefas.asledger.data.db.SettingEntity
import com.trefas.asledger.data.network.AsledgerApi
import com.trefas.asledger.data.network.EngineRoleDto
import com.trefas.asledger.data.network.EngineRolesDto
import com.trefas.asledger.data.repository.AccountRepository
import com.trefas.asledger.data.repository.LocalEntryRepository
import com.trefas.asledger.data.repository.RoleRepository
import com.trefas.asledger.data.repository.TransactionRepository
import com.trefas.asledger.data.security.TokenStore
import com.trefas.asledger.ui.MainDispatcherRule
import io.ktor.client.HttpClient
import io.ktor.client.engine.mock.MockEngine
import io.ktor.client.engine.mock.respondError
import io.ktor.http.HttpStatusCode
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.flow.flowOf
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.runTest
import kotlinx.serialization.encodeToString
import kotlinx.serialization.json.Json
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config

/** Форма проводки: матрица ролей, баланс, ошибки и офлайн-сохранение. */
@OptIn(kotlinx.coroutines.ExperimentalCoroutinesApi::class)
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [34])
class EntryViewModelTest {
    @get:Rule
    val mainDispatcherRule = MainDispatcherRule()

    private lateinit var database: AsledgerDatabase
    private val json = Json { encodeDefaults = true }
    private val tokenStore = object : TokenStore {
        override suspend fun save(token: String) = Unit
        override suspend fun load(): String? = null
        override suspend fun clear() = Unit
    }

    @Before
    fun setUp() {
        database = TestLibrary.database()
    }

    @After
    fun tearDown() {
        database.close()
    }

    private suspend fun seedRoles() {
        TestLibrary.imported(database)
        val matrix = EngineRolesDto(
            opType = "трата",
            title = "Трата",
            roles = listOf(
                EngineRoleDto(
                    role = "расход",
                    side = "DEBIT",
                    accountTypes = listOf("расход")
                ),
                EngineRoleDto(
                    role = "источник",
                    side = "CREDIT",
                    accountTypes = listOf("актив")
                )
            )
        )
        database.settingDao().upsertAll(
            listOf(
                SettingEntity(
                    key = "android.roles.трата",
                    value = json.encodeToString(matrix)
                )
            )
        )
    }

    private fun viewModel(
        editUuid: String? = null,
        copySource: Boolean = false,
        accountId: Long? = null,
        saved: () -> Unit = {}
    ): EntryViewModel {
        val api = AsledgerApi(
            HttpClient(MockEngine { respondError(HttpStatusCode.InternalServerError) })
        )
        return EntryViewModel(
            editUuid = editUuid,
            copySource = copySource,
            preselectedAccountId = accountId,
            configFlow = flowOf(null),
            tokenStore = tokenStore,
            rolesRepository = RoleRepository(database.settingDao(), api),
            accountsRepository = AccountRepository(database.accountDao()),
            transactionsRepository = TransactionRepository(
                database.transactionDao(),
                database.outboxDao()
            ),
            localEntries = LocalEntryRepository(database),
            onSaved = saved
        )
    }

    @Test
    fun `форма строит строки из кэша ролей и предвыбирает счёт`() = runEntryTest {
        seedRoles()

        val model = viewModel(accountId = 4)
        advanceUntilIdle()

        val state = model.state.value
        assertNotNull(state.roles)
        assertEquals(2, state.lines.size)
        assertEquals("расход", state.lines.first { it.side == "DEBIT" }.role)
        assertEquals("источник", state.lines.first { it.side == "CREDIT" }.role)
        assertEquals(4L, state.lines.first { it.side == "DEBIT" }.accountId)
        assertTrue(state.loadingRoles.not())
        assertNull(state.rolesErrorRes)
    }

    @Test
    fun `несбалансированная проводка не сохраняется и показывает строку ошибки`() = runTest(
        mainDispatcherRule.dispatcher
    ) {
        seedRoles()
        val model = viewModel()
        advanceUntilIdle()

        val debit = model.state.value.lines.first { it.side == "DEBIT" }
        val credit = model.state.value.lines.first { it.side == "CREDIT" }
        model.setAccount(debit.key, 8)
        model.setAccount(credit.key, 4)
        model.setAmount(debit.key, "100")
        model.setAmount(credit.key, "90")
        model.save()
        advanceUntilIdle()

        val state = model.state.value
        assertFalse(state.saved)
        assertEquals(R.string.entry_message_balance, state.messageRes)
        assertNotNull(state.lines.first { it.key == debit.key }.errorRes)
        assertEquals(0, database.outboxDao().all().size)
    }

    @Test
    fun `быстрые суммы складываются в копейках без float`() = runEntryTest {
        seedRoles()
        val model = viewModel()
        advanceUntilIdle()
        val key = model.state.value.lines.first().key

        model.addAmount(key, 100_00)
        model.addAmount(key, 50_50)

        assertEquals("150,50", model.state.value.lines.first { it.key == key }.amount)
        assertEquals(15_050, model.state.value.debitKop)
    }

    @Test
    fun `роль проверяется по типу счёта и стороне`() = runEntryTest {
        seedRoles()
        val model = viewModel()
        advanceUntilIdle()
        val debit = model.state.value.lines.first { it.side == "DEBIT" }
        val credit = model.state.value.lines.first { it.side == "CREDIT" }

        model.setAccount(debit.key, 8)
        model.setAccount(credit.key, 4)
        model.setRole(debit.key, "расход")
        model.setAmount(debit.key, "10")
        model.setAmount(credit.key, "10")
        model.setRole(debit.key, "источник")
        // Роль «источник» — кредитная, поэтому меняет сторону строки.
        assertEquals("CREDIT", model.state.value.lines.first { it.key == debit.key }.side)

        model.setRole(debit.key, "расход")
        assertEquals("DEBIT", model.state.value.lines.first { it.key == debit.key }.side)
        model.save()
        advanceUntilIdle()
        assertTrue(model.state.value.saved)
    }

    @Test
    fun `сбалансированная проводка сохраняется в outbox и журнал`() = runEntryTest {
        seedRoles()
        var saved = false
        val model = viewModel(saved = { saved = true })
        advanceUntilIdle()
        val debit = model.state.value.lines.first { it.side == "DEBIT" }
        val credit = model.state.value.lines.first { it.side == "CREDIT" }
        model.setDescription("Обед")
        model.setAccount(debit.key, 8)
        model.setAccount(credit.key, 4)
        model.setAmount(debit.key, "1 250,50")
        model.setAmount(credit.key, "1250.50")

        model.save()
        advanceUntilIdle()

        val state = model.state.value
        assertTrue(state.saved)
        assertTrue(saved)
        val outbox = database.outboxDao().all().single()
        assertEquals(OutboxStatus.PENDING, outbox.status)
        assertEquals("android:1", outbox.commitId)
        val details = database.transactionDao().observeDetails(outbox.transactionUuid).first()
        assertEquals("Обед", details?.transaction?.description)
        assertEquals(2, details?.lines?.size)
        assertEquals(
            125_050L,
            details?.lines?.filter { it.side == "DEBIT" }?.sumOf { it.amountKop }
        )
    }

    private fun runEntryTest(
        block: suspend kotlinx.coroutines.test.TestScope.() -> Unit
    ) = runTest(mainDispatcherRule.dispatcher, testBody = block)

    @Test
    fun `добавление и удаление строк не теряет баланс формы`() = runEntryTest {
        seedRoles()
        val model = viewModel()
        advanceUntilIdle()

        model.addLine("DEBIT")
        assertEquals(3, model.state.value.lines.size)
        val added = model.state.value.lines.last()
        model.removeLine(added.key)
        assertEquals(2, model.state.value.lines.size)
    }
}
