package com.trefas.asledger.util

import android.os.Build

/** Человекочитаемое имя устройства для журнала сервера. */
fun deviceName(): String {
    val manufacturer = Build.MANUFACTURER?.trim().orEmpty()
    val model = Build.MODEL?.trim().orEmpty()
    return listOf(manufacturer, model)
        .filter { it.isNotEmpty() }
        .joinToString(" ")
        .ifBlank { "Android" }
}
