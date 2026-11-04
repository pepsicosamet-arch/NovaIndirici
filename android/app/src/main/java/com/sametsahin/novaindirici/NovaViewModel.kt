package com.sametsahin.novaindirici

import android.app.Application
import android.content.ContentValues
import android.media.MediaScannerConnection
import android.net.Uri
import android.os.Build
import android.os.Environment
import android.provider.MediaStore
import android.webkit.MimeTypeMap
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableFloatStateOf
import androidx.compose.runtime.mutableLongStateOf
import androidx.compose.runtime.mutableStateListOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import androidx.lifecycle.AndroidViewModel
import androidx.lifecycle.viewModelScope
import com.yausername.youtubedl_android.YoutubeDL
import com.yausername.youtubedl_android.YoutubeDLRequest
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.sync.withPermit
import kotlinx.coroutines.withContext
import org.json.JSONObject
import java.io.File
import java.net.HttpURLConnection
import java.net.URL

enum class Mode { VIDEO, AUDIO }

data class MediaInfo(
    val url: String,
    val title: String,
    val uploader: String?,
    val thumbnail: String?,
    val duration: Int,
    val site: String?,
    val heights: List<Int>,
)

data class UpdateInfo(val version: String, val notes: String, val url: String)

class Job(
    val id: String,
    val url: String,
    title: String,
    val thumbnail: String?,
    val mode: Mode,
    val quality: String,
    val audioFormat: String,
) {
    var title by mutableStateOf(title)
    var status by mutableStateOf("queued") // queued, running, processing, done, error, canceled
    var stage by mutableStateOf("Sırada")
    var progress by mutableFloatStateOf(0f)
    var eta by mutableLongStateOf(-1L)
    var error by mutableStateOf<String?>(null)
    var files by mutableStateOf<List<String>>(emptyList())
    var uri by mutableStateOf<Uri?>(null)
    val active get() = status == "queued" || status == "running" || status == "processing"
}

class NovaViewModel(app: Application) : AndroidViewModel(app) {

    var url by mutableStateOf("")
    var loading by mutableStateOf(false)
    var info by mutableStateOf<MediaInfo?>(null)
    var infoError by mutableStateOf<String?>(null)
    var mode by mutableStateOf(Mode.VIDEO)
    var quality by mutableStateOf("best")
    var audioFormat by mutableStateOf("mp3")
    var update by mutableStateOf<UpdateInfo?>(null)
    var engineBusy by mutableStateOf(false)
    var message by mutableStateOf<String?>(null)

    val jobs get() = NovaApp.jobs
    private var seq = 0

    val outDir: File
        get() = File(Environment.getExternalStoragePublicDirectory(Environment.DIRECTORY_DOWNLOADS), "NOVA")

    init {
        checkUpdate()
    }

    fun fetchInfo() {
        val u = url.trim()
        if (!u.startsWith("http://") && !u.startsWith("https://")) {
            infoError = "Geçerli bir bağlantı gir (https:// ile başlamalı)."
            return
        }
        loading = true
        infoError = null
        info = null
        viewModelScope.launch {
            try {
                val v = withContext(Dispatchers.IO) {
                    waitReady()
                    val req = YoutubeDLRequest(u)
                    req.addOption("--no-playlist")
                    YoutubeDL.getInstance().getInfo(req)
                }
                val heights = v.formats.orEmpty()
                    .filter { it.height > 0 && it.vcodec != "none" }
                    .map { std(it.height) }.distinct().sortedDescending()
                info = MediaInfo(
                    url = u,
                    title = v.title ?: v.fulltitle ?: u,
                    uploader = v.uploader,
                    thumbnail = v.thumbnail,
                    duration = v.duration,
                    site = v.extractorKey ?: v.extractor,
                    heights = heights.filter { it >= 360 },
                )
                val qh = quality.toIntOrNull()
                if (quality != "best" && (qh == null || qh !in heights)) quality = "best"
            } catch (e: Throwable) {
                infoError = friendly(e.message)
            } finally {
                loading = false
            }
        }
    }

    private fun std(h: Int): Int {
        for (s in intArrayOf(4320, 2160, 1440, 1080, 720, 480, 360, 240, 144)) if (h >= s - 20) return s
        return h
    }

    fun download() {
        val i = info ?: return
        seq++
        val job = Job(
            id = "nova-${System.currentTimeMillis()}-$seq",
            url = i.url, title = i.title, thumbnail = i.thumbnail,
            mode = mode, quality = quality, audioFormat = audioFormat,
        )
        jobs.add(0, job)
        info = null
        url = ""
        message = "İndirme başladı. Dosya İndirilenler/NOVA klasörüne kaydedilecek."
        start(job)
    }

    /** İşi uygulama düzeyinde başlatır: ekran kapansa da sürer. */
    private fun start(job: Job) {
        DownloadService.start(getApplication())
        NovaApp.appScope.launch(Dispatchers.Main) { NovaApp.slots.withPermit { run(job) } }
    }

    private suspend fun run(job: Job) {
        if (job.status == "canceled") return
        job.status = "running"
        job.stage = "Hazırlanıyor"
        val app = getApplication<Application>()
        val work = File(app.getExternalFilesDir(null) ?: app.filesDir, "jobs/${job.id}")
        try {
            val saved = withContext(Dispatchers.IO) {
                waitReady { s -> NovaApp.appScope.launch(Dispatchers.Main) { job.stage = s } }
                work.deleteRecursively()
                val out = File(work, "out").apply { mkdirs() }
                val tmp = File(work, "part").apply { mkdirs() }
                val req = YoutubeDLRequest(job.url)
                req.addOption("--no-playlist")
                req.addOption("--newline")
                req.addOption("--no-mtime")
                req.addOption("--windows-filenames")
                req.addOption("--embed-metadata")
                req.addOption("-P", out.absolutePath)
                req.addOption("-P", "temp:" + tmp.absolutePath)
                req.addOption("-o", "%(title).100B [%(id)s].%(ext)s")
                if (job.mode == Mode.AUDIO) {
                    req.addOption("-f", "ba/b")
                    req.addOption("-x")
                    req.addOption("--audio-format", job.audioFormat)
                    req.addOption("--audio-quality", "0")
                } else {
                    val h = job.quality.toIntOrNull()
                    req.addOption("-f", "bv*+ba/b")
                    req.addOption("-S", if (h != null) "res:$h,ext:mp4:m4a" else "res,ext:mp4:m4a")
                    req.addOption("--merge-output-format", "mp4")
                }
                YoutubeDL.getInstance().execute(req, job.id) { p, eta, line ->
                    val stage = when {
                        line.startsWith("[Merger]") -> "Birleştiriliyor"
                        line.startsWith("[ExtractAudio]") -> "Ses dönüştürülüyor"
                        line.startsWith("[Metadata]") -> "Bilgiler yazılıyor"
                        line.startsWith("[download]") && p > 0f -> "İndiriliyor"
                        line.startsWith("[download] Destination") -> "İndiriliyor"
                        line.startsWith("[info]") || line.startsWith("[youtube]") || line.startsWith("[instagram]") ||
                            line.startsWith("[TikTok]") || line.startsWith("[generic]") -> "Bilgiler alınıyor"
                        else -> null
                    }
                    NovaApp.appScope.launch(Dispatchers.Main) {
                        if (job.status == "canceled") return@launch
                        if (p > 0f) job.progress = (p / 100f).coerceIn(0f, 1f)
                        if (eta > 0) job.eta = eta
                        if (stage != null) {
                            job.stage = stage
                            job.status = if (stage == "İndiriliyor" || stage == "Bilgiler alınıyor") "running" else "processing"
                        }
                    }
                }
                NovaApp.appScope.launch(Dispatchers.Main) { job.stage = "Kaydediliyor"; job.status = "processing" }
                val files = out.walkTopDown().filter { it.isFile && it.length() > 0 && !it.name.endsWith(".part") && !it.name.endsWith(".ytdl") }.toList()
                if (files.isEmpty()) throw IllegalStateException("İndirilen dosya bulunamadı. Başka bir kalite deneyin.")
                files.map { export(it) }
            }
            job.files = saved.map { it.first }
            job.uri = saved.firstOrNull()?.second
            job.progress = 1f
            job.status = "done"
            job.stage = "Tamamlandı"
        } catch (e: YoutubeDL.CanceledException) {
            job.status = "canceled"
            job.stage = "İptal edildi"
        } catch (e: Throwable) {
            if (job.status == "canceled") return
            job.status = "error"
            job.stage = "Hata"
            job.error = friendly(e.message)
        } finally {
            withContext(Dispatchers.IO) { work.deleteRecursively() }
        }
    }

    /** Dosyayı İndirilenler/NOVA klasörüne kaydeder; (ad, uri) döndürür. */
    private fun export(f: File): Pair<String, Uri?> {
        val app = getApplication<Application>()
        val ext = f.extension.lowercase()
        val mime = MimeTypeMap.getSingleton().getMimeTypeFromExtension(ext) ?: when (ext) {
            "mp4", "m4v" -> "video/mp4"; "webm" -> "video/webm"; "mkv" -> "video/x-matroska"
            "mp3" -> "audio/mpeg"; "m4a" -> "audio/mp4"; "opus" -> "audio/ogg"; "flac" -> "audio/flac"
            else -> "application/octet-stream"
        }
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.Q) {
            val cr = app.contentResolver
            val values = ContentValues().apply {
                put(MediaStore.MediaColumns.DISPLAY_NAME, f.name)
                put(MediaStore.MediaColumns.MIME_TYPE, mime)
                put(MediaStore.MediaColumns.RELATIVE_PATH, Environment.DIRECTORY_DOWNLOADS + "/NOVA")
                put(MediaStore.MediaColumns.IS_PENDING, 1)
            }
            val uri = cr.insert(MediaStore.Downloads.EXTERNAL_CONTENT_URI, values)
                ?: throw IllegalStateException("Dosya İndirilenler klasörüne kaydedilemedi.")
            cr.openOutputStream(uri)?.use { o -> f.inputStream().use { it.copyTo(o) } }
            values.clear()
            values.put(MediaStore.MediaColumns.IS_PENDING, 0)
            cr.update(uri, values, null, null)
            return f.name to uri
        }
        outDir.mkdirs()
        var dst = File(outDir, f.name)
        var n = 2
        while (dst.exists()) { dst = File(outDir, "${f.nameWithoutExtension} ($n).${f.extension}"); n++ }
        f.copyTo(dst)
        MediaScannerConnection.scanFile(app, arrayOf(dst.absolutePath), arrayOf(mime), null)
        return dst.name to Uri.fromFile(dst)
    }

    fun cancel(job: Job) {
        if (job.status == "queued") {
            job.status = "canceled"
            job.stage = "İptal edildi"
            return
        }
        job.status = "canceled"
        job.stage = "İptal edildi"
        YoutubeDL.getInstance().destroyProcessById(job.id)
    }

    fun retry(job: Job) {
        val copy = Job(
            id = job.id + "r", url = job.url, title = job.title, thumbnail = job.thumbnail,
            mode = job.mode, quality = job.quality, audioFormat = job.audioFormat,
        )
        jobs.remove(job)
        jobs.add(0, copy)
        start(copy)
    }

    fun clearFinished() {
        jobs.removeAll { !it.active }
    }

    fun updateEngine() {
        if (engineBusy) return
        engineBusy = true
        viewModelScope.launch {
            val res = withContext(Dispatchers.IO) {
                waitReady {}
                NovaApp.updateEngine() ?: "Güncellenemedi. İnternet bağlantını kontrol et."
            }
            message = res
            engineBusy = false
        }
    }

    private fun checkUpdate() {
        viewModelScope.launch {
            val u = withContext(Dispatchers.IO) {
                var found: UpdateInfo? = null
                for (src in UPDATE_SOURCES) {
                    try {
                        val c = URL(src + "?t=" + (System.currentTimeMillis() / 300000)).openConnection() as HttpURLConnection
                        c.connectTimeout = 10000
                        c.readTimeout = 10000
                        c.setRequestProperty("User-Agent", "NOVA-Indirici-Android/" + BuildConfig.VERSION_NAME)
                        c.setRequestProperty("Cache-Control", "no-cache")
                        if (c.responseCode != 200) continue
                        val j = JSONObject(c.inputStream.bufferedReader().use { it.readText() })
                        val v = j.optString("version")
                        if (v.isEmpty()) continue
                        val apk = j.optString("android")
                        found = if (newer(v, BuildConfig.VERSION_NAME) && apk.startsWith(REPO_BASE)) UpdateInfo(v, j.optString("notes"), apk) else null
                        break
                    } catch (e: Throwable) {
                        continue
                    }
                }
                found
            }
            update = u
        }
    }

    /** Motor hazır olana (ve devam eden güncelleme bitene) kadar bekler. */
    private suspend fun waitReady(onStage: (String) -> Unit = {}) {
        var waited = 0
        while (!NovaApp.ready.value || (NovaApp.updating.value && waited < 120_000)) {
            NovaApp.initError.value?.let { throw IllegalStateException("İndirme motoru başlatılamadı: $it") }
            onStage(if (!NovaApp.ready.value) "Motor hazırlanıyor" else "Motor güncelleniyor")
            kotlinx.coroutines.delay(300)
            waited += 300
        }
    }

    companion object {
        const val REPO_BASE = "https://github.com/pepsicosamet-arch/NovaIndirici/"
        const val LATEST_URL = REPO_BASE + "raw/main/dist/latest.json"
        /** Biri çalışmazsa sıradaki denenir. */
        val UPDATE_SOURCES = listOf(
            LATEST_URL,
            REPO_BASE + "releases/latest/download/latest.json",
            "https://novaindirici.com.tr/latest.json",
        )

        fun newer(a: String, b: String): Boolean {
            val pa = a.trim().removePrefix("v").split(".").map { it.toIntOrNull() ?: 0 }
            val pb = b.trim().removePrefix("v").split(".").map { it.toIntOrNull() ?: 0 }
            for (i in 0 until 3) {
                val x = pa.getOrElse(i) { 0 }
                val y = pb.getOrElse(i) { 0 }
                if (x != y) return x > y
            }
            return false
        }

        fun friendly(raw: String?): String {
            val text = raw ?: return "Bilinmeyen bir hata oluştu."
            val last = text.lines().map { it.trim() }.lastOrNull { it.startsWith("ERROR:") }
                ?.removePrefix("ERROR:")?.trim() ?: text.lines().lastOrNull { it.isNotBlank() }?.trim() ?: text
            val low = last.lowercase()
            return when {
                "unsupported url" in low -> "Bu bağlantı desteklenmiyor."
                "private video" in low || "login required" in low || "sign in" in low -> "Bu içerik giriş gerektiriyor ya da gizli."
                "video unavailable" in low || "not available" in low -> "İçerik kullanılamıyor ya da kaldırılmış."
                "unable to download webpage" in low || "timed out" in low || "network" in low -> "Bağlantı kurulamadı. İnternetini kontrol et."
                "requested format is not available" in low -> "Seçilen kalite bu içerikte yok. Başka bir kalite dene."
                else -> if (last.length > 220) last.take(220) + "…" else last
            }
        }
    }
}
