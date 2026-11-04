package com.sametsahin.novaindirici

import android.app.Application
import android.app.NotificationChannel
import android.app.NotificationManager
import android.os.Build
import androidx.compose.runtime.mutableStateListOf
import com.yausername.ffmpeg.FFmpeg
import com.yausername.youtubedl_android.YoutubeDL
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.launch
import kotlinx.coroutines.sync.Semaphore

/**
 * Uygulama açılırken yt-dlp, Python ve FFmpeg'i hazırlar ve indirme motorunu
 * günde bir kez otomatik günceller (YouTube sık değiştiği için gerekli).
 * İndirme listesi burada tutulur; böylece ekran kapansa da indirmeler sürer.
 */
class NovaApp : Application() {

    override fun onCreate() {
        super.onCreate()
        instance = this
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
            val ch = NotificationChannel(CHANNEL, "İndirmeler", NotificationManager.IMPORTANCE_LOW)
            ch.description = "Devam eden indirmelerin ilerlemesi"
            getSystemService(NotificationManager::class.java).createNotificationChannel(ch)
        }
        appScope.launch {
            try {
                status.value = "İndirme motoru hazırlanıyor… (ilk açılışta 20–30 sn sürebilir)"
                YoutubeDL.getInstance().init(this@NovaApp)
                FFmpeg.getInstance().init(this@NovaApp)
                engineVersion.value = YoutubeDL.getInstance().versionName(this@NovaApp)
                ready.value = true
                status.value = null
            } catch (e: Throwable) {
                initError.value = e.message ?: e.toString()
                status.value = null
                return@launch
            }
            autoUpdate()
        }
    }

    private fun autoUpdate() {
        val prefs = getSharedPreferences("nova", MODE_PRIVATE)
        val last = prefs.getLong("engineUpdated", 0L)
        if (System.currentTimeMillis() - last < 20 * 3600 * 1000L) return
        updateEngine()?.let { prefs.edit().putLong("engineUpdated", System.currentTimeMillis()).apply() }
    }

    companion object {
        const val CHANNEL = "downloads"
        lateinit var instance: NovaApp
            private set

        val appScope = CoroutineScope(SupervisorJob() + Dispatchers.IO)
        val ready = MutableStateFlow(false)
        val updating = MutableStateFlow(false)
        val initError = MutableStateFlow<String?>(null)
        val status = MutableStateFlow<String?>(null)
        val engineVersion = MutableStateFlow<String?>(null)

        /** Tüm indirmeler (ekran kapansa da yaşar). */
        val jobs = mutableStateListOf<Job>()
        val slots = Semaphore(2)

        /** İndirme motorunu günceller; sonuç metnini döndürür (hata olursa null). */
        fun updateEngine(): String? {
            if (updating.value) return null
            updating.value = true
            status.value = "İndirme motoru güncelleniyor…"
            return try {
                val st = YoutubeDL.getInstance().updateYoutubeDL(instance, YoutubeDL.UpdateChannel.STABLE)
                engineVersion.value = YoutubeDL.getInstance().versionName(instance)
                if (st == YoutubeDL.UpdateStatus.ALREADY_UP_TO_DATE) "İndirme motoru zaten güncel." else "İndirme motoru güncellendi."
            } catch (e: Throwable) {
                null
            } finally {
                updating.value = false
                status.value = null
            }
        }
    }
}
