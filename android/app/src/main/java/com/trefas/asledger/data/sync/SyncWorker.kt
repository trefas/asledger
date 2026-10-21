package com.trefas.asledger.data.sync

import android.content.Context
import androidx.work.BackoffPolicy
import androidx.work.Constraints
import androidx.work.CoroutineWorker
import androidx.work.ExistingPeriodicWorkPolicy
import androidx.work.ExistingWorkPolicy
import androidx.work.NetworkType
import androidx.work.OneTimeWorkRequestBuilder
import androidx.work.PeriodicWorkRequestBuilder
import androidx.work.WorkManager
import androidx.work.WorkerParameters
import com.trefas.asledger.AsledgerApp
import java.util.concurrent.TimeUnit
import kotlinx.coroutines.flow.first

/**
 * Фоновая синхронизация.
 *
 * WorkManager не запускает задачу без сети и соблюдает минимальный период
 * 15 минут, поэтому приложение не опрашивает сервер впустую.
 */
class SyncWorker(
    appContext: Context,
    params: WorkerParameters
) : CoroutineWorker(appContext, params) {
    override suspend fun doWork(): Result {
        val container = (applicationContext as AsledgerApp).container
        val config = container.serverConfig.config.first() ?: return Result.success()
        if (!config.hasSession) return Result.success()
        return when (val outcome = container.syncManager.sync(config)) {
            is SyncOutcome.Success, SyncOutcome.NothingToDo -> Result.success()
            is SyncOutcome.Rejected -> Result.success()
            is SyncOutcome.Failure -> if (SyncRetry.shouldRetry(outcome.error, runAttemptCount)) {
                Result.retry()
            } else {
                Result.success()
            }
        }
    }
}

/** Планирует фоновую и немедленную синхронизацию. */
object SyncScheduler {
    private const val PERIODIC_NAME = "asledger-periodic-sync"
    private const val IMMEDIATE_NAME = "asledger-immediate-sync"

    private val connected = Constraints.Builder()
        .setRequiredNetworkType(NetworkType.CONNECTED)
        .build()

    /** Периодическая задача; повторный вызов не создаёт вторую. */
    fun ensurePeriodic(context: Context) {
        val request = PeriodicWorkRequestBuilder<SyncWorker>(15, TimeUnit.MINUTES)
            .setConstraints(connected)
            .setBackoffCriteria(
                BackoffPolicy.EXPONENTIAL,
                SyncRetry.INITIAL_BACKOFF_SECONDS,
                TimeUnit.SECONDS
            )
            .build()
        WorkManager.getInstance(context).enqueueUniquePeriodicWork(
            PERIODIC_NAME,
            ExistingPeriodicWorkPolicy.KEEP,
            request
        )
    }

    /** Немедленная попытка после сохранения проводки или возврата в приложение. */
    fun requestNow(context: Context) {
        val request = OneTimeWorkRequestBuilder<SyncWorker>()
            .setConstraints(connected)
            .setBackoffCriteria(
                BackoffPolicy.EXPONENTIAL,
                SyncRetry.INITIAL_BACKOFF_SECONDS,
                TimeUnit.SECONDS
            )
            .build()
        WorkManager.getInstance(context).enqueueUniqueWork(
            IMMEDIATE_NAME,
            ExistingWorkPolicy.KEEP,
            request
        )
    }
}
