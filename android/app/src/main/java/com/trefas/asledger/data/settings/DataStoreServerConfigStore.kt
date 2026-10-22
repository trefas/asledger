package com.trefas.asledger.data.settings

import android.content.Context
import androidx.datastore.core.DataStore
import androidx.datastore.preferences.core.Preferences
import androidx.datastore.preferences.core.booleanPreferencesKey
import androidx.datastore.preferences.core.edit
import androidx.datastore.preferences.core.emptyPreferences
import androidx.datastore.preferences.core.intPreferencesKey
import androidx.datastore.preferences.core.longPreferencesKey
import androidx.datastore.preferences.core.stringPreferencesKey
import androidx.datastore.preferences.preferencesDataStore
import java.io.IOException
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.catch
import kotlinx.coroutines.flow.map

/** Единственное хранилище настроек приложения. */
private val Context.serverDataStore: DataStore<Preferences> by preferencesDataStore(
    name = "server"
)

/** Хранит адрес, пользователя и метаданные привязки в DataStore. */
class DataStoreServerConfigStore(
    private val context: Context
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
            ServerConfig(
                host = host,
                port = port,
                clientId = preferences[KEY_CLIENT_ID].orEmpty(),
                userId = preferences[KEY_USER_ID] ?: 0,
                userName = preferences[KEY_USER_NAME].orEmpty(),
                serverVersion = preferences[KEY_SERVER_VERSION].orEmpty(),
                formatVersion = preferences[KEY_FORMAT_VERSION] ?: 0,
                onboarded = preferences[KEY_ONBOARDED] ?: false,
                lastSyncAt = preferences[KEY_LAST_SYNC_AT] ?: 0
            )
        }

    override suspend fun saveServer(
        host: String,
        port: Int,
        serverVersion: String,
        formatVersion: Int
    ) {
        val normalized = ServerConfig.normalizeHost(host)
        require(normalized.isNotEmpty()) { "server host must not be blank" }
        require(port in MIN_PORT..MAX_PORT) { "port out of range" }
        context.serverDataStore.edit { preferences ->
            val clientId = preferences[KEY_CLIENT_ID]
                ?.takeIf { it.isNotBlank() }
                ?: ServerConfig.newClientId()
            preferences[KEY_HOST] = normalized
            preferences[KEY_PORT] = port
            preferences[KEY_CLIENT_ID] = clientId
            preferences[KEY_SERVER_VERSION] = serverVersion
            preferences[KEY_FORMAT_VERSION] = formatVersion
            preferences.remove(KEY_USER_ID)
            preferences.remove(KEY_USER_NAME)
            preferences.remove(KEY_ONBOARDED)
            preferences.remove(KEY_LAST_SYNC_AT)
        }
    }

    override suspend fun saveSession(userId: Long, userName: String, lastSyncAt: Long) {
        require(userId > 0) { "user is not selected" }
        context.serverDataStore.edit { preferences ->
            val clientId = preferences[KEY_CLIENT_ID]
                ?.takeIf { it.isNotBlank() }
                ?: ServerConfig.newClientId()
            preferences[KEY_CLIENT_ID] = clientId
            preferences[KEY_USER_ID] = userId
            preferences[KEY_USER_NAME] = userName
            preferences[KEY_LAST_SYNC_AT] = lastSyncAt
        }
    }

    override suspend fun saveServerInfo(serverVersion: String, formatVersion: Int) {
        context.serverDataStore.edit { preferences ->
            preferences[KEY_SERVER_VERSION] = serverVersion
            preferences[KEY_FORMAT_VERSION] = formatVersion
        }
    }

    override suspend fun saveLastSync(epochMillis: Long) {
        context.serverDataStore.edit { preferences ->
            preferences[KEY_LAST_SYNC_AT] = epochMillis
        }
    }

    override suspend fun setOnboarded() {
        context.serverDataStore.edit { preferences ->
            preferences[KEY_ONBOARDED] = true
        }
    }

    override suspend fun clearSession() {
        context.serverDataStore.edit { preferences ->
            preferences.remove(KEY_USER_ID)
            preferences.remove(KEY_USER_NAME)
            preferences.remove(KEY_ONBOARDED)
            preferences.remove(KEY_LAST_SYNC_AT)
        }
    }

    override suspend fun clear() {
        context.serverDataStore.edit { preferences ->
            preferences.clear()
        }
    }

    private companion object {
        val KEY_HOST = stringPreferencesKey("host")
        val KEY_PORT = intPreferencesKey("port")
        val KEY_CLIENT_ID = stringPreferencesKey("client_id")
        val KEY_USER_ID = longPreferencesKey("user_id")
        val KEY_USER_NAME = stringPreferencesKey("user_name")
        val KEY_SERVER_VERSION = stringPreferencesKey("server_version")
        val KEY_FORMAT_VERSION = intPreferencesKey("format_version")
        val KEY_ONBOARDED = booleanPreferencesKey("onboarded")
        val KEY_LAST_SYNC_AT = longPreferencesKey("last_sync_at")
        const val MIN_PORT = 1
        const val MAX_PORT = 65_535
    }
}
