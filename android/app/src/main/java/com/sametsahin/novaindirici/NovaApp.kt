package com.sametsahin.novaindirici

import android.app.Application
import com.yausername.ffmpeg.FFmpeg
import com.yausername.youtubedl_android.YoutubeDL
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.launch

/** Uygulama açılırken yt-dlp, Python ve FFmpeg'i hazırlar. */
class NovaApp : Application() {

    override fun onCreate() {
        super.onCreate()
        appScope.launch {
            try {
                YoutubeDL.getInstance().init(this@NovaApp)
                FFmpeg.getInstance().init(this@NovaApp)
                engineVersion.value = YoutubeDL.getInstance().versionName(this@NovaApp)
                ready.value = true
            } catch (e: Throwable) {
                initError.value = e.message ?: e.toString()
            }
        }
    }

    companion object {
        val appScope = CoroutineScope(SupervisorJob() + Dispatchers.IO)
        val ready = MutableStateFlow(false)
        val initError = MutableStateFlow<String?>(null)
        val engineVersion = MutableStateFlow<String?>(null)
    }
}
