#!/bin/sh
# Script-level tests for package-app.sh, validate-bundle.sh and dev builds.
# Unsigned (ad-hoc) and isolated in a temp directory; launches nothing.
set -eu
ROOT=$(cd "$(dirname "$0")/../.." && pwd)
S="$ROOT/macos/Scripts"
T=$(mktemp -d /tmp/pp-pkg.XXXXXX)
trap 'rm -rf "$T"' EXIT
pass() { echo "  ✓ $*"; }
fail() { echo "  ✗ $*" >&2; exit 1; }
get() { /usr/libexec/PlistBuddy -c "Print :$2" "$1"; }

echo "[1] dev build"
DEV_BUILD=1 sh "$S/package-app.sh" "$T/Dev.app" >/dev/null
[ "$(get "$T/Dev.app/Contents/Info.plist" CFBundleIdentifier)" = com.alexcatdad.paw-proxy.dev ] || fail "dev bundle ID"
[ ! -e "$T/Dev.app/Contents/Library/LaunchAgents" ] || fail "dev bundle has LaunchAgents"
[ "$("$T/Dev.app/Contents/Helpers/paw-proxy" version)" = "paw-proxy version dev (build 1)" ] || fail "dev helper version"
pass "dev bundle has .dev ID and no LaunchAgents"

echo "[2] release candidate"
APP_VERSION=2026.11.1 BUILD_NUMBER=7 sh "$S/package-app.sh" "$T/Paw Proxy.app" >/dev/null
R="$T/Paw Proxy.app/Contents"
[ -f "$R/Library/LaunchAgents/com.alexcatdad.paw-proxy.daemon.plist" ] || fail "release agent plist missing"
[ "$("$R/Helpers/up" -version)" = "up version 2026.11.1 (build 7)" ] || fail "release up version"
codesign --verify --deep --strict "$T/Paw Proxy.app" || fail "signature"
[ "$(stat -f %Lp "$R/Resources/AppIcon.icns")" = 644 ] && [ "$(stat -f %Lp "$R/Helpers/paw-proxy")" = 755 ] || fail "modes"
pass "release bundle assembled, versioned, signed (ad-hoc)"

echo "[3] rebuild in place with release-tools' Apple encoding of v2026.11.05.2"
APP_VERSION=2026.1105.2 BUILD_NUMBER=2026.1105.2 sh "$S/package-app.sh" "$T/Paw Proxy.app" >/dev/null
[ "$(get "$R/Info.plist" CFBundleShortVersionString)" = 2026.1105.2 ] || fail "in-place replace"
[ "$(get "$R/Info.plist" CFBundleVersion)" = 2026.1105.2 ] || fail "dotted CFBundleVersion"
[ "$("$R/Helpers/paw-proxy" version)" = "paw-proxy version 2026.1105.2 (build 2026.1105.2)" ] || fail "encoded helper version"
[ -z "$(ls -A "$T" | grep '^\.paw-proxy-stage' || true)" ] || fail "staging dir left behind"
pass "same-ID bundle replaced atomically, no staging left"

echo "[4] refuses foreign bundles and bad input"
mkdir -p "$T/Other.app/Contents"
cp "$ROOT/macos/Resources/Info.plist" "$T/Other.app/Contents/Info.plist"
/usr/libexec/PlistBuddy -c "Set :CFBundleIdentifier com.example.other" "$T/Other.app/Contents/Info.plist"
if APP_VERSION=1.0.0 sh "$S/package-app.sh" "$T/Other.app" >/dev/null 2>&1; then fail "replaced a foreign bundle"; fi
[ "$(get "$T/Other.app/Contents/Info.plist" CFBundleIdentifier)" = com.example.other ] || fail "foreign bundle modified"
if APP_VERSION=1.x sh "$S/package-app.sh" "$T/Bad.app" >/dev/null 2>&1; then fail "accepted invalid version"; fi
# Apple versions take at most three integers; the four-part CalVer stays in the tag.
if APP_VERSION=2026.11.05.2 sh "$S/package-app.sh" "$T/Bad.app" >/dev/null 2>&1; then fail "accepted four-part version"; fi
if APP_VERSION=2026.1105.2 BUILD_NUMBER=2026.11.05.2 sh "$S/package-app.sh" "$T/Bad.app" >/dev/null 2>&1; then fail "accepted four-part build"; fi
if DEV_BUILD=1 sh "$S/package-app.sh" "$T/Paw Proxy.app" >/dev/null 2>&1; then fail "dev build replaced the release bundle"; fi
pass "foreign bundle, invalid version and dev-over-release refused"

echo "[5] validator catches tampering"
cp -R "$T/Paw Proxy.app" "$T/V1.app"
/usr/libexec/PlistBuddy -c "Set :CFBundleVersion 99" "$T/V1.app/Contents/Info.plist"
if sh "$S/validate-bundle.sh" "$T/V1.app" >/dev/null 2>&1; then fail "version mismatch accepted"; fi
cp -R "$T/Paw Proxy.app" "$T/V2.app"
rm "$T/V2.app/Contents/Library/LaunchAgents/com.alexcatdad.paw-proxy.daemon.plist"
if sh "$S/validate-bundle.sh" "$T/V2.app" >/dev/null 2>&1; then fail "missing agent accepted"; fi
cp -R "$T/Paw Proxy.app" "$T/V3.app"
/usr/libexec/PlistBuddy -c "Set :Label dev.paw-proxy" "$T/V3.app/Contents/Library/LaunchAgents/com.alexcatdad.paw-proxy.daemon.plist"
if sh "$S/validate-bundle.sh" "$T/V3.app" >/dev/null 2>&1; then fail "legacy label accepted"; fi
cp -R "$T/Dev.app" "$T/V4.app"
mkdir -p "$T/V4.app/Contents/Library/LaunchAgents"
if sh "$S/validate-bundle.sh" "$T/V4.app" >/dev/null 2>&1; then fail "dev bundle with LaunchAgents accepted"; fi
pass "version skew, missing agent, legacy label, dev agents rejected"

echo "=== package tests passed ==="
