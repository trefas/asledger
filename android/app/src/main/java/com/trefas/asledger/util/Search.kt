package com.trefas.asledger.util

import java.util.Locale

/**
 * Нормализует строку для поиска.
 *
 * SQLite `NOCASE` не умеет кириллицу, поэтому регистр приводится в Kotlin до
 * запроса, а в индексе хранится уже нормализованный ключ.
 */
fun normalizeSearch(vararg parts: String): String =
    parts.joinToString(" ")
        .lowercase(Locale.ROOT)
        .trim()
