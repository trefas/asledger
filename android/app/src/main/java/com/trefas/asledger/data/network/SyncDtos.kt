package com.trefas.asledger.data.network

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable

/**
 * Запрос `POST /sync/push`.
 *
 * `commit_id` постоянен для пакета: повтор после обрыва связи возвращает тот же
 * ответ и не создаёт дубль на сервере.
 */
@Serializable
data class PushRequestDto(
    @SerialName("base_seq") val baseSeq: Long,
    @SerialName("commit_id") val commitId: String,
    val operations: List<PushOperationDto>
)

/** Операция пакета. */
@Serializable
data class PushOperationDto(
    @SerialName("op_type") val opType: String,
    val uuid: String,
    val date: String,
    val description: String = "",
    val direction: String? = null,
    val entries: List<PushEntryDto>
)

/** Строка операции: роль назначает сторону на сервере. */
@Serializable
data class PushEntryDto(
    @SerialName("account_id") val accountId: Long,
    @SerialName("amount_kop") val amountKop: Long,
    val role: String
)

/** Возможный дубль в ответе push. */
@Serializable
data class PushDuplicateDto(
    val uuid: String = "",
    val existing: List<String> = emptyList()
)

/** Ответ push; при 422 приходит статус rejected и error. */
@Serializable
data class PushResponseDto(
    val status: String = "",
    @SerialName("commit_id") val commitId: String = "",
    @SerialName("result_seq") val resultSeq: Long = 0,
    val applied: List<String>? = emptyList(),
    val warnings: List<String>? = emptyList(),
    val duplicates: List<PushDuplicateDto>? = emptyList(),
    val error: ApiErrorBody? = null
)

/** Запись журнала изменений. */
@Serializable
data class PullChangeDto(
    @SerialName("Seq") val seq: Long = 0,
    @SerialName("Entity") val entity: String = "",
    @SerialName("EntityID") val entityId: Long = 0,
    @SerialName("Op") val op: String = "",
    @SerialName("ChangedAt") val changedAt: String = ""
)

/** Коммит сервера для проверки хэш-цепочки. */
@Serializable
data class PullCommitDto(
    @SerialName("ID") val id: Long = 0,
    @SerialName("ClientID") val clientId: String = "",
    @SerialName("ActorID") val actorId: Long = 0,
    @SerialName("BaseSeq") val baseSeq: Long = 0,
    @SerialName("OpsJSON") val opsJson: String = "",
    @SerialName("ResultSeq") val resultSeq: Long = 0,
    @SerialName("Hash") val hash: String = "",
    @SerialName("Warnings") val warnings: List<String>? = emptyList(),
    @SerialName("CreatedAt") val createdAt: String = ""
)

/** Ответ `GET /sync/pull`. */
@Serializable
data class PullDto(
    @SerialName("format_version") val formatVersion: Int = 0,
    @SerialName("since") val since: Long = 0,
    @SerialName("head_seq") val headSeq: Long = 0,
    @SerialName("chain_hash") val chainHash: String = "",
    val changes: List<PullChangeDto>? = emptyList(),
    val transactions: List<SnapshotTransactionDto>? = emptyList(),
    val accounts: List<SnapshotAccountDto>? = emptyList(),
    @SerialName("accounts_read_only") val accountsReadOnly: Boolean = false,
    val commits: List<PullCommitDto>? = emptyList(),
    @SerialName("resync_required") val resyncRequired: Boolean = false
)

/** Роль строки в матрице ASL-2.2. */
@Serializable
data class EngineRoleDto(
    val role: String = "",
    val side: String = "",
    @SerialName("account_types") val accountTypes: List<String> = emptyList(),
    @SerialName("direction_required") val directionRequired: Boolean = false,
    val reserved: Boolean = false
)

/** Ответ `GET /engine/roles`. */
@Serializable
data class EngineRolesDto(
    @SerialName("op_type") val opType: String = "",
    val title: String = "",
    val roles: List<EngineRoleDto>? = emptyList(),
    @SerialName("min_debit") val minDebit: Int = 1,
    @SerialName("min_credit") val minCredit: Int = 1,
    val reserved: Map<String, List<String>>? = emptyMap(),
    @SerialName("accounts_read_only") val accountsReadOnly: Boolean = false
)
