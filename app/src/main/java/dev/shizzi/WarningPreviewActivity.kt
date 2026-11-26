package dev.shizzi

import android.app.Activity
import android.os.Bundle
import android.webkit.WebView
import android.webkit.WebViewClient
import android.webkit.WebResourceRequest
import android.webkit.WebResourceResponse
import java.io.ByteArrayInputStream

/** Offline preview only. This activity does not claim to install a captive portal. */
class WarningPreviewActivity : Activity() {
    private lateinit var web: WebView
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        web = WebView(this)
        web.settings.javaScriptEnabled = true
        web.settings.allowFileAccess = false
        web.settings.allowContentAccess = false
        web.settings.mediaPlaybackRequiresUserGesture = true
        web.webViewClient = object : WebViewClient() {
            override fun shouldOverrideUrlLoading(view: WebView?, request: WebResourceRequest?): Boolean =
                request?.url?.host != HOST
            override fun shouldInterceptRequest(view: WebView?, request: WebResourceRequest?): WebResourceResponse {
                val uri = request?.url
                val name = uri?.path?.removePrefix("/").orEmpty()
                if (uri?.scheme != "https" || uri.host != HOST || name !in FILES) {
                    return WebResourceResponse("text/plain", "utf-8", 404, "Not Found", emptyMap(), ByteArrayInputStream(byteArrayOf()))
                }
                return WebResourceResponse(FILES[name], null, assets.open("sheldon/$name"))
            }
        }
        setContentView(web)
        web.loadUrl("https://$HOST/index.html")
    }
    override fun onDestroy() {
        web.destroy()
        super.onDestroy()
    }
    companion object {
        private const val HOST = "sheldon.invalid"
        private val FILES = mapOf("index.html" to "text/html", "config.json" to "application/json", "warning.m4a" to "audio/mp4")
    }
}
