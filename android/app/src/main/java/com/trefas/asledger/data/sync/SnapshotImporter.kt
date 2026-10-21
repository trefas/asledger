package com.trefas.asledger.data.sync

import androidx.room.withTransaction
import com.trefas.asledger.data.db.AccountEntity
import com.trefas.asledger.data.db.AsledgerDatabase
import com.trefas.asledger.data.db.LineEntity
import com.trefas.asledger.data.db.SettingEntity
import com.trefas.asledger.data.db.SyncMetaEntity
import com.trefas.asledger.data.db.TransactionEntity
import com.trefas.asledger.data.db.UserEntity
import com.trefas.asledger.data.model.AccountType
import com.trefas.asledger.data.network.SnapshotDto
import com.trefas.asledger.util.normalizeSearch
import com.trefas.asledger.util.parseServerTime
import kotlinx.coroutines.currentCoroutineContext
import kotlinx.coroutines.ensureActive

/** Этапы импорта, по которым экран загрузки показывает прогресс. */
enum class ImportStage {
    ACCOUNTS,
    USERS,
    SETTINGS,
    TRANSACTIONS,
    FINALIZING
}

/** Ошибка структуры снапшота; транзакция импорта при ней откатывается. */
class SnapshotImportException(message: String, cause: Throwable? = null) :
    Exception(message, cause)

/**
 * Импортирует полный снапшот в Room одной транзакцией.
 *
 * Подтверждённые проводки заменяются целиком, поэтому повторный импорт
 * идемпотентен и не оставляет дублей. Проводки из очереди (`pending`/`sending`)
 * и их строки сохраняются: полный ресинк не должен терять офлайн-черновики.
 */
class SnapshotImporter(
    private val database: AsledgerDatabase,
    private val now: () -> Long = System::currentTimeMillis
) {
    suspend fun import(
        snapshot: SnapshotDto,
        onProgress: (ImportStage) -> Unit = {}
    ) {
        if (snapshot.formatVersion != SUPPORTED_FORMAT_VERSION) {
            throw SnapshotImportException(
                "формат снапшота ${snapshot.formatVersion} не поддерживается"
            )
        }
        val prepared = prepare(snapshot)
        database.withTransaction {
            onProgress(ImportStage.ACCOUNTS)
            currentCoroutineContext().ensureActive()
            database.accountDao().clear()
            database.accountDao().upsertAll(prepared.accounts)

            onProgress(ImportStage.USERS)
            currentCoroutineContext().ensureActive()
            database.userDao().clear()
            database.userDao().upsertAll(prepared.users)

            onProgress(ImportStage.SETTINGS)
            currentCoroutineContext().ensureActive()
            database.settingDao().clear()
            database.settingDao().upsertAll(prepared.settings)

            onProgress(ImportStage.TRANSACTIONS)
            currentCoroutineContext().ensureActive()
            val transactionDao = database.transactionDao()
            transactionDao.deleteConfirmedLines()
            prepared.transactions
                .map { it.uuid }
                .chunked(IMPORT_CHUNK)
                .forEach { uuids ->
                    currentCoroutineContext().ensureActive()
                    transactionDao.deleteLinesFor(uuids)
                }
            transactionDao.deleteConfirmedTransactions()
            transactionDao.upsertAll(prepared.transactions)
            prepared.lines.chunked(IMPORT_CHUNK).forEach { chunk ->
                currentCoroutineContext().ensureActive()
                transactionDao.upsertLines(chunk)
            }

            onProgress(ImportStage.FINALIZING)
            currentCoroutineContext().ensureActive()
            database.syncMetaDao().upsert(
                SyncMetaEntity(
                    headSeq = snapshot.headSeq,
                    chainHash = snapshot.chainHash,
                    formatVersion = snapshot.formatVersion,
                    initialLoaded = true,
                    lastSyncAt = now()
                )
            )
        }
    }

    private fun prepare(snapshot: SnapshotDto): PreparedSnapshot {
        val seenAccounts = HashSet<Long>()
        val accounts = snapshot.accounts.map { dto ->
            if (!seenAccounts.add(dto.id)) {
                throw SnapshotImportException("повторяющийся id счёта ${dto.id}")
            }
            val type = AccountType.fromServer(dto.type)
                ?: throw SnapshotImportException(
                    "неизвестный тип счёта ${dto.type} у ${dto.fullPath}"
                )
            AccountEntity(
                id = dto.id,
                name = dto.name,
                fullPath = dto.fullPath,
                type = type.serverValue,
                parentId = dto.parentId,
                isLeaf = dto.isLeaf,
                minBalanceKop = dto.minBalanceKop,
                isArchived = dto.isArchived,
                isSystem = dto.isSystem,
                createdAt = parseTime(dto.createdAt, "счёт ${dto.fullPath}"),
                searchKey = normalizeSearch(dto.name, dto.fullPath)
            )
        }.sortedBy { it.fullPath }

        val users = snapshot.users
            .distinctBy { it.id }
            .map { dto ->
                UserEntity(id = dto.id, name = dto.name, searchKey = normalizeSearch(dto.name))
            }

        val settings = snapshot.settings.map { (key, value) ->
            SettingEntity(key = key, value = value)
        }

        val seenTransactions = HashSet<String>()
        val seenServerIds = HashSet<Long>()
        val seenLineIds = HashSet<Long>()
        val transactions = snapshot.transactions.map { dto ->
            if (dto.uuid.isBlank()) {
                throw SnapshotImportException("пустой UUID проводки ${dto.id}")
            }
            if (!seenTransactions.add(dto.uuid)) {
                throw SnapshotImportException("повторяющийся UUID проводки ${dto.uuid}")
            }
            if (!seenServerIds.add(dto.id)) {
                throw SnapshotImportException("повторяющийся id проводки ${dto.id}")
            }
            if (dto.lines.isEmpty()) {
                throw SnapshotImportException("проводка ${dto.uuid} без строк")
            }
            TransactionEntity(
                uuid = dto.uuid,
                serverId = dto.id,
                opType = dto.opType,
                date = parseTime(dto.date, "проводка ${dto.uuid}"),
                description = dto.description,
                actorId = dto.actorId,
                reversesId = dto.reversesId,
                source = dto.source,
                sourceHash = dto.sourceHash,
                createdAt = parseTime(dto.createdAt, "проводка ${dto.uuid}"),
                commitId = dto.commitId,
                searchKey = normalizeSearch(dto.description, dto.uuid)
            )
        }

        val lines = snapshot.transactions.flatMap { transaction ->
            transaction.lines.map { line ->
                if (line.side != "DEBIT" && line.side != "CREDIT") {
                    throw SnapshotImportException(
                        "неизвестная сторона ${line.side} в проводке ${transaction.uuid}"
                    )
                }
                if (!seenLineIds.add(line.id)) {
                    throw SnapshotImportException("повторяющийся id строки ${line.id}")
                }
                LineEntity(
                    serverId = line.id,
                    transactionUuid = transaction.uuid,
                    accountId = line.accountId,
                    amountKop = line.amountKop,
                    side = line.side
                )
            }
        }

        return PreparedSnapshot(
            accounts = accounts,
            users = users,
            settings = settings,
            transactions = transactions,
            lines = lines
        )
    }

    private fun parseTime(value: String, what: String): Long = try {
        parseServerTime(value)
    } catch (failure: IllegalArgumentException) {
        throw SnapshotImportException("некорректная дата у «$what»", failure)
    }

    private data class PreparedSnapshot(
        val accounts: List<AccountEntity>,
        val users: List<UserEntity>,
        val settings: List<SettingEntity>,
        val transactions: List<TransactionEntity>,
        val lines: List<LineEntity>
    )

    companion object {
        /** Поддерживаемая версия снапшота; совпадает с серверной. */
        const val SUPPORTED_FORMAT_VERSION = 1

        /** SQLite ограничивает число параметров; импортируем пачками. */
        private const val IMPORT_CHUNK = 400
    }
}
