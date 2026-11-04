@file:OptIn(androidx.compose.material3.ExperimentalMaterial3Api::class)

package com.sametsahin.novaindirici

import android.Manifest
import android.content.ActivityNotFoundException
import android.content.Intent
import android.content.pm.PackageManager
import android.net.Uri
import android.os.Build
import android.os.Bundle
import android.view.WindowManager
import android.widget.Toast
import androidx.activity.ComponentActivity
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.compose.setContent
import androidx.activity.result.contract.ActivityResultContracts
import androidx.activity.viewModels
import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.Image
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.aspectRatio
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Close
import androidx.compose.material.icons.filled.ContentPaste
import androidx.compose.material.icons.filled.Download
import androidx.compose.material.icons.filled.Email
import androidx.compose.material.icons.filled.Info
import androidx.compose.material.icons.filled.Language
import androidx.compose.material.icons.filled.List
import androidx.compose.material.icons.filled.MusicNote
import androidx.compose.material.icons.filled.PhotoCamera
import androidx.compose.material.icons.filled.PlayArrow
import androidx.compose.material.icons.filled.Refresh
import androidx.compose.material.icons.filled.SmartDisplay
import androidx.compose.material.icons.filled.Videocam
import androidx.compose.material.icons.filled.Chat
import androidx.compose.material3.Button
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.FilterChip
import androidx.compose.material3.FilterChipDefaults
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.NavigationBar
import androidx.compose.material3.NavigationBarItem
import androidx.compose.material3.NavigationBarItemDefaults
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.OutlinedTextFieldDefaults
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.darkColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.platform.LocalClipboardManager
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.core.content.ContextCompat
import coil.compose.AsyncImage

private val Bg = Color(0xFF0C0B0F)
private val Bg2 = Color(0xFF121117)
private val Panel = Color(0xFF17151C)
private val Panel2 = Color(0xFF1F1B25)
private val Line = Color(0xFF2A2631)
private val Fg = Color(0xFFF3EFF2)
private val Muted = Color(0xFFA59CA8)
private val Dim = Color(0xFF6F6773)
private val Red = Color(0xFFFF3B3B)
private val Red2 = Color(0xFFD91F2A)
private val Ember = Color(0xFFFF7A45)
private val Ok = Color(0xFF3ECF8E)
private val Err = Color(0xFFFF5A5F)

private val NovaColors = darkColorScheme(
    primary = Red, onPrimary = Color.White, secondary = Ember,
    background = Bg, onBackground = Fg, surface = Panel, onSurface = Fg,
    surfaceVariant = Panel2, onSurfaceVariant = Muted, outline = Line, error = Err,
)

class MainActivity : ComponentActivity() {
    private val vm: NovaViewModel by viewModels()

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        handleShare(intent)
        setContent {
            MaterialTheme(colorScheme = NovaColors) { NovaRoot(vm) }
        }
    }

    override fun onNewIntent(intent: Intent) {
        super.onNewIntent(intent)
        handleShare(intent)
    }

    /** YouTube vb. uygulamalardan "Paylaş → NOVA" ile gelen bağlantı. */
    private fun handleShare(intent: Intent?) {
        if (intent?.action != Intent.ACTION_SEND) return
        val text = intent.getStringExtra(Intent.EXTRA_TEXT) ?: return
        val link = Regex("https?://\\S+").find(text)?.value ?: return
        vm.url = link
        vm.fetchInfo()
    }

    fun keepScreenOn(on: Boolean) {
        if (on) window.addFlags(WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON)
        else window.clearFlags(WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON)
    }
}

private fun openLink(ctx: android.content.Context, url: String) {
    try {
        ctx.startActivity(Intent(Intent.ACTION_VIEW, Uri.parse(url)).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK))
    } catch (e: ActivityNotFoundException) {
        Toast.makeText(ctx, url, Toast.LENGTH_LONG).show()
    }
}

@Composable
fun NovaRoot(vm: NovaViewModel) {
    var tab by rememberSaveable { mutableIntStateOf(0) }
    val ctx = LocalContext.current
    val ready by NovaApp.ready.collectAsState()
    val initError by NovaApp.initError.collectAsState()
    val activeCount = vm.jobs.count { it.active }

    DisposableEffect(activeCount > 0) {
        (ctx as? MainActivity)?.keepScreenOn(activeCount > 0)
        onDispose { }
    }
    LaunchedEffect(vm.message) {
        vm.message?.let { Toast.makeText(ctx, it, Toast.LENGTH_SHORT).show(); vm.message = null }
    }
    LaunchedEffect(vm.info, vm.loading) { if (vm.loading || vm.info != null) tab = 0 }

    Scaffold(
        containerColor = Bg,
        bottomBar = {
            NavigationBar(containerColor = Bg2) {
                val items = listOf(
                    Triple("İndir", Icons.Filled.Download, 0),
                    Triple("İndirilenler", Icons.Filled.List, 1),
                    Triple("Hakkında", Icons.Filled.Info, 2),
                )
                for ((label, icon, i) in items) {
                    NavigationBarItem(
                        selected = tab == i, onClick = { tab = i },
                        icon = {
                            Box {
                                Icon(icon, contentDescription = label)
                                if (i == 1 && activeCount > 0) {
                                    Box(
                                        Modifier.align(Alignment.TopEnd).size(9.dp).clip(CircleShape).background(Red)
                                    )
                                }
                            }
                        },
                        label = { Text(label) },
                        colors = NavigationBarItemDefaults.colors(
                            selectedIconColor = Color.White, selectedTextColor = Color.White,
                            indicatorColor = Red.copy(alpha = 0.22f),
                            unselectedIconColor = Muted, unselectedTextColor = Muted,
                        ),
                    )
                }
            }
        },
    ) { pad ->
        Box(Modifier.fillMaxSize().padding(pad).background(Brush.verticalGradient(listOf(Color(0xFF1E090D), Bg, Bg)))) {
            when (tab) {
                0 -> DownloadScreen(vm, ready, initError) { tab = 1 }
                1 -> JobsScreen(vm)
                else -> AboutScreen(vm)
            }
        }
    }
}

@Composable
private fun Brand() {
    Row(verticalAlignment = Alignment.CenterVertically) {
        Image(painterResource(R.drawable.nova_logo), null, Modifier.size(44.dp))
        Spacer(Modifier.width(10.dp))
        Column {
            Text(
                buildString { append("NOVA") }, color = Fg, fontWeight = FontWeight.ExtraBold,
                fontSize = 20.sp, letterSpacing = 2.sp,
            )
            Text("İNDİRİCİ", color = Muted, fontSize = 10.sp, letterSpacing = 4.sp, fontWeight = FontWeight.SemiBold)
        }
    }
}

@OptIn(ExperimentalLayoutApi::class)
@Composable
fun DownloadScreen(vm: NovaViewModel, ready: Boolean, initError: String?, goJobs: () -> Unit) {
    val ctx = LocalContext.current
    val clip = LocalClipboardManager.current
    val permLauncher = rememberLauncherForActivityResult(ActivityResultContracts.RequestPermission()) { ok ->
        if (ok) vm.download() else Toast.makeText(ctx, "Dosyaları kaydetmek için depolama izni gerekiyor.", Toast.LENGTH_LONG).show()
    }

    Column(
        Modifier.fillMaxSize().verticalScroll(rememberScrollState()).padding(horizontal = 18.dp, vertical = 16.dp),
        verticalArrangement = Arrangement.spacedBy(14.dp),
    ) {
        Brand()
        vm.update?.let { u ->
            Card(
                colors = CardDefaults.cardColors(containerColor = Red.copy(alpha = 0.14f)),
                border = BorderStroke(1.dp, Color(0xFF4A2228)), shape = RoundedCornerShape(16.dp),
            ) {
                Row(Modifier.padding(14.dp), verticalAlignment = Alignment.CenterVertically) {
                    Column(Modifier.weight(1f)) {
                        Text("Yeni sürüm hazır: v${u.version}", fontWeight = FontWeight.SemiBold, color = Fg)
                        if (u.notes.isNotBlank()) Text(u.notes, color = Muted, fontSize = 13.sp, maxLines = 2, overflow = TextOverflow.Ellipsis)
                    }
                    TextButton(onClick = { openLink(ctx, u.url) }) { Text("İndir", color = Red, fontWeight = FontWeight.Bold) }
                }
            }
        }
        Text(
            "Bağlantıyı yapıştır, gerisini NOVA halletsin.", color = Fg,
            fontSize = 26.sp, fontWeight = FontWeight.Bold, lineHeight = 31.sp,
        )
        Text(
            "YouTube, Instagram, TikTok, X ve 1000'den fazla siteden video ya da ses indir. İpucu: Uygulamalardaki Paylaş menüsünden NOVA'yı seçebilirsin.",
            color = Muted, fontSize = 14.sp,
        )
        OutlinedTextField(
            value = vm.url, onValueChange = { vm.url = it }, singleLine = true,
            placeholder = { Text("https://www.youtube.com/watch?v=…", color = Dim) },
            modifier = Modifier.fillMaxWidth(), shape = RoundedCornerShape(16.dp),
            trailingIcon = {
                IconButton(onClick = {
                    val t = clip.getText()?.text?.trim().orEmpty()
                    if (t.isNotEmpty()) { vm.url = t; vm.fetchInfo() } else Toast.makeText(ctx, "Pano boş.", Toast.LENGTH_SHORT).show()
                }) { Icon(Icons.Filled.ContentPaste, "Yapıştır", tint = Muted) }
            },
            colors = OutlinedTextFieldDefaults.colors(
                focusedBorderColor = Red, unfocusedBorderColor = Line, cursorColor = Red,
                focusedContainerColor = Panel, unfocusedContainerColor = Panel,
            ),
        )
        Button(
            onClick = { vm.fetchInfo() }, enabled = !vm.loading && vm.url.isNotBlank(),
            modifier = Modifier.fillMaxWidth().height(52.dp), shape = RoundedCornerShape(14.dp),
            colors = ButtonDefaults.buttonColors(containerColor = Red2),
        ) { Text(if (vm.loading) "İnceleniyor…" else "Getir", fontWeight = FontWeight.Bold, fontSize = 16.sp) }

        if (!ready) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                if (initError == null) CircularProgressIndicator(Modifier.size(16.dp), color = Red, strokeWidth = 2.dp)
                Spacer(Modifier.width(10.dp))
                Text(initError?.let { "İndirme motoru başlatılamadı: $it" } ?: "İndirme motoru hazırlanıyor… (ilk açılışta biraz sürebilir)", color = Muted, fontSize = 13.sp)
            }
        }
        if (vm.loading) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                CircularProgressIndicator(Modifier.size(18.dp), color = Red, strokeWidth = 2.dp)
                Spacer(Modifier.width(10.dp)); Text("Bağlantı inceleniyor…", color = Muted)
            }
        }
        vm.infoError?.let {
            Card(colors = CardDefaults.cardColors(containerColor = Err.copy(alpha = 0.10f)), border = BorderStroke(1.dp, Err.copy(alpha = 0.35f))) {
                Text(it, Modifier.padding(14.dp), color = Color(0xFFFFB3B5))
            }
        }

        vm.info?.let { info ->
            Card(colors = CardDefaults.cardColors(containerColor = Panel), border = BorderStroke(1.dp, Line), shape = RoundedCornerShape(20.dp)) {
                Column(Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(14.dp)) {
                    Box(Modifier.fillMaxWidth().aspectRatio(16f / 9f).clip(RoundedCornerShape(14.dp)).background(Bg2)) {
                        if (info.thumbnail != null) {
                            AsyncImage(info.thumbnail, null, Modifier.fillMaxSize(), contentScale = ContentScale.Crop)
                        } else {
                            Image(painterResource(R.drawable.nova_logo), null, Modifier.size(64.dp).align(Alignment.Center))
                        }
                        if (info.duration > 0) {
                            Text(
                                fmtTime(info.duration.toLong()), color = Color.White, fontSize = 12.sp, fontWeight = FontWeight.SemiBold,
                                modifier = Modifier.align(Alignment.BottomEnd).padding(8.dp)
                                    .clip(RoundedCornerShape(6.dp)).background(Color(0xC0000000)).padding(horizontal = 6.dp, vertical = 1.dp),
                            )
                        }
                    }
                    info.site?.let { Text(it, color = Color(0xFFFF8C8C), fontSize = 12.sp, fontWeight = FontWeight.Bold) }
                    Text(info.title, color = Fg, fontSize = 17.sp, fontWeight = FontWeight.SemiBold, maxLines = 3, overflow = TextOverflow.Ellipsis)
                    info.uploader?.let { Text(it, color = Muted, fontSize = 13.sp) }

                    Text("Tür", color = Muted, fontSize = 13.sp)
                    Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                        NovaChip(vm.mode == Mode.VIDEO, "Video", Icons.Filled.Videocam) { vm.mode = Mode.VIDEO }
                        NovaChip(vm.mode == Mode.AUDIO, "Sadece ses", Icons.Filled.MusicNote) { vm.mode = Mode.AUDIO }
                    }
                    if (vm.mode == Mode.VIDEO) {
                        Text("Kalite", color = Muted, fontSize = 13.sp)
                        val opts = listOf("best") + info.heights.map { it.toString() }
                        FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp), verticalArrangement = Arrangement.spacedBy(4.dp)) {
                            for (q in opts) NovaChip(vm.quality == q, qLabel(q)) { vm.quality = q }
                        }
                    } else {
                        Text("Biçim", color = Muted, fontSize = 13.sp)
                        FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                            for (f in listOf("mp3", "m4a", "opus", "flac")) NovaChip(vm.audioFormat == f, f.uppercase()) { vm.audioFormat = f }
                        }
                    }
                    Text("Kaydedilecek yer: İndirilenler/NOVA", color = Dim, fontSize = 12.sp)
                    Button(
                        onClick = {
                            val needPerm = Build.VERSION.SDK_INT <= Build.VERSION_CODES.P &&
                                ContextCompat.checkSelfPermission(ctx, Manifest.permission.WRITE_EXTERNAL_STORAGE) != PackageManager.PERMISSION_GRANTED
                            if (needPerm) permLauncher.launch(Manifest.permission.WRITE_EXTERNAL_STORAGE)
                            else { vm.download(); goJobs() }
                        },
                        modifier = Modifier.fillMaxWidth().height(54.dp), shape = RoundedCornerShape(14.dp),
                        colors = ButtonDefaults.buttonColors(containerColor = Red),
                    ) {
                        Icon(Icons.Filled.Download, null); Spacer(Modifier.width(8.dp))
                        Text("İndir", fontWeight = FontWeight.Bold, fontSize = 16.sp)
                    }
                }
            }
        }
        Spacer(Modifier.height(8.dp))
    }
}

private fun qLabel(q: String) = when (q) {
    "best" -> "En iyi"
    "2160" -> "4K"
    "4320" -> "8K"
    else -> "${q}p"
}

@Composable
private fun NovaChip(selected: Boolean, label: String, icon: ImageVector? = null, onClick: () -> Unit) {
    FilterChip(
        selected = selected, onClick = onClick,
        label = { Text(label, fontWeight = FontWeight.SemiBold) },
        leadingIcon = if (icon != null) { { Icon(icon, null, Modifier.size(18.dp)) } } else null,
        shape = RoundedCornerShape(10.dp),
        colors = FilterChipDefaults.filterChipColors(
            containerColor = Bg2, labelColor = Muted, iconColor = Muted,
            selectedContainerColor = Red.copy(alpha = 0.18f), selectedLabelColor = Color.White, selectedLeadingIconColor = Color.White,
        ),
        border = FilterChipDefaults.filterChipBorder(
            enabled = true, selected = selected,
            borderColor = Line, selectedBorderColor = Red,
        ),
    )
}

@Composable
fun JobsScreen(vm: NovaViewModel) {
    val ctx = LocalContext.current
    Column(Modifier.fillMaxSize().padding(horizontal = 18.dp, vertical = 16.dp)) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            Column(Modifier.weight(1f)) {
                Text("İndirilenler", color = Fg, fontSize = 24.sp, fontWeight = FontWeight.Bold)
                Text("Dosyalar İndirilenler/NOVA klasörüne kaydedilir.", color = Muted, fontSize = 13.sp)
            }
            if (vm.jobs.any { !it.active }) TextButton(onClick = { vm.clearFinished() }) { Text("Temizle", color = Muted) }
        }
        Spacer(Modifier.height(14.dp))
        if (vm.jobs.isEmpty()) {
            Box(Modifier.fillMaxWidth().padding(top = 60.dp), contentAlignment = Alignment.Center) {
                Text("Henüz bir şey indirmedin.\n“İndir” sekmesinden bir bağlantı yapıştırarak başla.", color = Dim, fontSize = 14.sp)
            }
        }
        LazyColumn(verticalArrangement = Arrangement.spacedBy(10.dp), contentPadding = PaddingValues(bottom = 20.dp)) {
            items(vm.jobs, key = { it.id }) { j -> JobCard(j, vm, ctx) }
        }
    }
}

@Composable
private fun JobCard(j: Job, vm: NovaViewModel, ctx: android.content.Context) {
    Card(colors = CardDefaults.cardColors(containerColor = Panel), border = BorderStroke(1.dp, Line), shape = RoundedCornerShape(16.dp)) {
        Column(Modifier.padding(12.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Box(Modifier.width(84.dp).aspectRatio(16f / 9f).clip(RoundedCornerShape(10.dp)).background(Bg2), contentAlignment = Alignment.Center) {
                    if (j.thumbnail != null) AsyncImage(j.thumbnail, null, Modifier.fillMaxSize(), contentScale = ContentScale.Crop)
                    else Icon(if (j.mode == Mode.AUDIO) Icons.Filled.MusicNote else Icons.Filled.Videocam, null, tint = Dim)
                }
                Spacer(Modifier.width(12.dp))
                Column(Modifier.weight(1f)) {
                    Text(j.title, color = Fg, fontWeight = FontWeight.SemiBold, maxLines = 2, overflow = TextOverflow.Ellipsis, fontSize = 14.sp)
                    val kind = if (j.mode == Mode.AUDIO) j.audioFormat.uppercase() else "Video · " + qLabel(j.quality)
                    val stColor = when (j.status) { "done" -> Ok; "error" -> Err; "canceled" -> Dim; else -> Color(0xFFFF8C8C) }
                    Row {
                        Text(j.stage, color = stColor, fontSize = 12.sp, fontWeight = FontWeight.SemiBold)
                        Text("  ·  $kind", color = Muted, fontSize = 12.sp)
                        if (j.status == "running" && j.progress > 0f) Text("  ·  %${(j.progress * 100).toInt()}", color = Muted, fontSize = 12.sp)
                        if (j.status == "running" && j.eta > 0) Text("  ·  ${fmtTime(j.eta)} kaldı", color = Muted, fontSize = 12.sp)
                    }
                }
                when {
                    j.active -> IconButton(onClick = { vm.cancel(j) }) { Icon(Icons.Filled.Close, "İptal", tint = Muted) }
                    j.status == "done" -> IconButton(onClick = {
                        val u = j.uri
                        if (u == null) {
                            Toast.makeText(ctx, "Dosya İndirilenler/NOVA klasöründe.", Toast.LENGTH_LONG).show()
                        } else {
                            try {
                                ctx.startActivity(
                                    Intent(Intent.ACTION_VIEW).setDataAndType(u, ctx.contentResolver.getType(u) ?: "*/*")
                                        .addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION or Intent.FLAG_ACTIVITY_NEW_TASK)
                                )
                            } catch (e: ActivityNotFoundException) {
                                Toast.makeText(ctx, "Bu dosyayı açacak uygulama bulunamadı.", Toast.LENGTH_LONG).show()
                            }
                        }
                    }) { Icon(Icons.Filled.PlayArrow, "Aç", tint = Ok) }
                    else -> IconButton(onClick = { vm.retry(j) }) { Icon(Icons.Filled.Refresh, "Tekrar dene", tint = Muted) }
                }
            }
            if (j.active || j.status == "done") {
                Spacer(Modifier.height(10.dp))
                if (j.status == "processing" || (j.status == "running" && j.progress <= 0f) || j.status == "queued") {
                    LinearProgressIndicator(Modifier.fillMaxWidth().height(5.dp).clip(RoundedCornerShape(5.dp)), color = Red, trackColor = Color(0xFF26222C))
                } else {
                    LinearProgressIndicator(
                        progress = { if (j.status == "done") 1f else j.progress },
                        modifier = Modifier.fillMaxWidth().height(5.dp).clip(RoundedCornerShape(5.dp)),
                        color = if (j.status == "done") Ok else Ember, trackColor = Color(0xFF26222C),
                    )
                }
            }
            j.error?.let { Text(it, color = Color(0xFFFF9A9D), fontSize = 12.sp, modifier = Modifier.padding(top = 8.dp)) }
        }
    }
}

@Composable
fun AboutScreen(vm: NovaViewModel) {
    val ctx = LocalContext.current
    val engine by NovaApp.engineVersion.collectAsState()
    Column(
        Modifier.fillMaxSize().verticalScroll(rememberScrollState()).padding(horizontal = 18.dp, vertical = 16.dp),
        verticalArrangement = Arrangement.spacedBy(14.dp),
    ) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            Image(painterResource(R.drawable.nova_logo), null, Modifier.size(64.dp))
            Spacer(Modifier.width(12.dp))
            Column {
                Text("NOVA İndirici", color = Fg, fontSize = 22.sp, fontWeight = FontWeight.Bold)
                Text("Sürüm ${BuildConfig.VERSION_NAME}", color = Muted, fontSize = 13.sp)
            }
        }
        Text("YAPAN", color = Muted, fontSize = 12.sp, letterSpacing = 1.5.sp, fontWeight = FontWeight.SemiBold)
        Card(colors = CardDefaults.cardColors(containerColor = Panel), border = BorderStroke(1.dp, Color(0xFF37313F)), shape = RoundedCornerShape(22.dp)) {
            Column(Modifier.padding(20.dp), verticalArrangement = Arrangement.spacedBy(10.dp)) {
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Image(
                        painterResource(R.drawable.avatar), "Samet Şahin",
                        Modifier.size(84.dp).clip(RoundedCornerShape(20.dp)), contentScale = ContentScale.Crop,
                    )
                    Spacer(Modifier.width(14.dp))
                    Column {
                        Text("GELİŞTİRİCİ", color = Red, fontSize = 11.sp, letterSpacing = 2.sp, fontWeight = FontWeight.Bold)
                        Text("Samet Şahin", color = Fg, fontSize = 24.sp, fontWeight = FontWeight.Bold)
                        Text("NOVA'yı tasarlayan ve geliştiren kişi", color = Muted, fontSize = 13.sp)
                    }
                }
                Text(
                    "NOVA'yı sıfırdan tasarlayıp geliştirdim. Yeni özellikleri, geliştirme sürecini ve perde arkasını sosyal medya hesaplarımda paylaşıyorum.",
                    color = Color(0xFFD7CFD8), fontSize = 14.sp,
                )
                SocialRow(Icons.Filled.Language, "Web sitesi", "novaindirici.com.tr") { openLink(ctx, "https://novaindirici.com.tr") }
                SocialRow(Icons.Filled.PhotoCamera, "Instagram", "@xsametsahinx") { openLink(ctx, "https://www.instagram.com/xsametsahinx/") }
                SocialRow(Icons.Filled.MusicNote, "TikTok", "@xsametsahinx") { openLink(ctx, "https://www.tiktok.com/@xsametsahinx") }
                SocialRow(Icons.Filled.SmartDisplay, "YouTube", "@xsametsahinx") { openLink(ctx, "https://www.youtube.com/@xsametsahinx") }
                SocialRow(Icons.Filled.Chat, "X (Twitter)", "@xsametsahinx") { openLink(ctx, "https://x.com/xsametsahinx") }
            }
        }
        Card(colors = CardDefaults.cardColors(containerColor = Panel), border = BorderStroke(1.dp, Line), shape = RoundedCornerShape(18.dp)) {
            Column(Modifier.padding(18.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
                Text("Şikayet ve öneri", color = Fg, fontWeight = FontWeight.SemiBold, fontSize = 16.sp)
                Text("Bir sorun mu yaşadın ya da aklında bir fikir mi var? Bana yaz:", color = Muted, fontSize = 13.sp)
                Text("samet.sahin.iletisim@gmail.com", color = Fg, fontSize = 14.sp)
                Button(
                    onClick = {
                        val i = Intent(Intent.ACTION_SENDTO, Uri.parse("mailto:samet.sahin.iletisim@gmail.com"))
                            .putExtra(Intent.EXTRA_SUBJECT, "NOVA İndirici (Android ${BuildConfig.VERSION_NAME}) · Şikayet / Öneri")
                        try { ctx.startActivity(i) } catch (e: ActivityNotFoundException) {
                            Toast.makeText(ctx, "E-posta uygulaması bulunamadı.", Toast.LENGTH_LONG).show()
                        }
                    },
                    colors = ButtonDefaults.buttonColors(containerColor = Red), shape = RoundedCornerShape(12.dp),
                ) { Icon(Icons.Filled.Email, null); Spacer(Modifier.width(8.dp)); Text("E-posta gönder") }
            }
        }
        Card(colors = CardDefaults.cardColors(containerColor = Panel), border = BorderStroke(1.dp, Line), shape = RoundedCornerShape(18.dp)) {
            Row(Modifier.padding(18.dp), verticalAlignment = Alignment.CenterVertically) {
                Column(Modifier.weight(1f)) {
                    Text("İndirme motoru", color = Fg, fontWeight = FontWeight.SemiBold)
                    Text("yt-dlp ${engine ?: "—"}", color = Muted, fontSize = 13.sp)
                    Text("Bir site indirmeyi reddederse önce motoru güncelle.", color = Dim, fontSize = 12.sp)
                }
                OutlinedButton(onClick = { vm.updateEngine() }, enabled = !vm.engineBusy, border = BorderStroke(1.dp, Color(0xFF37313F))) {
                    Text(if (vm.engineBusy) "Güncelleniyor…" else "Güncelle", color = Fg)
                }
            }
        }
        Text(
            "NOVA hesap istemez, veri toplamaz. Yalnızca indirme hakkına sahip olduğun içerikleri indir. İndirme için yt-dlp, dönüştürme için FFmpeg kullanılır.",
            color = Dim, fontSize = 12.sp,
        )
        Spacer(Modifier.height(8.dp))
    }
}

@Composable
private fun SocialRow(icon: ImageVector, title: String, sub: String, onClick: () -> Unit) {
    OutlinedButton(
        onClick = onClick, modifier = Modifier.fillMaxWidth(), shape = RoundedCornerShape(12.dp),
        border = BorderStroke(1.dp, Color(0xFF37313F)), contentPadding = PaddingValues(horizontal = 12.dp, vertical = 10.dp),
    ) {
        Box(Modifier.size(32.dp).clip(RoundedCornerShape(9.dp)).background(Red.copy(alpha = 0.14f)), contentAlignment = Alignment.Center) {
            Icon(icon, null, tint = Color(0xFFFF7A7A), modifier = Modifier.size(18.dp))
        }
        Spacer(Modifier.width(12.dp))
        Column(Modifier.weight(1f)) {
            Text(title, color = Fg, fontWeight = FontWeight.SemiBold, fontSize = 14.sp)
            Text(sub, color = Muted, fontSize = 12.sp)
        }
    }
}

private fun fmtTime(s: Long): String {
    if (s <= 0) return ""
    val h = s / 3600
    val m = (s % 3600) / 60
    val x = s % 60
    return if (h > 0) "%d:%02d:%02d".format(h, m, x) else "%d:%02d".format(m, x)
}
