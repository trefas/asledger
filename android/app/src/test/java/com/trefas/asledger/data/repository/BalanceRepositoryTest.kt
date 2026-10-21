package com.trefas.asledger.data.repository

import com.trefas.asledger.data.TestLibrary
import com.trefas.asledger.data.db.AccountEntity
import com.trefas.asledger.data.db.AccountMovement
import com.trefas.asledger.data.db.AsledgerDatabase
import com.trefas.asledger.data.db.LineEntity
import com.trefas.asledger.data.db.OutboxEntity
import com.trefas.asledger.data.db.TransactionEntity
import com.trefas.asledger.data.model.Account
import com.trefas.asledger.data.model.AccountType
import com.trefas.asledger.util.normalizeSearch
import com.trefas.asledger.util.parseServerTime
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.test.runTest
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config

/** Локальные остатки и обороты совпадают с формулой сервера. */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [34])
class BalanceRepositoryTest {
    private val database: AsledgerDatabase = TestLibrary.database()

    @After
    fun tearDown() {
        database.close()
    }

    @Test
    fun `остатки и итоги по поддеревьям`() = runTest {
        TestLibrary.imported(database)
        val report = BalanceRepository(
            database.accountDao(),
            database.transactionDao()
        ).observeReport().first()

        val cash = report.rows.first { it.account.id == 4L }
        assertEquals(180000, cash.ownKop)
        assertEquals(180000, cash.totalKop)

        val assets = report.rows.first { it.account.name == "Активы" }
        assertEquals(180000, assets.totalKop)

        assertEquals(100000, report.rows.first { it.account.id == 2L }.totalKop)
        assertEquals(50000, report.rows.first { it.account.id == 6L }.totalKop)
        assertEquals(20000, report.rows.first { it.account.id == 8L }.totalKop)
        assertEquals(50000, report.rows.first { it.account.id == 10L }.totalKop)

        assertEquals(180000, report.assetsKop)
        assertEquals(50000, report.liabilitiesKop)
        assertEquals(130000, report.netWorthKop)
    }

    @Test
    fun `обороты за период и остаток на начало`() = runTest {
        TestLibrary.imported(database)
        val report = BalanceRepository(
            database.accountDao(),
            database.transactionDao()
        ).observeReport(
            fromDate = parseServerTime("2026-01-05T00:00:00Z"),
            toDate = parseServerTime("2026-01-31T23:59:59Z")
        ).first()

        val cash = report.rows.first { it.account.id == 4L }
        assertEquals(100000, cash.openingKop)
        assertEquals(100000, cash.periodDebitKop)
        assertEquals(20000, cash.periodCreditKop)
        assertEquals(180000, cash.ownKop)

        val assets = report.rows.first { it.account.name == "Активы" }
        assertEquals(100000, assets.openingTotalKop)
        assertEquals(100000, assets.periodDebitTotalKop)
        assertEquals(20000, assets.periodCreditTotalKop)

        val expenses = report.rows.first { it.account.name == "Расходы" }
        assertEquals(20000, expenses.periodDebitTotalKop)
        assertEquals(20000, expenses.totalKop)
    }

    @Test
    fun `очередь не меняет подтверждённый остаток`() = runTest {
        TestLibrary.imported(database)
        val now = 1_700_000_000_000L
        database.transactionDao().upsertAll(
            listOf(
                TransactionEntity(
                    uuid = "local-1",
                    serverId = null,
                    opType = "расход",
                    date = now,
                    description = "Черновик",
                    actorId = 1,
                    reversesId = null,
                    source = "android",
                    sourceHash = "",
                    createdAt = now,
                    commitId = 0,
                    searchKey = normalizeSearch("Черновик")
                )
            )
        )
        database.transactionDao().upsertLines(
            listOf(
                LineEntity(
                    serverId = null,
                    transactionUuid = "local-1",
                    accountId = 8,
                    amountKop = 1000,
                    side = "DEBIT"
                ),
                LineEntity(
                    serverId = null,
                    transactionUuid = "local-1",
                    accountId = 4,
                    amountKop = 1000,
                    side = "CREDIT"
                )
            )
        )
        database.outboxDao().upsert(
            OutboxEntity(
                commitId = "client:1",
                transactionUuid = "local-1",
                clientId = "client",
                baseSeq = 4,
                opsJson = "{}",
                status = "pending",
                createdAt = now,
                updatedAt = now
            )
        )

        val report = BalanceRepository(
            database.accountDao(),
            database.transactionDao()
        ).observeReport().first()

        assertEquals(180000, report.assetsKop)
        assertEquals(-1000, report.pendingAssetsKop)
        assertEquals(-1000, report.pendingNetWorthKop)
        assertEquals(1000, report.rows.first { it.account.id == 8L }.pendingOwnKop)
        assertEquals(-1000, report.rows.first { it.account.id == 4L }.pendingOwnKop)
    }

    @Test
    fun `отрицательный остаток сохраняет знак`() {
        val account = Account(
            id = 1,
            name = "Наличные",
            fullPath = "Активы:Наличные",
            type = AccountType.ASSET,
            parentId = null,
            isLeaf = true,
            minBalanceKop = 0,
            isArchived = false,
            isSystem = false,
            createdAt = 0
        )
        val report = buildReport(
            accounts = listOf(account),
            confirmed = listOf(
                AccountMovement(
                    accountId = 1,
                    openingDebit = 0,
                    openingCredit = 0,
                    periodDebit = 0,
                    periodCredit = 5000
                )
            ),
            pending = emptyList()
        )

        assertEquals(-5000, report.rows.single().ownKop)
        assertEquals(-5000, report.assetsKop)
        assertEquals(-5000, report.netWorthKop)
    }

    @Test
    fun `нулевой остаток не теряет счёт`() {
        val entity = AccountEntity(
            id = 1,
            name = "Наличные",
            fullPath = "Активы:Наличные",
            type = AccountType.ASSET.serverValue,
            parentId = null,
            isLeaf = true,
            minBalanceKop = 0,
            isArchived = false,
            isSystem = false,
            createdAt = 0,
            searchKey = normalizeSearch("Наличные")
        )
        val report = buildReport(
            accounts = listOf(entity.toDomain()),
            confirmed = emptyList(),
            pending = emptyList()
        )

        assertEquals(1, report.rows.size)
        assertEquals(0, report.rows.single().totalKop)
    }
}
