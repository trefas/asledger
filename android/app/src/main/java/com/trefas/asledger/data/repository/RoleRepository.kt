package com.trefas.asledger.data.repository

import com.trefas.asledger.data.db.SettingDao
import com.trefas.asledger.data.db.SettingEntity
import com.trefas.asledger.data.model.AccountType
import com.trefas.asledger.data.model.OperationRoles
import com.trefas.asledger.data.model.RoleRule
import com.trefas.asledger.data.network.ApiResult
import com.trefas.asledger.data.network.AsledgerApi
import com.trefas.asledger.data.network.EngineRolesDto
import com.trefas.asledger.data.settings.ServerConfig
import kotlinx.serialization.encodeToString
import kotlinx.serialization.json.Json

/**
 * Матрица ролей ASL-2.2.
 *
 * После первой загрузки ответ сервера кэшируется в настройках Room, чтобы
 * создать проводку можно было офлайн. Источник истины остаётся на сервере:
 * при следующем онлайн-запуске кэш обновляется.
 */
class RoleRepository(
    private val settings: SettingDao,
    private val api: AsledgerApi
) {
    private val json = Json {
        ignoreUnknownKeys = true
        explicitNulls = false
        encodeDefaults = true
    }

    /** Роли из кэша без сети; null, если тип ещё ни разу не загружался. */
    suspend fun cached(opType: String): OperationRoles? {
        val raw = settings.value(key(opType)) ?: return null
        return runCatching { json.decodeFromString<EngineRolesDto>(raw).toDomain() }.getOrNull()
    }

    /** Загружает роли с сервера и обновляет кэш. */
    suspend fun fetch(
        config: ServerConfig,
        token: String,
        opType: String
    ): ApiResult<OperationRoles> = when (val result = api.roles(config, token, opType)) {
        is ApiResult.Failure -> result
        is ApiResult.Ok -> {
            settings.upsertAll(
                listOf(SettingEntity(key(opType), json.encodeToString(result.value)))
            )
            ApiResult.Ok(result.value.toDomain())
        }
    }

    /** Обновляет кэш для типов первой версии; сетевые ошибки не блокируют вход. */
    suspend fun prefetch(config: ServerConfig, token: String, opTypes: List<String>) {
        opTypes.forEach { opType ->
            fetch(config, token, opType)
        }
    }

    private fun key(opType: String) = "android.roles.$opType"
}

/** Преобразует ответ сервера в доменную матрицу. */
fun EngineRolesDto.toDomain(): OperationRoles = OperationRoles(
    opType = opType,
    title = title,
    roles = roles.orEmpty().map { role ->
        RoleRule(
            role = role.role,
            side = role.side,
            accountTypes = role.accountTypes.mapNotNull { AccountType.fromServer(it) },
            directionRequired = role.directionRequired,
            reserved = role.reserved
        )
    }.filterNot { it.reserved },
    minDebit = minDebit,
    minCredit = minCredit
)

/** Типы операций, доступные в мобильной форме первой версии. */
val MOBILE_OP_TYPES = listOf("трата", "доход", "перевод")
