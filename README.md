# NOVA İndirici

Bağlantıyı yapıştır, gerisini NOVA halletsin. YouTube, Instagram, TikTok, X ve 1000'den fazla siteden video ya da ses indir.

<p align="center"><img src="web/logo.png" width="160" alt="NOVA İndirici"></p>

🌐 **Web sitesi:** [novaindirici.com.tr](https://novaindirici.com.tr)

## ⬇️ İndir

**[NOVA-Indirici-Setup.exe](https://github.com/pepsicosamet-arch/NovaIndirici/raw/main/dist/NOVA-Indirici-Setup.exe)** (Windows 10 / 11, 64 bit) indir ve çalıştır. Kurulum birkaç saniye sürer, yönetici izni gerekmez.

Kurulum istemeyenler için taşınabilir sürüm: [NOVA-Indirici.exe](https://github.com/pepsicosamet-arch/NovaIndirici/raw/main/dist/NOVA-Indirici.exe)

> Dosya imzasız olduğu için Windows SmartScreen uyarı verebilir: **Ek bilgi → Yine de çalıştır**.
> İlk açılışta indirme motoru (yt-dlp) ve FFmpeg otomatik indirilir (yalnızca bir kez).

## Özellikler
- 4K'ya kadar video (En iyi / 1440p / 1080p / 720p / 480p / 360p)
- Sadece ses: MP3, M4A, OPUS, FLAC, WAV
- Oynatma listesini tek seferde indirme
- Canlı ilerleme, hız ve kalan süre; iptal ve tekrar dene
- Aynı anda birden fazla indirme, geçmiş ve arama
- Reklamsız, hesap istemez, veri toplamaz

## 👤 Geliştirici

**Samet Şahin** · NOVA'yı tasarlayan ve geliştiren kişi. Yeni özellikleri, geliştirme sürecini ve perde arkasını sosyal medyada paylaşıyorum.

[Instagram](https://www.instagram.com/xsametsahinx/) · [TikTok](https://www.tiktok.com/@xsametsahinx) · [YouTube](https://www.youtube.com/@xsametsahinx) · [X (Twitter)](https://x.com/xsametsahinx)

## ✉️ Şikayet ve öneri

Bir sorun mu yaşadın ya da aklında bir fikir mi var? **[samet.sahin.iletisim@gmail.com](mailto:samet.sahin.iletisim@gmail.com)**

## 🌍 Web sürümü (sunucu)

NOVA aynı kodla bir sunucuda da çalışır: ziyaretçi bağlantıyı yapıştırır, dosya sunucuda hazırlanır ve bitince tarayıcıdan bilgisayarına iner.

- Her ziyaretçi yalnızca kendi indirmelerini görür (çerezle ayrılır).
- Dosyalar 30 dakika sonra sunucudan otomatik silinir.
- Sınırlar: kişi başı aynı anda 2 indirme, saatte 20 indirme, dosya başına 2 GB, listelerde en fazla 10 öğe (`render.yaml` içinden değiştirilebilir).
- İç ağ adreslerine (127.0.0.1, 10.x, 169.254.x …) istek engellenir.
- İndirme motoru (yt-dlp) 12 saatte bir kendini günceller.

**Hetzner (önerilen, 20 TB trafik dahil):** Sunucu oluştururken `deploy/cloud-init.yaml` içeriğini **Cloud config** kutusuna yapıştır. Docker, NOVA ve otomatik HTTPS (Caddy) kendiliğinden kurulur; sunucu 30 dakikada bir GitHub'daki son sürümü çeker. Alan adı: `app.novaindirici.com.tr` için sunucunun IP adresine bir **A** kaydı ekle.

**Render.com ile kurulum:** Render'da **New → Blueprint** seçip bu depoyu bağla; `render.yaml` her şeyi ayarlar. Her `git push` sonrası sunucu otomatik yeniden kurulur.

**Kendi sunucunda (Docker):**

    docker build -t nova-web .
    docker run -d -p 8080:8080 --restart unless-stopped nova-web

---

## Geliştiriciler için
## Yapı
- main.go   — yerel sunucu, ayarlar, API
- jobs.go   — indirme kuyruğu, yt-dlp ilerleme takibi
- tools.go  — yt-dlp ve FFmpeg'in ilk açılışta otomatik indirilmesi
- uninstall_windows.go — kaldırma (--uninstall)
- web/      — arayüz (index.html, logo.svg, avatar.jpg)
- setup/    — kurulum sihirbazı (NOVA-Indirici-Setup.exe)

Veriler: %LOCALAPPDATA%\NovaIndirici (ayarlar, geçmiş, yt-dlp, ffmpeg)

## Derleme (Go 1.24+)
    go install github.com/akavel/rsrc@latest
    rsrc -ico nova.ico -manifest app.manifest -arch amd64 -o rsrc_windows_amd64.syso
    cp rsrc_windows_amd64.syso setup/
    GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "-s -w -H windowsgui" -o setup/payload/NOVA-Indirici.exe .
    cd setup && GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "-s -w -H windowsgui" -o NOVA-Indirici-Setup.exe .

Sessiz kurulum: `NOVA-Indirici-Setup.exe /S` (isteğe bağlı `/D=C:\Klasör`)
Kaldırma: Ayarlar → Uygulamalar → NOVA İndirici → Kaldır (ya da `NOVA-Indirici.exe --uninstall`)
