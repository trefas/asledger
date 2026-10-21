package com.trefas.asledger.data.sync

import com.trefas.asledger.data.model.SyncCommit
import java.security.MessageDigest

/**
 * Звено хэш-цепочки: SHA-256(prevHash || opsJson).
 *
 * `opsJson` берётся из ответа сервера без повторной сериализации: экранирование
 * и порядок полей в Kotlin могут отличаться от canonical JSON Go.
 */
fun chainHash(prevHash: String, opsJson: String): String {
    val digest = MessageDigest.getInstance("SHA-256")
    val bytes = digest.digest((prevHash + opsJson).toByteArray(Charsets.UTF_8))
    return bytes.joinToString(separator = "") { byte -> "%02x".format(byte) }
}

/**
 * Проверяет цепочку коммитов от известного хеша.
 *
 * Возвращает новый хеш головы или null, если хотя бы одно звено не совпало.
 */
fun verifyChain(prevHash: String, commits: List<SyncCommit>): String? {
    var current = prevHash
    for (commit in commits) {
        val computed = chainHash(current, commit.opsJson)
        if (computed != commit.hash) {
            return null
        }
        current = commit.hash
    }
    return current
}
