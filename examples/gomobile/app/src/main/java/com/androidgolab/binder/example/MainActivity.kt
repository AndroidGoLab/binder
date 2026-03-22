package com.androidgolab.binder.example

import android.app.Activity
import android.os.Bundle
import android.util.Log
import android.widget.LinearLayout
import android.widget.ScrollView
import android.widget.TextView
import client.Client

class MainActivity : Activity() {
    private val TAG = "BinderExample"

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)

        val layout = LinearLayout(this).apply {
            orientation = LinearLayout.VERTICAL
            setPadding(32, 32, 32, 32)
        }
        val scroll = ScrollView(this).apply { addView(layout) }
        setContentView(scroll)

        val output = TextView(this).apply {
            textSize = 14f
            typeface = android.graphics.Typeface.MONOSPACE
        }
        layout.addView(output)

        Thread {
            val sb = StringBuilder()
            try {
                val binderClient = Client.newBinderClient()
                sb.appendLine("=== Binder Client Connected ===\n")

                // Power status
                try {
                    val power = binderClient.getPowerStatus()
                    sb.appendLine("Power Status:")
                    sb.appendLine("  Screen on: ${power.isInteractive}")
                    sb.appendLine("  Power save: ${power.isPowerSaveMode}")
                    sb.appendLine("  Battery saver supported: ${power.isBatterySaverSupported}")
                    sb.appendLine()
                } catch (e: Exception) {
                    sb.appendLine("Power: ${e.message}\n")
                }

                // Display info
                try {
                    val display = binderClient.getDisplayInfo()
                    sb.appendLine("Display:")
                    sb.appendLine("  Brightness: ${display.brightness}")
                    sb.appendLine("  Night mode: ${display.nightMode}")
                    sb.appendLine()
                } catch (e: Exception) {
                    sb.appendLine("Display: ${e.message}\n")
                }

                // Device info (aggregate)
                try {
                    val device = binderClient.getDeviceInfo()
                    sb.appendLine("Device:")
                    sb.appendLine("  Thermal status: ${device.thermalStatus}")
                    sb.appendLine("  Service count: ${device.serviceCount}")
                    sb.appendLine()
                } catch (e: Exception) {
                    sb.appendLine("Device: ${e.message}\n")
                }

                // Service existence check
                try {
                    val exists = binderClient.checkServiceExists("activity")
                    sb.appendLine("Activity service exists: $exists")
                } catch (e: Exception) {
                    sb.appendLine("Service check: ${e.message}")
                }

                binderClient.close()
                sb.appendLine("\n=== Done ===")

            } catch (e: Exception) {
                sb.appendLine("Error: ${e.message}")
                Log.e(TAG, "Binder error", e)
            }

            runOnUiThread { output.text = sb.toString() }
        }.start()
    }
}
