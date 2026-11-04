# NOVA İndirici

Samet Şahin tarafından geliştirilen video / ses indirici.

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
