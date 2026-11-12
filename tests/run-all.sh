#!/usr/bin/env bash
# Runs all GradeStream measurements and writes results.md.
# Needs: a Linux/macOS build of app/ at $BIN, Node.js, `npm install` + `npx playwright install chromium` in tests/.
set -uo pipefail
cd "$(dirname "$0")"
BIN=${BIN:-../app/gs-test}
OUT=results.md
start() { # $1 = simulated network jitter, ms
  pkill -x "$(basename "$BIN")" 2>/dev/null || true
  for _ in $(seq 50); do pgrep -x "$(basename "$BIN")" >/dev/null || break; sleep 0.2; done
  rm -f "${TMPDIR:-/tmp}/gradestream/settings.json"
  LANG=en_US.UTF-8 GS_NO_TUNNEL=1 GS_LAN_ANY=1 GS_TEST_JITTER_MS="$1" "$BIN" > gs.log 2>&1 &
  for _ in $(seq 50); do grep -q "room " gs.log 2>/dev/null && break; sleep 0.2; done
  sleep 0.5
}
{
  echo "# GradeStream measurements"
  echo
  echo "Run: $(date -u '+%Y-%m-%d %H:%M UTC') · $(uname -sm) · Chromium (Playwright) · software H.264 encoder · one machine"
  echo
  echo "## Color (41 patches, ΔE2000)"
  echo '```'
  start 0; GS_LOG=gs.log BR=8000000 node colortest.js 2>&1
  echo '```'
  echo
  echo "## Client scopes accuracy (75% bars + gray ramp through both paths)"
  echo '```'
  start 0; GS_LOG=gs.log node scopetest.js 2>&1
  echo '```'
  echo
  echo "## Client layout, scope switching, portrait screens, out-of-range alarm, full screen"
  echo '```'
  start 0; GS_LOG=gs.log SRC=land node layouttest.js 2>&1
  start 0; GS_LOG=gs.log SRC=port node layouttest.js 2>&1
  start 0; GS_LOG=gs.log CASE=bad node alarmtest.js 2>&1
  start 0; GS_LOG=gs.log CASE=good node alarmtest.js 2>&1
  start 0; GS_LOG=gs.log IPHONE=1 node fullscreentest.js 2>&1
  start 0; GS_LOG=gs.log IPHONE=0 node fullscreentest.js 2>&1
  echo '```'
  echo
  echo "## Audio/video sync and frame pacing"
  echo
  echo "Sign: negative = sound ahead of the picture. Pacing at 25 fps: deviation of display intervals from 40 ms."
  echo '```'
  start 0;   GS_LOG=gs.log J=0   PLAY=sync SOUND=1 SECS=20 node avtest.js 2>&1
  start 120; GS_LOG=gs.log J=120 PLAY=sync SOUND=1 SECS=20 node avtest.js 2>&1
  start 120; GS_LOG=gs.log J=120 PLAY=live SOUND=0 SECS=12 node avtest.js 2>&1
  start 120; GS_LOG=gs.log J=120 PLAY=sync SOUND=0 SECS=12 node avtest.js 2>&1
  start 0;   GS_LOG=gs.log PLAY=sync SOUND=1 VPATH=lan SECS=6 node avtest.js 2>&1
  start 0;   GS_LOG=gs.log PLAY=live SOUND=1 VPATH=lan SECS=6 node avtest.js 2>&1
  echo '```'
} | tee "$OUT"
pkill -x "$(basename "$BIN")" 2>/dev/null || true
