#!/bin/sh
# Builds Paw Proxy.app at DEST.
#
#   APP_VERSION=2026.11.1 BUILD_NUMBER=1 [SIGNING_IDENTITY='Developer ID Application: …'] \
#     sh macos/Scripts/package-app.sh '/path/Paw Proxy.app'
#   DEV_BUILD=1 sh macos/Scripts/package-app.sh '/path/Paw Proxy Dev.app'
#
# Without SIGNING_IDENTITY the bundle is ad-hoc signed: a local candidate
# only, never distributed. Dev builds use the .dev bundle ID and carry no
# LaunchAgents, so they can never register or replace the installed daemon.
# Notarization, stapling and publication belong to `task release`.
set -eu

DEST=${1:?usage: package-app.sh DEST.app}
case "$DEST" in *.app) ;; *) echo "DEST must end in .app" >&2; exit 2 ;; esac

ROOT=$(cd "$(dirname "$0")/../.." && pwd)
MACOS="$ROOT/macos"
DEV_BUILD=${DEV_BUILD:-0}
SIGNING_IDENTITY=${SIGNING_IDENTITY:-}
BASE_ID=com.alexcatdad.paw-proxy
API=github.com/alexcatdad/paw-proxy/internal/api

if [ "$DEV_BUILD" = 1 ]; then
  BUNDLE_ID="$BASE_ID.dev"; NAME="Paw Proxy Dev"
  APP_VERSION=${APP_VERSION:-dev}; BUILD_NUMBER=${BUILD_NUMBER:-1}
else
  BUNDLE_ID="$BASE_ID"; NAME="Paw Proxy"
  APP_VERSION=${APP_VERSION:?APP_VERSION is required for release builds}
  BUILD_NUMBER=${BUILD_NUMBER:-1}
  echo "$APP_VERSION" | grep -Eq '^[0-9]+(\.[0-9]+){1,2}$' || { echo "invalid APP_VERSION: $APP_VERSION" >&2; exit 2; }
fi
echo "$BUILD_NUMBER" | grep -Eq '^[0-9]+$' || { echo "invalid BUILD_NUMBER: $BUILD_NUMBER" >&2; exit 2; }

plist_get() { /usr/libexec/PlistBuddy -c "Print :$2" "$1" 2>/dev/null || true; }

# Never replace a bundle that belongs to something else.
if [ -e "$DEST" ]; then
  existing=$(plist_get "$DEST/Contents/Info.plist" CFBundleIdentifier)
  if [ "$existing" != "$BUNDLE_ID" ]; then
    echo "refusing to replace $DEST (bundle ID '${existing:-none}', expected $BUNDLE_ID)" >&2
    exit 1
  fi
fi

PARENT=$(dirname "$DEST")
mkdir -p "$PARENT"
STAGE=$(mktemp -d "$PARENT/.paw-proxy-stage.XXXXXX")
trap 'rm -rf "$STAGE"' EXIT
APP="$STAGE/app.app"

echo "==> Building menu-bar app (arm64)"
swift build -c release --arch arm64 --package-path "$MACOS" --product PawProxy >/dev/null
BIN=$(swift build -c release --arch arm64 --package-path "$MACOS" --show-bin-path)

echo "==> Building helpers $APP_VERSION (build $BUILD_NUMBER)"
LDFLAGS="-s -w -X main.version=$APP_VERSION -X main.build=$BUILD_NUMBER -X $API.Version=$APP_VERSION -X $API.Build=$BUILD_NUMBER"
mkdir -p "$APP/Contents/MacOS" "$APP/Contents/Helpers" "$APP/Contents/Resources"
for cmd in paw-proxy up; do
  CGO_ENABLED=1 GOOS=darwin GOARCH=arm64 go build -C "$ROOT" -trimpath -ldflags "$LDFLAGS" \
    -o "$APP/Contents/Helpers/$cmd" "./cmd/$cmd"
done

echo "==> Assembling $NAME"
cp "$BIN/PawProxy" "$APP/Contents/MacOS/PawProxy"
cp "$MACOS/Resources/AppIcon.icns" "$APP/Contents/Resources/AppIcon.icns"
cp "$MACOS/Resources/Info.plist" "$APP/Contents/Info.plist"
PB=/usr/libexec/PlistBuddy
$PB -c "Set :CFBundleIdentifier $BUNDLE_ID" \
    -c "Set :CFBundleName $NAME" \
    -c "Set :CFBundleDisplayName $NAME" \
    -c "Set :CFBundleShortVersionString $APP_VERSION" \
    -c "Set :CFBundleVersion $BUILD_NUMBER" "$APP/Contents/Info.plist"
if [ "$DEV_BUILD" != 1 ]; then
  mkdir -p "$APP/Contents/Library/LaunchAgents"
  cp "$MACOS/Resources/com.alexcatdad.paw-proxy.daemon.plist" "$APP/Contents/Library/LaunchAgents/"
fi
find "$APP" -type d -exec chmod 0755 {} +
find "$APP" -type f -exec chmod 0644 {} +
chmod 0755 "$APP/Contents/MacOS/PawProxy" "$APP/Contents/Helpers/paw-proxy" "$APP/Contents/Helpers/up"

echo "==> Validating"
sh "$MACOS/Scripts/validate-bundle.sh" "$APP"

echo "==> Signing"
if [ -n "$SIGNING_IDENTITY" ]; then
  SIGN="--options runtime --timestamp -s"; ID="$SIGNING_IDENTITY"
else
  SIGN="-s"; ID="-"
  echo "    (ad-hoc: local candidate only)"
fi
# Helpers first; the outer app last so its seal covers the finished helpers.
# shellcheck disable=SC2086
codesign --force $SIGN "$ID" "$APP/Contents/Helpers/paw-proxy"
# shellcheck disable=SC2086
codesign --force $SIGN "$ID" "$APP/Contents/Helpers/up"
# shellcheck disable=SC2086
codesign --force $SIGN "$ID" "$APP"
codesign --verify --deep --strict "$APP"

# Swap into place; restore the previous bundle if the move fails.
if [ -e "$DEST" ]; then
  OLD="$STAGE/previous.app"
  mv "$DEST" "$OLD"
  if ! mv "$APP" "$DEST"; then mv "$OLD" "$DEST"; echo "install failed; previous bundle restored" >&2; exit 1; fi
else
  mv "$APP" "$DEST"
fi
echo "==> $DEST"
