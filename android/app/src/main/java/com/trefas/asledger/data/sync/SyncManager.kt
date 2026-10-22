package com.trefas.asledger.data.sync

import androidx.room.withTransaction
import com.trefas.asledger.R
import com.trefas.asledger.data.db.AsledgerDatabase
import com.trefas.asledger.data.db.OutboxEntity
import com.trefas.asledger.data.db.OutboxStatus
import com.trefas.asledger.data.db.SyncMetaEntity
import com.trefas.asledger.data.network.ApiResult
import com.trefas.asledger.data.network.AsledgerApi
import com.trefas.asledger.data.network.ConnectionError
import com.trefas.asledger.data.network.PushOperationDto
import com.trefas.asledger.data.network.PushRequestDto
import com.trefas.asledger.data.network.PushResponseDto
import com.trefas.asledger.data.security.TokenStore
import com.trefas.asledger.data.settings.ServerConfig
import com.trefas.asledger.data.settings.ServerConfigStore
import com.trefas.asledger.util.encodeStringList
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import kotlinx.serialization.json.Json

private const val ERR_CONNECTION = "ERR_CONNECTION"

/** Фаза синхронизации для индикатора в интерфейсе. */
enum class SyncPhase {
    IDLE,
    SYNCING,
    SUCCESS,
    OFFLINE,
    REJECTED
}

/** Состояние синхронизации. */
data class SyncState(
    val phase: SyncPhase = SyncPhase.IDLE,
    val lastSyncAt: Long = 0
)

/** Итог одной попытки синхронизации. */
sealed interface SyncOutcome {
    data class Success(
        val warnings: List<String> = emptyList(),
        val duplicates: List<String> = emptyList()
    ) : SyncOutcome

    data class Rejected(
        val code: String,
        val message: String
    ) : SyncOutcome

    data class Failure(val error: ConnectionError) : SyncOutcome

    data object NothingToDo : SyncOutcome
}

/**
 * Отправляет очередь и применяет дельту сервера.
 *
 * Один пакет получает один commit_id. Мьютекс запрещает параллельную отправку
 * одних и тех же черновиков; после обрыва повтор идёт с тем же commit_id, а
 * сервер возвращает прежний результат.
 */
class SyncManager(
    private val database: AsledgerDatabase,
    private val api: AsledgerApi,
    private val tokenStore: TokenStore,
    private val importer: SnapshotImporter,
    private val applier: SyncApplier,
    private val configStore: ServerConfigStore,
    private val now: () -> Long = System::currentTimeMillis
) {
    private val json = Json {
        ignoreUnknownKeys = true
        explicitNulls = false
    }
    private val mutex = Mutex()
    private val _state = MutableStateFlow(SyncState())
    val state: StateFlow<SyncState> = _state.asStateFlow()

    /** Отправляет pending и затем подтягивает изменения других клиентов. */
    suspend fun sync(config: ServerConfig): SyncOutcome = mutex.withLock {
        if (!config.hasSession) return SyncOutcome.NothingToDo
        _state.value = _state.value.copy(phase = SyncPhase.SYNCING)
        val token = tokenStore.load()
            ?: return fail(ConnectionError.Unauthorized)

        val pushed = pushLocked(config, token)
        if (pushed is SyncOutcome.Failure || pushed is SyncOutcome.Rejected) {
            return pushed
        }

        val pulled = pullLocked(config, token)
        if (pulled is SyncOutcome.Failure) {
            // Проводки уже приняты сервером; покажем это, но попробуем pull
            // при следующем запуске или ручном обновлении.
            return pulled
        }
        if (pulled is SyncOutcome.Rejected) return pulled

        val warnings = (pushed as? SyncOutcome.Success)?.warnings.orEmpty()
        val duplicates = (pushed as? SyncOutcome.Success)?.duplicates.orEmpty()
        _state.value = SyncState(
            phase = SyncPhase.SUCCESS,
            lastSyncAt = now()
        )
        return SyncOutcome.Success(warnings, duplicates)
    }

    /** Только отправка очереди (кнопка «Отправить»). */
    suspend fun pushOnly(config: ServerConfig): SyncOutcome = mutex.withLock {
        if (!config.hasSession) return SyncOutcome.NothingToDo
        _state.value = _state.value.copy(phase = SyncPhase.SYNCING)
        val token = tokenStore.load()
            ?: return fail(ConnectionError.Unauthorized)
        val pushed = pushLocked(config, token)
        if (pushed is SyncOutcome.Success) {
            _state.value = SyncState(
                phase = SyncPhase.SUCCESS,
                lastSyncAt = now()
            )
        }
        pushed
    }

    /** Только pull (потянуть-обновить). */
    suspend fun pullOnly(config: ServerConfig): SyncOutcome = mutex.withLock {
        if (!config.hasSession) return SyncOutcome.NothingToDo
        _state.value = _state.value.copy(phase = SyncPhase.SYNCING)
        val token = tokenStore.load()
            ?: return fail(ConnectionError.Unauthorized)
        pullLocked(config, token)
    }

    /** Повторяет синхронизацию после ошибки. */
    suspend fun retry(config: ServerConfig): SyncOutcome = sync(config)

    /**
     * Сторно подтверждённой проводки.
     *
     * Сервер создаёт обратную проводку, после чего pull приносит её в журнал.
     */
    suspend fun reverse(config: ServerConfig, uuid: String): SyncOutcome {
        if (!config.hasSession) return SyncOutcome.NothingToDo
        _state.value = _state.value.copy(phase = SyncPhase.SYNCING)
        val token = tokenStore.load()
            ?: return fail(ConnectionError.Unauthorized)
        return when (val result = api.reverse(config, token, uuid)) {
            is ApiResult.Failure -> fail(result.error)
            is ApiResult.Ok -> pullOnly(config)
        }
    }

    private suspend fun pushLocked(config: ServerConfig, token: String): SyncOutcome {
        val active = database.outboxDao().active()
        if (active.isEmpty()) return SyncOutcome.Success()
        val meta = database.syncMetaDao().get()
        val prepared = prepareBatch(active, config.clientId, meta)
        val operations = prepared.mapNotNull { row ->
            runCatching { json.decodeFromString<PushOperationDto>(row.opsJson) }.getOrNull()
        }
        if (operations.size != prepared.size) {
            return fail(ConnectionError.InvalidResponse(R.string.connection_detail_broken_outbox))
        }
        val batchId = prepared.first().commitId
        val request = PushRequestDto(
            baseSeq = prepared.first().baseSeq,
            commitId = batchId,
            operations = operations
        )

        return when (val result = api.push(config, token, request)) {
            is ApiResult.Failure -> {
                markFailed(prepared, keepSending = false)
                fail(result.error)
            }

            is ApiResult.Ok -> {
                val response = result.value
                if (response.status == "rejected") {
                    markRejected(prepared, response)
                    val code = response.error?.code ?: "ERR_SYNC_REJECTED"
                    val message = response.error?.message.orEmpty()
                    _state.value = SyncState(
                        phase = SyncPhase.REJECTED,
                        lastSyncAt = now()
                    )
                    SyncOutcome.Rejected(code, message)
                } else {
                    markSynced(prepared, response)
                    SyncOutcome.Success(
                        warnings = response.warnings.orEmpty(),
                        duplicates = response.duplicates.orEmpty()
                            .flatMap { it.existing }
                            .distinct()
                    )
                }
            }
        }
    }

    /**
     * Готовит один пакет: у всех записей будет общий commit_id.
     *
     * Если после прошлой попытки записи уже имеют один commit_id, он
     * переиспользуется. Иначе выдаётся новый номер и сохраняется в meta.
     */
    private suspend fun prepareBatch(
        active: List<OutboxEntity>,
        clientId: String,
        meta: SyncMetaEntity?
    ): List<OutboxEntity> {
        val existingIds = active.map { it.commitId }.distinct()
        val timestamp = now()
        return database.withTransaction {
            val current = meta ?: SyncMetaEntity(
                headSeq = 0,
                chainHash = "",
                formatVersion = 1,
                initialLoaded = false,
                lastSyncAt = 0
            )
            val batchId = if (existingIds.size == 1) {
                existingIds.single()
            } else {
                val number = current.localCounter + 1
                database.syncMetaDao().upsert(current.copy(localCounter = number))
                "$clientId:$number"
            }
            val rows = active.map { row ->
                row.copy(
                    commitId = batchId,
                    baseSeq = current.headSeq,
                    status = OutboxStatus.SENDING,
                    attempts = row.attempts + 1,
                    lastError = null,
                    errorCode = null,
                    updatedAt = timestamp
                )
            }
            rows.forEach { database.outboxDao().upsert(it) }
            rows
        }
    }

    private suspend fun markSynced(rows: List<OutboxEntity>, response: PushResponseDto) {
        val warningsJson = encodeStringList(response.warnings.orEmpty())
        val timestamp = now()
        database.withTransaction {
            rows.forEach { row ->
                val duplicates = response.duplicates.orEmpty()
                    .filter { it.uuid == row.transactionUuid }
                    .flatMap { it.existing }
                database.outboxDao().upsert(
                    row.copy(
                        status = OutboxStatus.SYNCED,
                        lastError = null,
                        errorCode = null,
                        warningsJson = warningsJson,
                        duplicateUuidsJson = encodeStringList(duplicates),
                        updatedAt = timestamp
                    )
                )
            }
        }
    }

    private suspend fun markRejected(rows: List<OutboxEntity>, response: PushResponseDto) {
        val timestamp = now()
        val warningsJson = encodeStringList(response.warnings.orEmpty())
        database.withTransaction {
            rows.forEach { row ->
                database.outboxDao().upsert(
                    row.copy(
                        status = OutboxStatus.REJECTED,
                        lastError = response.error?.message,
                        errorCode = response.error?.code,
                        warningsJson = warningsJson,
                        updatedAt = timestamp
                    )
                )
            }
        }
    }

    private suspend fun markFailed(
        rows: List<OutboxEntity>,
        keepSending: Boolean
    ) {
        val timestamp = now()
        database.withTransaction {
            rows.forEach { row ->
                database.outboxDao().upsert(
                    row.copy(
                        status = if (keepSending) row.status else OutboxStatus.PENDING,
                        lastError = null,
                        errorCode = ERR_CONNECTION,
                        updatedAt = timestamp
                    )
                )
            }
        }
    }

    private suspend fun pullLocked(config: ServerConfig, token: String): SyncOutcome {
        val meta = database.syncMetaDao().get()
        if (meta == null || !meta.initialLoaded) {
            return fullResync(config, token)
        }
        return when (val result = api.pull(config, token, meta.headSeq, meta.chainHash)) {
            is ApiResult.Failure -> fail(result.error)
            is ApiResult.Ok -> {
                val pull = result.value
                if (pull.resyncRequired) {
                    return fullResync(config, token)
                }
                try {
                    applier.apply(pull, meta.chainHash)
                    configStore.saveLastSync(now())
                    _state.value = SyncState(
                        phase = SyncPhase.SUCCESS,
                        lastSyncAt = now()
                    )
                    SyncOutcome.Success()
                } catch (cancelled: CancellationException) {
                    throw cancelled
                } catch (_: ChainMismatchException) {
                    fullResync(config, token)
                } catch (_: Exception) {
                    fail(
                        ConnectionError.InvalidResponse(R.string.connection_detail_apply_failed)
                    )
                }
            }
        }
    }

    private suspend fun fullResync(
        config: ServerConfig,
        token: String
    ): SyncOutcome = when (val snapshot = api.snapshot(config, token)) {
        is ApiResult.Failure -> fail(snapshot.error)
        is ApiResult.Ok -> try {
            importer.import(snapshot.value)
            configStore.saveLastSync(now())
            _state.value = SyncState(
                phase = SyncPhase.SUCCESS,
                lastSyncAt = now()
            )
            SyncOutcome.Success()
        } catch (cancelled: CancellationException) {
            throw cancelled
        } catch (_: Exception) {
            fail(
                ConnectionError.InvalidResponse(R.string.connection_detail_reload_failed)
            )
        }
    }

    private fun fail(error: ConnectionError): SyncOutcome {
        _state.value = SyncState(phase = SyncPhase.OFFLINE)
        return SyncOutcome.Failure(error)
    }
}
