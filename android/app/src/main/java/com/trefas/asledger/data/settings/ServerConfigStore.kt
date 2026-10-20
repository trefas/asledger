package com.trefas.asledger.data.settings

import kotlinx.coroutines.flow.Flow

/** Хранилище адреса сервера и метаданных привязки. */
interface ServerConfigStore {
    /** Текущее состояние или null, если сервер ещё не настроен. */
    val config: Flow<ServerConfig?>

    /**
     * Сохраняет адрес после успешной проверки `/health`.
     *
     * Идентификатор устройства сохраняется между попытками привязки, чтобы
     * повторный enroll перевыпускал токен, а не создавал новое устройство.
     * Данные прошлой сессии сбрасываются.
     */
    suspend fun saveServer(host: String, port: Int, serverVersion: String, formatVersion: Int)

    /** Сохраняет пользователя после успешной привязки. */
    suspend fun saveSession(userId: Long, userName: String, lastSyncAt: Long)

    /** Обновляет версию сервера из диагностики, не трогая сессию. */
    suspend fun saveServerInfo(serverVersion: String, formatVersion: Int)

    /** Отмечает время последней синхронизации. */
    suspend fun saveLastSync(epochMillis: Long)

    /** Завершает обучение (используется в вехе S14). */
    suspend fun setOnboarded()

    /** Забывает пользователя и токен, сохраняя адрес для перепривязки. */
    suspend fun clearSession()

    /** Забывает сервер полностью. */
    suspend fun clear()
}
