package com.trefas.asledger

import android.app.Application
import com.trefas.asledger.data.AppContainer
import com.trefas.asledger.data.sync.SyncScheduler

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
        // В Robolectric WorkManager может быть не инициализирован; в реальном
        // приложении планировщик доступен сразу после старта процесса.
        runCatching { SyncScheduler.ensurePeriodic(this) }
    }
}
