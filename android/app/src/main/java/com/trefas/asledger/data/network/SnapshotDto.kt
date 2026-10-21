package com.trefas.asledger.data.network

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable

/**
 * Полный снапшот книги.
 *
 * Сервер отдаёт поля Go-моделей в верхнем регистре (`ID`, `Name`, …), а
 * служебные поля — в snake_case. DTO фиксирует этот контракт явно, чтобы
 * переименование Kotlin-полей не ломало разбор.
 */
@Serializable
data class SnapshotDto(
    @SerialName("format_version") val formatVersion: Int = 0,
    @SerialName("head_seq") val headSeq: Long = 0,
    @SerialName("chain_hash") val chainHash: String = "",
    val accounts: List<SnapshotAccountDto> = emptyList(),
    val users: List<SnapshotUserDto> = emptyList(),
    val settings: Map<String, String> = emptyMap(),
    val transactions: List<SnapshotTransactionDto> = emptyList()
)

/** Счёт снапшота. */
@Serializable
data class SnapshotAccountDto(
    @SerialName("ID") val id: Long,
    @SerialName("Name") val name: String,
    @SerialName("FullPath") val fullPath: String,
    @SerialName("Type") val type: String,
    @SerialName("ParentID") val parentId: Long? = null,
    @SerialName("IsLeaf") val isLeaf: Boolean = false,
    @SerialName("MinBalance") val minBalanceKop: Long = 0,
    @SerialName("IsArchived") val isArchived: Boolean = false,
    @SerialName("IsSystem") val isSystem: Boolean = false,
    @SerialName("CreatedAt") val createdAt: String = ""
)

/** Пользователь снапшота. */
@Serializable
data class SnapshotUserDto(
    val id: Long,
    val name: String
)

/** Проводка снапшота. */
@Serializable
data class SnapshotTransactionDto(
    @SerialName("ID") val id: Long,
    @SerialName("UUID") val uuid: String,
    @SerialName("OpType") val opType: String,
    @SerialName("Date") val date: String,
    @SerialName("Description") val description: String = "",
    @SerialName("ActorID") val actorId: Long,
    @SerialName("ReversesID") val reversesId: Long? = null,
    @SerialName("Source") val source: String = "",
    @SerialName("SourceHash") val sourceHash: String = "",
    @SerialName("CreatedAt") val createdAt: String = "",
    @SerialName("CommitID") val commitId: Long = 0,
    @SerialName("Lines") val lines: List<SnapshotLineDto> = emptyList()
)

/** Строка проводки снапшота. */
@Serializable
data class SnapshotLineDto(
    @SerialName("ID") val id: Long,
    @SerialName("TransactionID") val transactionId: Long,
    @SerialName("AccountID") val accountId: Long,
    @SerialName("Amount") val amountKop: Long,
    @SerialName("Side") val side: String
)
