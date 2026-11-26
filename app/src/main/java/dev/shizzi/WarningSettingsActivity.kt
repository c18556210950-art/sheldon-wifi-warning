package dev.shizzi

import android.content.Intent
import android.os.Bundle
import android.widget.Button
import android.widget.LinearLayout
import android.widget.ScrollView
import android.widget.Switch
import android.widget.TextView
import androidx.activity.ComponentActivity
import androidx.lifecycle.lifecycleScope
import kotlinx.coroutines.launch
import org.json.JSONObject

class WarningSettingsActivity : ComponentActivity() {
    private val controller = TetherClient()
    private lateinit var status: TextView
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        title = "谢尔顿热点警告"
        val layout = LinearLayout(this).apply {
            orientation = LinearLayout.VERTICAL
            val p = (24 * resources.displayMetrics.density).toInt()
            setPadding(p,p,p,p)
        }
        fun text(value: String) = TextView(this).apply { text=value; textSize=17f; setPadding(0,16,0,16); layout.addView(this) }
        text("谢尔顿 Wi-Fi 警告").textSize = 27f
        text("连接热点后，系统可能弹出登录页或显示登录提示。点击播放录音，播放结束自动放行；也可点“继续上网”。")
        text("共享模式：一人继续后整组放行。新设备接入时再次启用警告，可能影响已连接设备的新网络请求。此版本不区分设备白名单。")
        val prefs = getSharedPreferences("sheldon",0)
        layout.addView(Switch(this).apply {
            text = "启用热点警告"
            isChecked = prefs.getBoolean("enabled",true)
            setOnCheckedChangeListener { _, checked ->
                prefs.edit().putBoolean("enabled",checked).apply()
                if (SessionService.isSessionUp) lifecycleScope.launch {
                    runCatching { controller.setWarningEnabled(checked) }
                        .onFailure { status.text="设置失败：${it.message}" }
                    refresh()
                }
            }
        })
        layout.addView(Button(this).apply {
            text="立即再次警告（共享模式）"
            setOnClickListener {
                if (!SessionService.isSessionUp) {status.text="请先返回主页并启动热点共享。";return@setOnClickListener}
                lifecycleScope.launch {
                    runCatching { controller.rearmWarning() }.onFailure { status.text="操作失败：${it.message}" }
                    refresh()
                }
            }
        })
        layout.addView(Button(this).apply {
            text="本机预览页面与录音"
            setOnClickListener { startActivity(Intent(this@WarningSettingsActivity,WarningPreviewActivity::class.java)) }
        })
        status=text("请先在主页完成 Shizuku 授权并启动共享。")
        text("共享运行时，访客也可手动打开 http://198.18.0.1/ 。自动弹页取决于访客系统的联网检测。")
        layout.addView(Button(this).apply {text="返回";setOnClickListener {finish()}})
        setContentView(ScrollView(this).apply { addView(layout) })
    }
    override fun onResume() {super.onResume();lifecycleScope.launch {refresh()}}
    private suspend fun refresh() {
        if (!SessionService.isSessionUp) {status.text="共享未运行。请先在主页启动共享。";return}
        runCatching { JSONObject(controller.warningStatus()) }.onSuccess {
            status.text = if (!it.optBoolean("enabled")) "共享运行中，警告已关闭。"
                else if (it.optBoolean("pending")) "等待访客打开警告页，本轮尚未放行。"
                else "本轮已放行；有新设备接入时再次警告。"
        }.onFailure {status.text="无法读取警告状态：${it.message}"}
    }
    override fun onDestroy() {controller.unbind();super.onDestroy()}
}
