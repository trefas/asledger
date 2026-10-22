package com.trefas.asledger.data.settings

import android.content.Context

/** Запоминает, какие обучающие подсказки пользователь уже видел. */
interface HintStore {
    /** Подсказка уже показывалась. */
    fun wasSeen(id: String): Boolean

    /** Отмечает подсказку просмотренной. */
    fun markSeen(id: String)
}

/** Хранит отметки в отдельном SharedPreferences: они не мешают настройкам сервера. */
class SharedPreferencesHintStore(context: Context) : HintStore {
    private val preferences = context.applicationContext.getSharedPreferences(
        FILE_NAME,
        Context.MODE_PRIVATE
    )

    override fun wasSeen(id: String): Boolean = preferences.getBoolean(id, false)

    override fun markSeen(id: String) {
        preferences.edit().putBoolean(id, true).apply()
    }

    private companion object {
        const val FILE_NAME = "hints"
    }
}
