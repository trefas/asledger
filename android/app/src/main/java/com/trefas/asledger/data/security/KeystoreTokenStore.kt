package com.trefas.asledger.data.security

import android.content.Context
import android.security.keystore.KeyGenParameterSpec
import android.security.keystore.KeyProperties
import android.util.Base64
import androidx.datastore.core.DataStore
import androidx.datastore.preferences.core.Preferences
import androidx.datastore.preferences.core.edit
import androidx.datastore.preferences.core.stringPreferencesKey
import androidx.datastore.preferences.preferencesDataStore
import java.security.KeyStore
import javax.crypto.Cipher
import javax.crypto.KeyGenerator
import javax.crypto.SecretKey
import javax.crypto.spec.GCMParameterSpec
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.withContext

/** Отдельное хранилище для шифротекста токена. */
private val Context.tokenDataStore: DataStore<Preferences> by preferencesDataStore(
    name = "token"
)

/**
 * Токен устройства в DataStore, зашифрованный ключом Android Keystore.
 *
 * В файле приложения лежит только AES/GCM-шифротекст с IV. Ключ создаётся в
 * Android Keystore и не экспортируется. Токен никогда не пишется в журнал.
 */
class KeystoreTokenStore(
    private val context: Context
) : TokenStore {
    override suspend fun save(token: String) {
        require(token.isNotBlank()) { "empty token" }
        val encrypted = withContext(Dispatchers.IO) { encrypt(token) }
        context.tokenDataStore.edit { preferences ->
            preferences[KEY_TOKEN] = encrypted
        }
    }

    override suspend fun load(): String? = withContext(Dispatchers.IO) {
        val encoded = context.tokenDataStore.data.first()[KEY_TOKEN] ?: return@withContext null
        runCatching { decrypt(encoded) }.getOrNull()
    }

    override suspend fun clear() {
        context.tokenDataStore.edit { preferences ->
            preferences.remove(KEY_TOKEN)
        }
    }

    private fun encrypt(token: String): String {
        val cipher = Cipher.getInstance(TRANSFORMATION)
        cipher.init(Cipher.ENCRYPT_MODE, getOrCreateKey())
        val iv = cipher.iv
        val ciphertext = cipher.doFinal(token.toByteArray(Charsets.UTF_8))
        val payload = ByteArray(1 + iv.size + ciphertext.size)
        payload[0] = iv.size.toByte()
        iv.copyInto(payload, destinationOffset = 1)
        ciphertext.copyInto(payload, destinationOffset = 1 + iv.size)
        return Base64.encodeToString(payload, Base64.NO_WRAP)
    }

    private fun decrypt(encoded: String): String {
        val payload = Base64.decode(encoded, Base64.NO_WRAP)
        require(payload.isNotEmpty()) { "empty ciphertext" }
        val ivSize = payload[0].toInt() and 0xFF
        require(ivSize in 1..MAX_IV_SIZE && payload.size > ivSize + 1) { "corrupted ciphertext" }
        val iv = payload.copyOfRange(1, 1 + ivSize)
        val ciphertext = payload.copyOfRange(1 + ivSize, payload.size)
        val cipher = Cipher.getInstance(TRANSFORMATION)
        cipher.init(Cipher.DECRYPT_MODE, getOrCreateKey(), GCMParameterSpec(TAG_BITS, iv))
        return String(cipher.doFinal(ciphertext), Charsets.UTF_8)
    }

    @Synchronized
    private fun getOrCreateKey(): SecretKey {
        val keyStore = KeyStore.getInstance(ANDROID_KEYSTORE).apply { load(null) }
        val existing = keyStore.getEntry(KEY_ALIAS, null) as? KeyStore.SecretKeyEntry
        if (existing != null) {
            return existing.secretKey
        }
        val generator = KeyGenerator.getInstance(KeyProperties.KEY_ALGORITHM_AES, ANDROID_KEYSTORE)
        generator.init(
            KeyGenParameterSpec.Builder(
                KEY_ALIAS,
                KeyProperties.PURPOSE_ENCRYPT or KeyProperties.PURPOSE_DECRYPT
            )
                .setBlockModes(KeyProperties.BLOCK_MODE_GCM)
                .setEncryptionPaddings(KeyProperties.ENCRYPTION_PADDING_NONE)
                .setKeySize(KEY_SIZE_BITS)
                .build()
        )
        return generator.generateKey()
    }

    private companion object {
        const val ANDROID_KEYSTORE = "AndroidKeyStore"
        const val KEY_ALIAS = "asledger-device-token"
        const val TRANSFORMATION = "AES/GCM/NoPadding"
        const val TAG_BITS = 128
        const val KEY_SIZE_BITS = 256
        const val MAX_IV_SIZE = 32
        val KEY_TOKEN = stringPreferencesKey("device_token")
    }
}
