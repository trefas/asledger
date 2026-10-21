package com.trefas.asledger.data.repository

import com.trefas.asledger.data.db.AccountEntity
import com.trefas.asledger.data.db.SettingEntity
import com.trefas.asledger.data.db.SyncMetaEntity
import com.trefas.asledger.data.db.UserEntity
import com.trefas.asledger.data.model.Account
import com.trefas.asledger.data.model.AccountType
import com.trefas.asledger.data.model.LibrarySetting
import com.trefas.asledger.data.model.LibraryUser
import com.trefas.asledger.data.model.SyncMeta

/** Преобразования между Room и доменными моделями. */
internal fun AccountEntity.toDomain(): Account = Account(
    id = id,
    name = name,
    fullPath = fullPath,
    type = AccountType.fromServer(type)
        ?: throw IllegalStateException("неизвестный тип счёта: $type"),
    parentId = parentId,
    isLeaf = isLeaf,
    minBalanceKop = minBalanceKop,
    isArchived = isArchived,
    isSystem = isSystem,
    createdAt = createdAt
)

internal fun UserEntity.toDomain(): LibraryUser = LibraryUser(id = id, name = name)

internal fun SettingEntity.toDomain(): LibrarySetting =
    LibrarySetting(key = key, value = value)

internal fun SyncMetaEntity.toDomain(): SyncMeta = SyncMeta(
    headSeq = headSeq,
    chainHash = chainHash,
    formatVersion = formatVersion,
    initialLoaded = initialLoaded,
    lastSyncAt = lastSyncAt
)
