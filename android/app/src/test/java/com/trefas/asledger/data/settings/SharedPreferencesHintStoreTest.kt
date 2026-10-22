package com.trefas.asledger.data.settings

import android.content.Context
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.RuntimeEnvironment
import org.robolectric.annotation.Config

/** Отметки о подсказках сохраняются между запусками. */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [34])
class SharedPreferencesHintStoreTest {
    @Test
    fun `просмотренная подсказка запоминается`() {
        val context = RuntimeEnvironment.getApplication() as Context
        context.getSharedPreferences("hints", Context.MODE_PRIVATE)
            .edit()
            .clear()
            .commit()
        val store = SharedPreferencesHintStore(context)

        assertFalse(store.wasSeen("entry-form"))
        store.markSeen("entry-form")
        assertTrue(store.wasSeen("entry-form"))
    }
}
