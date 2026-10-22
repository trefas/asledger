package com.trefas.asledger.data

import android.content.Context
import com.trefas.asledger.data.db.AsledgerDatabase
import com.trefas.asledger.data.network.AsledgerApi
import com.trefas.asledger.data.network.createHttpClient
import com.trefas.asledger.data.repository.AccountRepository
import com.trefas.asledger.data.repository.BalanceRepository
import com.trefas.asledger.data.repository.LocalEntryRepository
import com.trefas.asledger.data.repository.RoleRepository
import com.trefas.asledger.data.repository.TransactionRepository
import com.trefas.asledger.data.security.KeystoreTokenStore
import com.trefas.asledger.data.security.TokenStore
import com.trefas.asledger.data.settings.DataStoreServerConfigStore
import com.trefas.asledger.data.settings.HintStore
import com.trefas.asledger.data.settings.ServerConfigStore
import com.trefas.asledger.data.settings.SharedPreferencesHintStore
import com.trefas.asledger.data.sync.LibraryLoader
import com.trefas.asledger.data.sync.SnapshotImporter
import com.trefas.asledger.data.sync.SyncApplier
import com.trefas.asledger.data.sync.SyncManager
import com.trefas.asledger.data.sync.SyncScheduler
import io.ktor.client.HttpClient
import io.ktor.client.engine.okhttp.OkHttp
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.MutableSharedFlow
import kotlinx.coroutines.flow.SharedFlow
import kotlinx.coroutines.flow.asSharedFlow
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.launch

/**
 * Ручной контейнер зависимостей.
 *
 * В нём настройки сервера, зашифрованный токен, HTTP-клиент, API и локальная
 * реплика Room. Очередь отправки появится в S12.
 */
class AppContainer(context: Context) {
    /** Контекст приложения: не держим Activity. */
    val appContext: Context = context.applicationContext

    /** Адрес, пользователь и метаданные привязки. */
    val serverConfig: ServerConfigStore = DataStoreServerConfigStore(appContext)

    /** Токен устройства; в файлах лежит только шифротекст Keystore. */
    val tokenStore: TokenStore = KeystoreTokenStore(appContext)

    /** Отметки о просмотренных обучающих подсказках. */
    val hintStore: HintStore = SharedPreferencesHintStore(appContext)

    /** HTTP-клиент для REST API asledger. */
    val httpClient: HttpClient by lazy { createHttpClient(OkHttp.create { }) }

    private val appScope = CoroutineScope(SupervisorJob() + Dispatchers.IO)
    private val _sessionExpired = MutableSharedFlow<Unit>(extraBufferCapacity = 1)

    /** Событие «сервер ответил 401»: UI переводит на повторную привязку. */
    val sessionExpired: SharedFlow<Unit> = _sessionExpired.asSharedFlow()

    /** Клиент API; при 401 сессия очищается, локальные черновики сохраняются. */
    val api: AsledgerApi = AsledgerApi(httpClient) {
        appScope.launch { expireSession() }
    }

    /** Локальная реплика книги. */
    val database: AsledgerDatabase by lazy { AsledgerDatabase.create(appContext) }

    val accounts: AccountRepository by lazy {
        AccountRepository(database.accountDao())
    }

    val transactions: TransactionRepository by lazy {
        TransactionRepository(database.transactionDao(), database.outboxDao())
    }

    val balances: BalanceRepository by lazy {
        BalanceRepository(database.accountDao(), database.transactionDao())
    }

    /** Черновики и очередь отправки. */
    val localEntries: LocalEntryRepository by lazy {
        LocalEntryRepository(database)
    }

    /** Кэш ролей ASL-2.2 для офлайн-ввода. */
    val roles: RoleRepository by lazy {
        RoleRepository(database.settingDao(), api)
    }

    /** Импорт полного снапшота одной транзакцией. */
    val snapshotImporter: SnapshotImporter by lazy {
        SnapshotImporter(database)
    }

    /** Применение дельты pull. */
    val syncApplier: SyncApplier by lazy {
        SyncApplier(database)
    }

    /** Отправка очереди и pull. */
    val syncManager: SyncManager by lazy {
        SyncManager(
            database = database,
            api = api,
            tokenStore = tokenStore,
            importer = snapshotImporter,
            applier = syncApplier,
            configStore = serverConfig
        )
    }

    /** Первичная загрузка книги после привязки. */
    val libraryLoader: LibraryLoader by lazy {
        LibraryLoader(api, tokenStore, snapshotImporter, serverConfig, roles)
    }

    /** true, когда первичный снапшот уже импортирован. */
    val libraryLoaded: Flow<Boolean> = database.syncMetaDao()
        .observe()
        .map { it?.initialLoaded == true }

    /** Ставит немедленную синхронизацию в очередь WorkManager. */
    fun requestSync() {
        runCatching { SyncScheduler.requestNow(appContext) }
    }

    /** Забывает пользователя и токен, сохраняя адрес сервера. */
    suspend fun forgetSession() {
        tokenStore.clear()
        serverConfig.clearSession()
    }

    /** Забывает сервер полностью. */
    suspend fun forgetServer() {
        tokenStore.clear()
        serverConfig.clear()
    }

    private suspend fun expireSession() {
        runCatching { tokenStore.clear() }
        runCatching { serverConfig.clearSession() }
        _sessionExpired.tryEmit(Unit)
    }
}
