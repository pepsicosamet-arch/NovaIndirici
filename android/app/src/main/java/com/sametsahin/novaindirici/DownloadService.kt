package com.sametsahin.novaindirici

import android.app.PendingIntent
import android.app.Service
import android.content.Context
import android.content.Intent
import android.content.pm.ServiceInfo
import android.os.Build
import android.os.IBinder
import android.os.PowerManager
import androidx.core.app.NotificationCompat
import androidx.core.app.NotificationManagerCompat
import androidx.core.app.ServiceCompat
import androidx.core.content.ContextCompat
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job as CJob
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch

/**
 * İndirme sürerken uygulamayı ön planda tutar: ekran kapansa ya da başka
 * uygulamaya geçilse de Android indirmeyi durdurmaz. Bildirimde ilerleme görünür.
 */
class DownloadService : Service() {

    private var loop: CJob? = null
    private var wake: PowerManager.WakeLock? = null

    override fun onBind(intent: Intent?): IBinder? = null

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        val type = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.Q) ServiceInfo.FOREGROUND_SERVICE_TYPE_DATA_SYNC else 0
        ServiceCompat.startForeground(this, NOTIF_ID, build(), type)
        if (wake == null) {
            wake = (getSystemService(Context.POWER_SERVICE) as PowerManager)
                .newWakeLock(PowerManager.PARTIAL_WAKE_LOCK, "nova:download").apply { acquire(3 * 60 * 60 * 1000L) }
        }
        if (loop == null) {
            loop = NovaApp.appScope.launch(Dispatchers.Main) {
                while (true) {
                    delay(1000)
                    if (NovaApp.jobs.none { it.active }) break
                    try { NotificationManagerCompat.from(this@DownloadService).notify(NOTIF_ID, build()) } catch (_: SecurityException) {}
                }
                stopSelf()
            }
        }
        return START_NOT_STICKY
    }

    private fun build(): android.app.Notification {
        val active = NovaApp.jobs.filter { it.active }
        val first = active.firstOrNull()
        val open = PendingIntent.getActivity(
            this, 0, Intent(this, MainActivity::class.java).addFlags(Intent.FLAG_ACTIVITY_SINGLE_TOP),
            PendingIntent.FLAG_IMMUTABLE or PendingIntent.FLAG_UPDATE_CURRENT,
        )
        val b = NotificationCompat.Builder(this, NovaApp.CHANNEL)
            .setSmallIcon(android.R.drawable.stat_sys_download)
            .setContentTitle(if (active.size > 1) "${active.size} indirme sürüyor" else (first?.title ?: "NOVA İndirici"))
            .setContentText(first?.let { j -> j.stage + if (j.progress > 0f && j.status == "running") " · %${(j.progress * 100).toInt()}" else "" } ?: "Hazırlanıyor")
            .setOngoing(true)
            .setOnlyAlertOnce(true)
            .setSilent(true)
            .setContentIntent(open)
        if (first != null && first.status == "running" && first.progress > 0f) b.setProgress(100, (first.progress * 100).toInt(), false)
        else b.setProgress(0, 0, true)
        return b.build()
    }

    override fun onDestroy() {
        loop?.cancel()
        loop = null
        wake?.let { if (it.isHeld) it.release() }
        wake = null
        super.onDestroy()
    }

    companion object {
        private const val NOTIF_ID = 42

        fun start(ctx: Context) {
            try {
                ContextCompat.startForegroundService(ctx, Intent(ctx, DownloadService::class.java))
            } catch (_: Exception) {
                // Arka planda başlatma kısıtlıysa indirme yine sürer; yalnızca bildirim olmaz.
            }
        }
    }
}
