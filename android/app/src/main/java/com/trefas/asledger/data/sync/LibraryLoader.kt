package com.trefas.asledger.data.sync

import androidx.annotation.StringRes
import com.trefas.asledger.R
import com.trefas.asledger.data.network.ApiResult
import com.trefas.asledger.data.network.AsledgerApi
import com.trefas.asledger.data.network.ConnectionError
import com.trefas.asledger.data.repository.MOBILE_OP_TYPES
import com.trefas.asledger.data.repository.RoleRepository
import com.trefas.asledger.data.security.TokenStore
import com.trefas.asledger.data.settings.ServerConfig
import com.trefas.asledger.data.settings.ServerConfigStore
import kotlinx.coroutines.CancellationException

/** Прогресс первичной загрузки для экрана. */
data class LibraryProgress(
    @StringRes val titleRes: Int,
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
    private val roles: RoleRepository? = null,
    private val now: () -> Long = System::currentTimeMillis
) {
    suspend fun load(
        config: ServerConfig,
        onProgress: (LibraryProgress) -> Unit = {}
    ): LibraryLoadResult {
        val token = tokenStore.load()
            ?: return LibraryLoadResult.Failure(ConnectionError.Unauthorized)
        onProgress(LibraryProgress(R.string.progress_fetch_book, 5))
        return when (val result = api.snapshot(config, token)) {
            is ApiResult.Failure -> LibraryLoadResult.Failure(result.error)
            is ApiResult.Ok -> {
                onProgress(LibraryProgress(R.string.progress_parse_book, 20))
                try {
                    importer.import(result.value) { stage ->
                        onProgress(stage.toProgress())
                    }
                    // Время синхронизации уже сохранено в meta; DataStore нужен
                    // для экрана диагностики до открытия базы.
                    runCatching { configStore.saveLastSync(now()) }
                    runCatching { roles?.prefetch(config, token, MOBILE_OP_TYPES) }
                    onProgress(LibraryProgress(R.string.progress_done, 100))
                    LibraryLoadResult.Success
                } catch (cancelled: CancellationException) {
                    throw cancelled
                } catch (_: Exception) {
                    LibraryLoadResult.Failure(
                        ConnectionError.InvalidResponse(R.string.connection_detail_import_failed)
                    )
                }
            }
        }
    }
}

private fun ImportStage.toProgress(): LibraryProgress = when (this) {
    ImportStage.ACCOUNTS -> LibraryProgress(R.string.progress_accounts, 30)
    ImportStage.USERS -> LibraryProgress(R.string.progress_users, 45)
    ImportStage.SETTINGS -> LibraryProgress(R.string.progress_settings, 50)
    ImportStage.TRANSACTIONS -> LibraryProgress(R.string.progress_transactions, 60)
    ImportStage.FINALIZING -> LibraryProgress(R.string.progress_finalizing, 95)
}
