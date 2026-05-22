#!/usr/bin/env bash
#
# desktop/build.sh — compile and assemble DevCore.app.
#
# Depends on: swiftc (Xcode or the Command Line Tools).
# Depended on by: developers building the desktop shell locally; not yet in CI.
# Why it exists: DevCore's desktop shell (buildspec Path B) is a small native
#   macOS app with no Xcode project. This script compiles the Swift shell and
#   assembles the .app bundle around it and the prototype web assets.
#
# Usage:  ./build.sh        # produces build/DevCore.app

set -euo pipefail
cd "$(dirname "$0")"

app="build/DevCore.app"
rm -rf build
mkdir -p "$app/Contents/MacOS" "$app/Contents/Resources/web"

echo "compiling the Swift shell..."
swiftc -swift-version 6 Shell/main.swift \
  -framework AppKit -framework WebKit \
  -o "$app/Contents/MacOS/DevCore"

echo "bundling metadata and web assets..."
cp Shell/Info.plist "$app/Contents/Info.plist"
cp web/DevCore.app.html web/styles.css web/*.jsx "$app/Contents/Resources/web/"

echo "built: desktop/$app"
echo "run:   open desktop/$app"
