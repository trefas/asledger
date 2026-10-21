package com.trefas.asledger.data

import android.content.Context
import com.trefas.asledger.data.db.AsledgerDatabase
import com.trefas.asledger.data.network.AsledgerApi
import com.trefas.asledger.data.network.createHttpClient
import com.trefas.asledger.data.repository.AccountRepository
import com.trefas.asledger.data.repository.BalanceRepository
import com.trefas.asledger.data.repository.TransactionRepository
import com.trefas.asledger.data.security.KeystoreTokenStore
import com.trefas.asledger.data.security.TokenStore
import com.trefas.asledger.data.settings.DataStoreServerConfigStore
import com.trefas.asledger.data.settings.ServerConfigStore
import com.trefas.asledger.data.sync.LibraryLoader
import com.trefas.asledger.data.sync.SnapshotImporter
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
        TransactionRepository(database.transactionDao())
    }

    val balances: BalanceRepository by lazy {
        BalanceRepository(database.accountDao(), database.transactionDao())
    }

    /** Импорт полного снапшота одной транзакцией. */
    val snapshotImporter: SnapshotImporter by lazy {
        SnapshotImporter(database)
    }

    /** Первичная загрузка книги после привязки. */
    val libraryLoader: LibraryLoader by lazy {
        LibraryLoader(api, tokenStore, snapshotImporter, serverConfig)
    }

    /** true, когда первичный снапшот уже импортирован. */
    val libraryLoaded: Flow<Boolean> = database.syncMetaDao()
        .observe()
        .map { it?.initialLoaded == true }

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
