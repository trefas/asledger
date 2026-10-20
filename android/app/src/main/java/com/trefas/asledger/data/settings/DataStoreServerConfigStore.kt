package com.trefas.asledger.data.settings

import android.content.Context
import androidx.datastore.core.DataStore
import androidx.datastore.preferences.core.Preferences
import androidx.datastore.preferences.core.edit
import androidx.datastore.preferences.core.emptyPreferences
import androidx.datastore.preferences.core.intPreferencesKey
import androidx.datastore.preferences.core.stringPreferencesKey
import androidx.datastore.preferences.preferencesDataStore
import java.io.IOException
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.catch
import kotlinx.coroutines.flow.map

/** Единственное хранилище настроек приложения. */
private val Context.serverDataStore: DataStore<Preferences> by preferencesDataStore(
    name = "server",
)

/** Хранит адрес сервера в DataStore. */
class DataStoreServerConfigStore(
    private val context: Context,
) : ServerConfigStore {

    override val config: Flow<ServerConfig?> = context.serverDataStore.data
        .catch { error ->
            if (error is IOException) {
                emit(emptyPreferences())
            } else {
                throw error
            }
        }
        .map { preferences ->
            val host = preferences[KEY_HOST]?.trim().orEmpty()
            if (host.isEmpty()) {
                return@map null
            }
            val port = preferences[KEY_PORT]
                ?.takeIf { it in MIN_PORT..MAX_PORT }
                ?: ServerConfig.DEFAULT_PORT
            ServerConfig(host = host, port = port)
        }

    override suspend fun save(config: ServerConfig) {
        val host = ServerConfig.normalizeHost(config.host)
        require(host.isNotEmpty()) { "адрес сервера не может быть пустым" }
        require(config.port in MIN_PORT..MAX_PORT) { "порт вне диапазона" }
        context.serverDataStore.edit { preferences ->
            preferences[KEY_HOST] = host
            preferences[KEY_PORT] = config.port
        }
    }

    override suspend fun clear() {
        context.serverDataStore.edit { preferences ->
            preferences.remove(KEY_HOST)
            preferences.remove(KEY_PORT)
        }
    }

    private companion object {
        val KEY_HOST = stringPreferencesKey("host")
        val KEY_PORT = intPreferencesKey("port")
        const val MIN_PORT = 1
        const val MAX_PORT = 65_535
    }
}
