package com.trefas.asledger.data.security

/** Хранилище токена устройства. Открытое значение не покидает память. */
interface TokenStore {
    /** Шифрует и сохраняет токен. */
    suspend fun save(token: String)

    /** Возвращает расшифрованный токен или null, если его нет/он повреждён. */
    suspend fun load(): String?

    /** Удаляет токен (отзыв, «Забыть сервер»). */
    suspend fun clear()
}
