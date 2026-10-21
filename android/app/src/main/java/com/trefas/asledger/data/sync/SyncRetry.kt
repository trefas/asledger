package com.trefas.asledger.data.sync

import com.trefas.asledger.data.network.ConnectionError

/**
 * Политика повторных попыток фоновой синхронизации.
 *
 * Повторяем только временные сетевые ошибки и не больше [MAX_ATTEMPTS] раз,
 * чтобы телефон не будил радиосвязь бесконечно при выключенном сервере.
 */
object SyncRetry {
    /** Предельное число запусков одной задачи WorkManager. */
    const val MAX_ATTEMPTS = 8

    /** Начальная задержка экспоненциального backoff. */
    const val INITIAL_BACKOFF_SECONDS = 30L

    /** Временная ли ошибка, при которой имеет смысл повторить запрос. */
    fun isTransient(error: ConnectionError): Boolean = when (error) {
        ConnectionError.NetworkUnavailable,
        ConnectionError.HostNotFound,
        ConnectionError.ConnectionRefused,
        ConnectionError.Timeout -> true
        else -> false
    }

    /**
     * Нужен ли ещё один запуск.
     *
     * [attempt] — значение `runAttemptCount` до текущего запуска: 0 для
     * первой попытки. После [MAX_ATTEMPTS] задача завершается успешно, а
     * очередь остаётся на устройстве и отправится вручную или при следующем
     * подключении.
     */
    fun shouldRetry(error: ConnectionError, attempt: Int): Boolean =
        isTransient(error) && attempt + 1 < MAX_ATTEMPTS
}
