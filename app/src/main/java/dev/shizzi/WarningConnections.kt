package dev.shizzi

import android.content.Context
import java.lang.reflect.Proxy
import java.util.concurrent.Executors
import java.util.concurrent.Executor
import java.util.concurrent.atomic.AtomicBoolean

/** Re-arm on real Wi-Fi client association events; no IP identity is assumed. */
class WarningConnections(private val context: Context, private val rearm: () -> Unit) {
    private val running = AtomicBoolean(false)
    private val executor = Executors.newSingleThreadExecutor()
    private var manager: Any? = null
    private var callback: Any? = null
    private var poller: Thread? = null
    private val clientsByAp = mutableMapOf<String, Set<String>>()

    fun start() {
        running.set(true)
        runCatching { register() }.onSuccess {
            SessionLog.info("warning: Wi-Fi association callback registered")
        }.onFailure {
            SessionLog.warn("warning: association callback unavailable; polling fallback: ${it.message}")
            poller = Thread({ poll() }, "warning-connections").apply { isDaemon = true; start() }
        }
    }
    private fun register() {
        val wifi = context.getSystemService("wifi") ?: error("wifi unavailable")
        val type = Class.forName("android.net.wifi.WifiManager\$SoftApCallback")
        check(type.isInterface) { "SoftApCallback is not an interface" }
        val proxy = Proxy.newProxyInstance(type.classLoader, arrayOf(type)) { self, method, args ->
            when (method.name) {
                "hashCode" -> return@newProxyInstance System.identityHashCode(self)
                "equals" -> return@newProxyInstance self === args?.firstOrNull()
                "toString" -> return@newProxyInstance "SheldonWarningConnections"
                "onConnectedClientsChanged" -> if (running.get()) {
                    val list = args?.filterIsInstance<List<*>>()?.lastOrNull().orEmpty()
                    val info = args?.firstOrNull { it?.javaClass?.simpleName == "SoftApInfo" }
                    val ap = if (info == null) "all" else runCatching {
                        info.javaClass.getMethod("getApInstanceIdentifier").invoke(info).toString()
                    }.getOrDefault("ap")
                    val ids = list.filterNotNull().map { client ->
                        runCatching { client.javaClass.getMethod("getMacAddress").invoke(client).toString() }
                            .getOrElse { client.toString() }
                    }.toSet()
                    val old = clientsByAp.values.flatten().toSet()
                    if (ap == "all") clientsByAp.clear()
                    else clientsByAp.remove("all")
                    clientsByAp[ap] = ids
                    val current = clientsByAp.values.flatten().toSet()
                    if ((current - old).isNotEmpty()) trigger()
                }
            }
            null
        }
        wifi.javaClass.getMethod("registerSoftApCallback", Executor::class.java, type)
            .invoke(wifi, executor, proxy)
        manager = wifi; callback = proxy
    }
    private fun trigger() {
        if (!running.get()) return
        SessionLog.info("warning: new association; shared warning re-armed")
        rearm()
    }
    private fun poll() {
        val inspector = UpstreamInspector()
        var count: Int? = null
        var marker: String? = null
        while (running.get()) {
            val observed = runCatching { inspector.observeWifi() }.getOrNull()
            if (observed != null && !observed.didTimeout) {
                val active = observed.rawOutput.substringBefore("Stopped SoftApManagers:")
                val next = Regex("getConnectedClientList\\(\\)\\.size\\(\\):\\s*(\\d+)")
                    .findAll(active).mapNotNull { it.groupValues[1].toIntOrNull() }.maxOrNull()
                val event = active.lineSequence().lastOrNull { "what=CMD_ASSOCIATED_STATIONS_CHANGED" in it }
                if (count != null && next != null &&
                    (next > count!! || (next == count && next > 0 && event != null && event != marker))) trigger()
                if (next != null) count = next
                marker = event
            }
            try { Thread.sleep(2000) } catch (_: InterruptedException) { return }
        }
    }
    fun stop() {
        running.set(false)
        val registered = callback
        if (registered != null) runCatching {
            val type = Class.forName("android.net.wifi.WifiManager\$SoftApCallback")
            manager?.javaClass?.getMethod("unregisterSoftApCallback", type)?.invoke(manager, registered)
        }
        callback = null; manager = null
        poller?.interrupt(); poller = null
        executor.shutdownNow()
    }
}
