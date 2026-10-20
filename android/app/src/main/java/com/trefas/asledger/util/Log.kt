package com.trefas.asledger.util

import com.trefas.asledger.BuildConfig

/**
 * Единая точка логирования.
 *
 * В release-сборке сообщения отбрасываются. Токены, секреты и полные суммы
 * проводок в журнал не попадают никогда — только идентификаторы и коды ошибок.
 */
object Log {
    private const val PREFIX = "asledger"

    fun d(tag: String, message: String) {
        if (BuildConfig.DEBUG) {
            android.util.Log.d("$PREFIX/$tag", message)
        }
    }

    fun i(tag: String, message: String) {
        if (BuildConfig.DEBUG) {
            android.util.Log.i("$PREFIX/$tag", message)
        }
    }

    fun w(tag: String, message: String, error: Throwable? = null) {
        if (BuildConfig.DEBUG) {
            android.util.Log.w("$PREFIX/$tag", message, error)
        }
    }

    fun e(tag: String, message: String, error: Throwable? = null) {
        if (BuildConfig.DEBUG) {
            android.util.Log.e("$PREFIX/$tag", message, error)
        }
    }
}
