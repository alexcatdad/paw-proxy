#!/bin/sh
# Builds 'Paw Proxy Dev.app' (bundle ID com.alexcatdad.paw-proxy.dev,
# ad-hoc signed, no LaunchAgents) for iterating on the menu-bar UI against
# the installed daemon. It never registers or replaces services.
#   sh macos/Scripts/dev-app.sh [--run]
set -eu
ROOT=$(cd "$(dirname "$0")/../.." && pwd)
DEST="$ROOT/macos/.build/dev/Paw Proxy Dev.app"
DEV_BUILD=1 sh "$ROOT/macos/Scripts/package-app.sh" "$DEST"
if [ "${1:-}" = --run ]; then
  EXE="$DEST/Contents/MacOS/PawProxy"
  # Stop only this exact dev executable, never the installed app.
  for pid in $(pgrep -x PawProxy || true); do
    if [ "$(ps -o comm= -p "$pid")" = "$EXE" ]; then kill -TERM "$pid"; fi
  done
  sleep 0.5
  open "$DEST"
fi
