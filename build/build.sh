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
  # Newer osslsigncode exits non-zero when it can't build the full chain without a CA file;
  # give it the system CAs and judge by its report: signer is Cloudflare and no check failed.
  CA=/etc/ssl/certs/ca-certificates.crt
  sig=$(osslsigncode verify ${CA:+-CAfile "$CA"} -in "$PAYLOAD/cloudflared.exe" 2>&1 || true)
  echo "$sig" | grep -E 'Subject: CN=Cloudflare|verification|Succeeded|Failed' || true
  # signer is Cloudflare (newer osslsigncode escapes the comma: "Cloudflare\, Inc."), the file's own signature is ok, nothing failed
  if ! echo "$sig" | grep -qE 'CN=Cloudflare\\?, Inc' || ! echo "$sig" | grep -qE '^Signature verification: ok' || echo "$sig" | grep -qiE 'verification: failed|^Failed'; then
    rm "$PAYLOAD/cloudflared.exe"; echo "cloudflared signature not verified"; exit 1
  fi
else
  echo "Warning: osslsigncode not found, cloudflared signature not verified"
fi
cp docs/gradestream.ico "$PAYLOAD/gradestream.ico"

export GOOS=windows GOARCH=amd64 CGO_ENABLED=0
(cd app && go build -mod=mod -trimpath -ldflags "-s -w -H windowsgui" -o "../$PAYLOAD/GradeStream.exe" .)
(cd installer && go build -trimpath -ldflags "-s -w -H windowsgui" -o "../dist/GradeStream-Setup-$VERSION.exe" .)
echo "Done: dist/GradeStream-Setup-$VERSION.exe"
