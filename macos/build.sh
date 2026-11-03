#!/bin/bash
set -euo pipefail
cd "$(dirname "$0")/.."
if [[ "$(uname -s)" != Darwin ]]; then
  echo "请在 macOS 上使用 Xcode Command Line Tools 构建。" >&2
  exit 1
fi
output="${REPOWATCH_OUTPUT_DIR:-$PWD/dist}"
mkdir -p "$output"
app="$output/Repo Watch.app"
# Build in a fresh temporary directory so an interrupted build never leaves a
# partially replaced application at the destination.
temp="$(mktemp -d "${TMPDIR:-/tmp}/repowatch-build.XXXXXX")"
trap 'rm -rf "$temp"' EXIT
staged="$temp/Repo Watch.app"
mkdir -p "$staged/Contents/MacOS" "$staged/Contents/Resources"
cp macos/Info.plist "$staged/Contents/Info.plist"
mkdir -p "$staged/Contents/Resources/fonts" "$staged/Contents/Resources/icons/lucide" "$staged/Contents/Resources/icons/simple-icons"
cp assets/fonts/inter/Inter-Regular.ttf assets/fonts/inter/Inter-Bold.ttf "$staged/Contents/Resources/fonts/"
cp assets/icons/lucide/*.png "$staged/Contents/Resources/icons/lucide/"
cp assets/icons/simple-icons/*.png "$staged/Contents/Resources/icons/simple-icons/"
export MACOSX_DEPLOYMENT_TARGET=12.0
case "$(go env GOARCH)" in
  arm64) swift_arch=arm64 ;;
  amd64) swift_arch=x86_64 ;;
  *) echo "不支持的 Go 架构：$(go env GOARCH)" >&2; exit 1 ;;
esac
CGO_ENABLED=1 go build -o "$staged/Contents/MacOS/repowatch-helper" .
xcrun swiftc -O -parse-as-library -target "$swift_arch-apple-macosx12.0" macos/RepoWatch.swift macos/WatchViews.swift -o "$staged/Contents/MacOS/RepoWatch" -framework AppKit -framework SwiftUI -framework UserNotifications -framework CoreText
iconset="$temp/AppIcon.iconset"
mkdir -p "$iconset"
for size in 16 32 128 256 512; do
  sips -z "$size" "$size" assets/logo-kit/windows/app-icon-512.png --out "$iconset/icon_${size}x${size}.png" >/dev/null
  double=$((size * 2))
  sips -z "$double" "$double" assets/logo-kit/windows/app-icon-512.png --out "$iconset/icon_${size}x${size}@2x.png" >/dev/null
done
iconutil -c icns "$iconset" -o "$staged/Contents/Resources/AppIcon.icns"
identity="${REPOWATCH_SIGNING_IDENTITY:--}"
if [[ "$identity" == "-" ]]; then
  codesign --force --sign - --timestamp=none "$staged/Contents/MacOS/repowatch-helper"
  codesign --force --sign - --timestamp=none "$staged"
else
  codesign --force --sign "$identity" --options runtime --timestamp "$staged/Contents/MacOS/repowatch-helper"
  codesign --force --sign "$identity" --options runtime --timestamp "$staged"
fi
codesign --verify --deep --strict "$staged"
if [[ -e "$app" ]]; then
  echo "目标已存在：$app。请先移动或删除旧构建。" >&2
  exit 1
fi
mv "$staged" "$app"
archive="$output/RepoWatch-macOS-$swift_arch.zip"
ditto -c -k --sequesterRsrc --keepParent "$app" "$archive"
echo "应用：$app"
echo "架构：$swift_arch"
echo "压缩包：$archive"
