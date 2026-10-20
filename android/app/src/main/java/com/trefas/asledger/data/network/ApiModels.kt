package com.trefas.asledger.data.network

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable

/** Ответ `GET /api/v1/health`. */
@Serializable
data class HealthInfo(
    val status: String = "",
    val version: String = "",
    @SerialName("format_version") val formatVersion: Int = 0
)

/** Тело ошибки API. */
@Serializable
data class ApiErrorBody(
    val code: String = "",
    val message: String = ""
)

/** Конверт ошибки API. */
@Serializable
data class ApiErrorEnvelope(
    val error: ApiErrorBody = ApiErrorBody()
)

/** Пользователь в ответе выбора при привязке. */
@Serializable
data class EnrollUser(
    val id: Long,
    val name: String
)

/** Запрос привязки устройства. */
@Serializable
data class EnrollRequest(
    @SerialName("client_id") val clientId: String,
    @SerialName("device_name") val deviceName: String,
    @SerialName("user_id") val userId: Long? = null
)

/** Успешная привязка устройства. */
@Serializable
data class EnrollResponse(
    val token: String = "",
    @SerialName("client_id") val clientId: String = "",
    @SerialName("user_id") val userId: Long = 0,
    @SerialName("user_name") val userName: String = ""
)

/** Ответ 409, когда нужно выбрать пользователя. */
@Serializable
data class EnrollChoiceResponse(
    val error: ApiErrorBody = ApiErrorBody(),
    val users: List<EnrollUser> = emptyList()
)

/**
 * Заголовок снапшота.
 *
 * Полный снапшот содержит счета, пользователей и проводки; для проверки
 * совместимости достаточно `format_version`. Неизвестные поля игнорируются.
 */
@Serializable
data class SnapshotHeader(
    @SerialName("format_version") val formatVersion: Int = 0,
    @SerialName("head_seq") val headSeq: Long = 0,
    @SerialName("chain_hash") val chainHash: String = ""
)
