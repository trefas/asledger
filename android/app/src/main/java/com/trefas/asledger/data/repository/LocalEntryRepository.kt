package com.trefas.asledger.data.repository

import androidx.room.withTransaction
import com.trefas.asledger.data.db.AsledgerDatabase
import com.trefas.asledger.data.db.LineEntity
import com.trefas.asledger.data.db.OutboxEntity
import com.trefas.asledger.data.db.OutboxStatus
import com.trefas.asledger.data.db.SyncMetaEntity
import com.trefas.asledger.data.db.TransactionEntity
import com.trefas.asledger.data.model.EntryDraft
import com.trefas.asledger.data.model.OutboxItem
import com.trefas.asledger.data.model.SyncStatus
import com.trefas.asledger.data.network.PushEntryDto
import com.trefas.asledger.data.network.PushOperationDto
import com.trefas.asledger.util.decodeStringList
import com.trefas.asledger.util.normalizeSearch
import java.time.Instant
import java.util.UUID
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.map
import kotlinx.serialization.encodeToString
import kotlinx.serialization.json.Json

/**
 * Черновики и очередь отправки.
 *
 * Проводка и запись outbox создаются в одной Room-транзакции: не бывает
 * черновика без очереди и очереди без проводки. Номер коммита хранится в
 * `sync_meta`, поэтому переживает перезапуск приложения.
 */
class LocalEntryRepository(
    private val database: AsledgerDatabase,
    private val now: () -> Long = System::currentTimeMillis
) {
    private val json = Json {
        ignoreUnknownKeys = true
        explicitNulls = false
        encodeDefaults = true
    }

    /** Сохраняет новый черновик и возвращает его UUID. */
    suspend fun save(
        draft: EntryDraft,
        clientId: String,
        actorId: Long
    ): String = database.withTransaction {
        require(draft.lines.isNotEmpty()) { "проводка без строк" }
        val timestamp = now()
        val meta = database.syncMetaDao().get() ?: emptyMeta()
        val number = meta.localCounter + 1
        database.syncMetaDao().upsert(meta.copy(localCounter = number))
        val uuid = draft.uuid ?: UUID.randomUUID().toString()
        val transaction = draft.toEntity(
            uuid = uuid,
            actorId = actorId,
            createdAt = timestamp
        )
        val lines = draft.toEntities(uuid)
        database.transactionDao().upsertAll(listOf(transaction))
        database.transactionDao().deleteLines(uuid)
        database.transactionDao().upsertLines(lines)
        database.outboxDao().upsert(
            OutboxEntity(
                commitId = commitId(clientId, number),
                transactionUuid = uuid,
                clientId = clientId,
                baseSeq = meta.headSeq,
                opsJson = operationJson(draft, uuid),
                status = OutboxStatus.PENDING,
                createdAt = timestamp,
                updatedAt = timestamp
            )
        )
        uuid
    }

    /**
     * Обновляет ожидающий или отклонённый черновик.
     *
     * После изменения содержимого выдаётся новый commit_id: старый ответ
     * сервера уже не соответствует новому пакету.
     */
    suspend fun update(
        uuid: String,
        draft: EntryDraft,
        clientId: String,
        actorId: Long
    ) = database.withTransaction {
        val outbox = database.outboxDao().byTransaction(uuid)
            ?: throw IllegalStateException("проводка не является черновиком")
        if (outbox.status == OutboxStatus.SYNCED) {
            throw IllegalStateException("синхронизированную проводку нельзя редактировать")
        }
        if (outbox.status == OutboxStatus.SENDING) {
            throw IllegalStateException("проводка уже отправляется")
        }
        val existing = database.transactionDao().get(uuid)
            ?: throw IllegalStateException("черновик не найден")
        val timestamp = now()
        val meta = database.syncMetaDao().get() ?: emptyMeta()
        val number = meta.localCounter + 1
        database.syncMetaDao().upsert(meta.copy(localCounter = number))
        database.transactionDao().upsertAll(
            listOf(
                draft.toEntity(
                    uuid = uuid,
                    actorId = actorId,
                    createdAt = existing.createdAt
                )
            )
        )
        database.transactionDao().deleteLines(uuid)
        database.transactionDao().upsertLines(draft.toEntities(uuid))
        database.outboxDao().upsert(
            outbox.copy(
                commitId = commitId(clientId, number),
                baseSeq = meta.headSeq,
                opsJson = operationJson(draft, uuid),
                status = OutboxStatus.PENDING,
                attempts = 0,
                lastError = null,
                errorCode = null,
                warningsJson = "[]",
                duplicateUuidsJson = "[]",
                updatedAt = timestamp
            )
        )
    }

    /** Удаляет неотправленный черновик вместе со строками и outbox. */
    suspend fun delete(uuid: String) = database.withTransaction {
        val outbox = database.outboxDao().byTransaction(uuid)
            ?: throw IllegalStateException("очередь не найдена")
        if (outbox.status == OutboxStatus.SYNCED) {
            throw IllegalStateException("синхронизированную проводку удалить нельзя")
        }
        if (outbox.status == OutboxStatus.SENDING) {
            throw IllegalStateException("проводка уже отправляется")
        }
        database.transactionDao().deleteLines(uuid)
        database.outboxDao().deleteByTransaction(uuid)
        database.transactionDao().delete(uuid)
    }

    /** Все записи очереди для экрана «Синхронизация». */
    fun observeItems(): Flow<List<OutboxItem>> =
        database.outboxDao().observeRows().map { rows ->
            rows.map { row ->
                OutboxItem(
                    transactionUuid = row.outbox.transactionUuid,
                    commitId = row.outbox.commitId,
                    status = SyncStatus.fromServer(row.outbox.status),
                    attempts = row.outbox.attempts,
                    lastError = row.outbox.lastError,
                    errorCode = row.outbox.errorCode,
                    warnings = decodeStringList(row.outbox.warningsJson),
                    duplicateUuids = decodeStringList(row.outbox.duplicateUuidsJson),
                    description = row.description.orEmpty(),
                    amountKop = row.amountKop ?: 0,
                    date = row.date ?: row.outbox.createdAt
                )
            }
        }

    private fun operationJson(draft: EntryDraft, uuid: String): String =
        json.encodeToString(
            PushOperationDto(
                opType = draft.opType,
                uuid = uuid,
                date = Instant.ofEpochMilli(draft.date).toString(),
                description = draft.description,
                entries = draft.lines.map { line ->
                    PushEntryDto(
                        accountId = line.accountId,
                        amountKop = line.amountKop,
                        role = line.role
                    )
                }
            )
        )

    private fun EntryDraft.toEntity(
        uuid: String,
        actorId: Long,
        createdAt: Long
    ): TransactionEntity {
        require(opType.isNotBlank()) { "не выбран тип операции" }
        require(lines.all { it.accountId > 0 && it.amountKop > 0 }) {
            "все строки должны содержать счёт и положительную сумму"
        }
        return TransactionEntity(
            uuid = uuid,
            serverId = null,
            opType = opType,
            date = date,
            description = description,
            actorId = actorId,
            reversesId = null,
            source = "android",
            sourceHash = "",
            createdAt = createdAt,
            commitId = 0,
            searchKey = normalizeSearch(description, opType)
        )
    }

    private fun EntryDraft.toEntities(uuid: String): List<LineEntity> =
        lines.map { line ->
            require(line.side == "DEBIT" || line.side == "CREDIT") {
                "неизвестная сторона строки"
            }
            LineEntity(
                serverId = null,
                transactionUuid = uuid,
                accountId = line.accountId,
                amountKop = line.amountKop,
                side = line.side,
                role = line.role
            )
        }

    private fun emptyMeta() = SyncMetaEntity(
        headSeq = 0,
        chainHash = "",
        formatVersion = 1,
        initialLoaded = false,
        lastSyncAt = 0,
        localCounter = 0
    )

    private fun commitId(clientId: String, number: Long): String = "$clientId:$number"
}
