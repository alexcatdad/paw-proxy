#!/bin/sh
# Structural checks for a Paw Proxy.app bundle (signed or not).
#   sh macos/Scripts/validate-bundle.sh '/path/Paw Proxy.app'
set -eu
APP=${1:?usage: validate-bundle.sh APP}
C="$APP/Contents"
fail() { echo "validate-bundle: $*" >&2; exit 1; }
get() { /usr/libexec/PlistBuddy -c "Print :$2" "$1" 2>/dev/null || true; }

plutil -lint -s "$C/Info.plist" || fail "Info.plist does not lint"
ID=$(get "$C/Info.plist" CFBundleIdentifier)
VERSION=$(get "$C/Info.plist" CFBundleShortVersionString)
BUILD=$(get "$C/Info.plist" CFBundleVersion)
[ "$(get "$C/Info.plist" CFBundleExecutable)" = PawProxy ] || fail "CFBundleExecutable must be PawProxy"
[ "$(get "$C/Info.plist" LSUIElement)" = true ] || fail "LSUIElement must be true"

for exe in MacOS/PawProxy Helpers/paw-proxy Helpers/up; do
  [ -f "$C/$exe" ] && [ -x "$C/$exe" ] || fail "missing executable $exe"
done
[ -f "$C/Resources/AppIcon.icns" ] || fail "missing AppIcon.icns"

want_pp="paw-proxy version $VERSION (build $BUILD)"
got_pp=$("$C/Helpers/paw-proxy" version)
[ "$got_pp" = "$want_pp" ] || fail "paw-proxy reports '$got_pp', want '$want_pp'"
want_up="up version $VERSION (build $BUILD)"
got_up=$("$C/Helpers/up" -version)
[ "$got_up" = "$want_up" ] || fail "up reports '$got_up', want '$want_up'"

AGENTS="$C/Library/LaunchAgents"
case "$ID" in
  com.alexcatdad.paw-proxy.dev)
    [ ! -e "$AGENTS" ] || fail "dev bundle must not contain Library/LaunchAgents"
    ;;
  com.alexcatdad.paw-proxy)
    P="$AGENTS/com.alexcatdad.paw-proxy.daemon.plist"
    [ -f "$P" ] || fail "missing daemon agent plist"
    plutil -lint -s "$P" || fail "agent plist does not lint"
    [ "$(get "$P" Label)" = com.alexcatdad.paw-proxy.daemon ] || fail "agent Label must be com.alexcatdad.paw-proxy.daemon"
    BP=$(get "$P" BundleProgram)
    [ "$BP" = Contents/Helpers/paw-proxy ] && [ -x "$APP/$BP" ] || fail "agent BundleProgram must be the bundled paw-proxy"
    [ -n "$(get "$P" Sockets:https:SockServiceName)" ] || fail "agent must declare launchd sockets"
    [ "$(ls "$AGENTS" | wc -l | tr -d ' ')" = 1 ] || fail "unexpected extra LaunchAgents"
    ! grep -q "dev.paw-proxy" "$P" "$C/Info.plist" || fail "legacy label dev.paw-proxy must not appear in bundle plists"
    ;;
  *) fail "unexpected bundle ID '$ID'" ;;
esac
echo "validate-bundle: ok ($ID $VERSION build $BUILD)"
