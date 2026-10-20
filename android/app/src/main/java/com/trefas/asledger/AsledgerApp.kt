package com.trefas.asledger

import android.app.Application
import com.trefas.asledger.data.AppContainer

/**
 * Приложение asledger.
 *
 * Контейнер создаётся один раз на процесс и передаётся в UI явно: без
 * DI-фреймворка, как зафиксировано в docs/android.md.
 */
class AsledgerApp : Application() {
    lateinit var container: AppContainer
        private set

    override fun onCreate() {
        super.onCreate()
        container = AppContainer(this)
    }
}
