package com.trefas.asledger.util

import kotlinx.serialization.builtins.ListSerializer
import kotlinx.serialization.builtins.serializer
import kotlinx.serialization.json.Json

private val json = Json { ignoreUnknownKeys = true }

/** Читает JSON-массив строк; повреждённое значение не роняет экран. */
fun decodeStringList(raw: String?): List<String> {
    if (raw.isNullOrBlank()) return emptyList()
    return runCatching {
        json.decodeFromString(ListSerializer(String.serializer()), raw)
    }.getOrDefault(emptyList())
}

/** Сериализует список строк для хранения в Room. */
fun encodeStringList(values: List<String>): String =
    json.encodeToString(ListSerializer(String.serializer()), values)
