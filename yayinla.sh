#!/usr/bin/env bash
# NOVA İndirici — yeni sürüm hazırlama
# Kullanım: ./yayinla.sh 1.2.0 "Yenilikler: ..."
# Ardından: git add -A && git commit -m "v1.2.0" && git push
set -euo pipefail
VER="${1:?Sürüm numarası gerekli, örn: ./yayinla.sh 1.2.0 \"notlar\"}"
NOTES="${2:-Yenilikler ve düzeltmeler.}"
VER="${VER#v}"
cd "$(dirname "$0")"
LD="-s -w -H windowsgui -X main.AppVersion=$VER"
export GOOS=windows GOARCH=amd64 CGO_ENABLED=0

if [ ! -f rsrc_windows_amd64.syso ]; then
  rsrc -ico nova.ico -manifest app.manifest -arch amd64 -o rsrc_windows_amd64.syso
fi
cp rsrc_windows_amd64.syso setup/

echo "→ Uygulama derleniyor ($VER)"
go build -trimpath -ldflags "$LD" -o setup/payload/NOVA-Indirici.exe .
echo "→ Kurulum derleniyor"
(cd setup && go build -trimpath -ldflags "$LD" -o ../dist/NOVA-Indirici-Setup.exe .)
cp setup/payload/NOVA-Indirici.exe dist/NOVA-Indirici.exe

python3 - "$VER" "$NOTES" <<'PY'
import json, sys
ver, notes = sys.argv[1], sys.argv[2]
base = "https://github.com/pepsicosamet-arch/NovaIndirici/raw/main/dist/"
# Mac ve Android paketleri derleme hattında üretilip sürüm sayfasına yüklenir.
rel = "https://github.com/pepsicosamet-arch/NovaIndirici/releases/download/v" + ver + "/"
json.dump({"version": ver, "notes": notes,
           "url": base + "NOVA-Indirici-Setup.exe",
           "mac": rel + "NOVA-Indirici-Mac.dmg",
           "android": rel + "NOVA-Indirici-Android.apk"},
          open("dist/latest.json", "w", encoding="utf-8"), ensure_ascii=False, indent=2)
PY
echo "✓ Hazır: dist/ (v$VER). Şimdi: git add -A && git commit -m \"v$VER\" && git push"
