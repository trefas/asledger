package com.trefas.asledger.data.settings

import kotlinx.coroutines.flow.Flow

/** Хранилище адреса сервера. */
interface ServerConfigStore {
    /** Текущий адрес или null, если книга ещё не подключена. */
    val config: Flow<ServerConfig?>

    /** Сохраняет адрес, нормализуя хост. */
    suspend fun save(config: ServerConfig)

    /** Забывает адрес (переход к экрану подключения). */
    suspend fun clear()
}
