package com.trefas.asledger.data.sync

import androidx.room.withTransaction
import com.trefas.asledger.data.db.AccountEntity
import com.trefas.asledger.data.db.AsledgerDatabase
import com.trefas.asledger.data.db.CommitEntity
import com.trefas.asledger.data.db.LineEntity
import com.trefas.asledger.data.db.OutboxStatus
import com.trefas.asledger.data.db.SyncMetaEntity
import com.trefas.asledger.data.db.TransactionEntity
import com.trefas.asledger.data.model.AccountType
import com.trefas.asledger.data.model.SyncCommit
import com.trefas.asledger.data.network.PullCommitDto
import com.trefas.asledger.data.network.PullDto
import com.trefas.asledger.data.network.SnapshotAccountDto
import com.trefas.asledger.data.network.SnapshotTransactionDto
import com.trefas.asledger.util.encodeStringList
import com.trefas.asledger.util.normalizeSearch
import com.trefas.asledger.util.parseServerTime

/** Цепочка не сошлась: нужен полный снапшот, продолжать нельзя. */
class ChainMismatchException(message: String) : Exception(message)

/**
 * Применяет дельту pull в Room одной транзакцией.
 *
 * Перед записью проверяется хэш-цепочка по точным `ops_json` сервера.
 * Локальные pending/sending-проводки сохраняются: если сервер уже вернул такую
 * проводку, outbox помечается синхронизированным.
 */
class SyncApplier(
    private val database: AsledgerDatabase,
    private val now: () -> Long = System::currentTimeMillis
) {
    suspend fun apply(pull: PullDto, knownHash: String): Long {
        if (pull.formatVersion != SnapshotImporter.SUPPORTED_FORMAT_VERSION) {
            throw ChainMismatchException(
                "unsupported pull format ${pull.formatVersion}"
            )
        }
        val commits = pull.commits.orEmpty().sortedBy { it.resultSeq }
        val chainHead = verifyChain(
            prevHash = if (pull.since == 0L) "" else knownHash,
            commits = commits.map { it.toDomain() }
        ) ?: throw ChainMismatchException("hash chain mismatch")

        val expected = if (commits.isEmpty() && pull.since == 0L) "" else chainHead
        if (expected != pull.chainHash) {
            throw ChainMismatchException("final chain hash mismatch")
        }

        val accounts = pull.accounts.orEmpty().map { it.toEntity() }
            .sortedBy { it.fullPath }
        val transactions = pull.transactions.orEmpty()
        val timestamp = now()

        database.withTransaction {
            val meta = database.syncMetaDao().get()
            database.accountDao().clear()
            database.accountDao().upsertAll(accounts)

            val transactionDao = database.transactionDao()
            val outboxDao = database.outboxDao()
            transactions.forEach { dto ->
                transactionDao.deleteLines(dto.uuid)
                transactionDao.upsertAll(listOf(dto.toEntity()))
                transactionDao.upsertLines(dto.toLines())
                val outbox = outboxDao.byTransaction(dto.uuid)
                if (outbox != null && outbox.status == OutboxStatus.SENDING) {
                    outboxDao.upsert(
                        outbox.copy(
                            status = OutboxStatus.SYNCED,
                            lastError = null,
                            errorCode = null,
                            updatedAt = timestamp
                        )
                    )
                }
            }

            database.commitDao().upsertAll(commits.map { it.toEntity() })
            database.syncMetaDao().upsert(
                SyncMetaEntity(
                    headSeq = pull.headSeq,
                    chainHash = pull.chainHash,
                    formatVersion = pull.formatVersion,
                    initialLoaded = true,
                    lastSyncAt = timestamp,
                    localCounter = meta?.localCounter ?: 0
                )
            )
        }
        return pull.headSeq
    }

    private fun PullCommitDto.toDomain(): SyncCommit = SyncCommit(
        id = id,
        clientId = clientId,
        actorId = actorId,
        baseSeq = baseSeq,
        opsJson = opsJson,
        resultSeq = resultSeq,
        hash = hash,
        warnings = warnings.orEmpty(),
        createdAt = runCatching { parseServerTime(createdAt) }.getOrDefault(0)
    )

    private fun PullCommitDto.toEntity(): CommitEntity = CommitEntity(
        id = id,
        clientId = clientId,
        actorId = actorId,
        baseSeq = baseSeq,
        opsJson = opsJson,
        resultSeq = resultSeq,
        hash = hash,
        warningsJson = encodeStringList(warnings.orEmpty()),
        createdAt = runCatching { parseServerTime(createdAt) }.getOrDefault(0)
    )

    private fun SnapshotAccountDto.toEntity(): AccountEntity {
        val accountType = AccountType.fromServer(type)
            ?: throw SnapshotImportException("unknown account type $type at $fullPath")
        return AccountEntity(
            id = id,
            name = name,
            fullPath = fullPath,
            type = accountType.serverValue,
            parentId = parentId,
            isLeaf = isLeaf,
            minBalanceKop = minBalanceKop,
            isArchived = isArchived,
            isSystem = isSystem,
            createdAt = runCatching { parseServerTime(createdAt) }.getOrDefault(0),
            searchKey = normalizeSearch(name, fullPath)
        )
    }

    private fun SnapshotTransactionDto.toEntity(): TransactionEntity {
        if (uuid.isBlank()) throw SnapshotImportException("empty UUID in pull")
        return TransactionEntity(
            uuid = uuid,
            serverId = id,
            opType = opType,
            date = runCatching { parseServerTime(date) }
                .getOrElse { throw SnapshotImportException("invalid date for $uuid") },
            description = description,
            actorId = actorId,
            reversesId = reversesId,
            source = source,
            sourceHash = sourceHash,
            createdAt = runCatching { parseServerTime(createdAt) }.getOrDefault(0),
            commitId = commitId,
            searchKey = normalizeSearch(description, uuid)
        )
    }

    private fun SnapshotTransactionDto.toLines(): List<LineEntity> =
        lines.map { line ->
            if (line.side != "DEBIT" && line.side != "CREDIT") {
                throw SnapshotImportException("unknown side ${line.side} in $uuid")
            }
            LineEntity(
                serverId = line.id,
                transactionUuid = uuid,
                accountId = line.accountId,
                amountKop = line.amountKop,
                side = line.side
            )
        }
}
