#!/usr/bin/env bash
# Сборка установщика GradeStream для Windows из Linux/macOS (кросс-компиляция).
# Нужно: Go 1.27+, curl, osslsigncode (для проверки подписи cloudflared; без него — предупреждение).
# Запуск из корня репозитория:  ./build/build.sh
set -euo pipefail
VERSION=$(sed -n 's/^const appVersion = "\(.*\)"/\1/p' app/main.go)
CLOUDFLARED=2026.10.7
PAYLOAD=installer/payload
mkdir -p "$PAYLOAD" dist

if [ ! -f "$PAYLOAD/cloudflared.exe" ]; then
  echo "Скачиваю cloudflared $CLOUDFLARED..."
  curl -fsSL -o "$PAYLOAD/cloudflared.exe" \
    "https://github.com/cloudflare/cloudflared/releases/download/$CLOUDFLARED/cloudflared-windows-amd64.exe"
fi
if command -v osslsigncode >/dev/null; then
  osslsigncode verify "$PAYLOAD/cloudflared.exe" | grep -q 'Cloudflare, Inc' || { rm "$PAYLOAD/cloudflared.exe"; echo "Подпись cloudflared не подтверждена"; exit 1; }
else
  echo "Внимание: osslsigncode не найден, подпись cloudflared не проверена"
fi
cp docs/gradestream.ico "$PAYLOAD/gradestream.ico"

export GOOS=windows GOARCH=amd64 CGO_ENABLED=0
(cd app && go build -mod=mod -trimpath -ldflags "-s -w -H windowsgui" -o "../$PAYLOAD/GradeStream.exe" .)
(cd installer && go build -trimpath -ldflags "-s -w -H windowsgui" -o "../dist/GradeStream-Setup-$VERSION.exe" .)
echo "Готово: dist/GradeStream-Setup-$VERSION.exe"
