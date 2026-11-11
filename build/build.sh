#!/usr/bin/env bash
# Builds the GradeStream Windows installer on Linux/macOS (cross-compilation).
# Requires: Go 1.27+, curl, osslsigncode (to verify cloudflared's signature; a warning is shown without it).
# Run from the repository root:  ./build/build.sh
set -euo pipefail
VERSION=$(sed -n 's/^const appVersion = "\(.*\)"/\1/p' app/main.go)
CLOUDFLARED=2026.10.7
PAYLOAD=installer/payload
mkdir -p "$PAYLOAD" dist

if [ ! -f "$PAYLOAD/cloudflared.exe" ]; then
  echo "Downloading cloudflared $CLOUDFLARED..."
  curl -fsSL -o "$PAYLOAD/cloudflared.exe" \
    "https://github.com/cloudflare/cloudflared/releases/download/$CLOUDFLARED/cloudflared-windows-amd64.exe"
fi
if command -v osslsigncode >/dev/null; then
  osslsigncode verify "$PAYLOAD/cloudflared.exe" | grep -q 'Cloudflare, Inc' || { rm "$PAYLOAD/cloudflared.exe"; echo "cloudflared signature not verified"; exit 1; }
else
  echo "Warning: osslsigncode not found, cloudflared signature not verified"
fi
cp docs/gradestream.ico "$PAYLOAD/gradestream.ico"

export GOOS=windows GOARCH=amd64 CGO_ENABLED=0
(cd app && go build -mod=mod -trimpath -ldflags "-s -w -H windowsgui" -o "../$PAYLOAD/GradeStream.exe" .)
(cd installer && go build -trimpath -ldflags "-s -w -H windowsgui" -o "../dist/GradeStream-Setup-$VERSION.exe" .)
echo "Done: dist/GradeStream-Setup-$VERSION.exe"
