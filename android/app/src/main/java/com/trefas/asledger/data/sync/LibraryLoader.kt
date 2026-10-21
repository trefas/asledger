package com.trefas.asledger.data.sync

import com.trefas.asledger.data.network.ApiResult
import com.trefas.asledger.data.network.AsledgerApi
import com.trefas.asledger.data.network.ConnectionError
import com.trefas.asledger.data.security.TokenStore
import com.trefas.asledger.data.settings.ServerConfig
import com.trefas.asledger.data.settings.ServerConfigStore
import kotlinx.coroutines.CancellationException

/** Прогресс первичной загрузки для экрана. */
data class LibraryProgress(
    val title: String,
    val percent: Int
)

/** Результат загрузки книги. */
sealed interface LibraryLoadResult {
    data object Success : LibraryLoadResult
    data class Failure(val error: ConnectionError) : LibraryLoadResult
}

/**
 * Загружает снапшот и импортирует его в Room.
 *
 * Сетевой слой и импорт остаются раздельными: экран видит только прогресс и
 * результат, а повторный вызов безопасен — снапшот идемпотентен по UUID.
 */
class LibraryLoader(
    private val api: AsledgerApi,
    private val tokenStore: TokenStore,
    private val importer: SnapshotImporter,
    private val configStore: ServerConfigStore,
    private val now: () -> Long = System::currentTimeMillis
) {
    suspend fun load(
        config: ServerConfig,
        onProgress: (LibraryProgress) -> Unit = {}
    ): LibraryLoadResult {
        val token = tokenStore.load()
            ?: return LibraryLoadResult.Failure(ConnectionError.Unauthorized)
        onProgress(LibraryProgress("Получаем книгу с сервера", 5))
        return when (val result = api.snapshot(config, token)) {
            is ApiResult.Failure -> LibraryLoadResult.Failure(result.error)
            is ApiResult.Ok -> {
                onProgress(LibraryProgress("Разбираем счета и проводки", 20))
                try {
                    importer.import(result.value) { stage ->
                        onProgress(stage.toProgress())
                    }
                    // Время синхронизации уже сохранено в meta; DataStore нужен
                    // для экрана диагностики до открытия базы.
                    runCatching { configStore.saveLastSync(now()) }
                    onProgress(LibraryProgress("Готово", 100))
                    LibraryLoadResult.Success
                } catch (cancelled: CancellationException) {
                    throw cancelled
                } catch (failure: Exception) {
                    LibraryLoadResult.Failure(
                        ConnectionError.InvalidResponse(
                            failure.message ?: "не удалось импортировать книгу"
                        )
                    )
                }
            }
        }
    }
}

private fun ImportStage.toProgress(): LibraryProgress = when (this) {
    ImportStage.ACCOUNTS -> LibraryProgress("Сохраняем план счетов", 30)
    ImportStage.USERS -> LibraryProgress("Сохраняем пользователей", 45)
    ImportStage.SETTINGS -> LibraryProgress("Сохраняем настройки", 50)
    ImportStage.TRANSACTIONS -> LibraryProgress("Сохраняем проводки", 60)
    ImportStage.FINALIZING -> LibraryProgress("Проверяем данные", 95)
}
