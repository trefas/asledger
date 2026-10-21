package com.trefas.asledger

import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import com.trefas.asledger.ui.navigation.AsledgerNavHost
import com.trefas.asledger.ui.theme.AsledgerTheme

/** Единственная Activity: весь интерфейс на Compose. */
class MainActivity : ComponentActivity() {
    private lateinit var container: com.trefas.asledger.data.AppContainer

    override fun onResume() {
        super.onResume()
        if (::container.isInitialized) {
            container.requestSync()
        }
    }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        enableEdgeToEdge()
        container = (application as AsledgerApp).container
        setContent {
            AsledgerTheme {
                AsledgerNavHost(container = container)
            }
        }
    }
}
