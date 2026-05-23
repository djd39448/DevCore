#!/usr/bin/env bash
#
# desktop/build.sh — compile and assemble DevCore.app.
#
# Depends on: swiftc (Xcode or the Command Line Tools) and the Go toolchain
#   (`go` on PATH; the api binary is a sibling Go module under cmd/).
# Depended on by: developers building the desktop shell locally; not yet in CI.
# Why it exists: DevCore's desktop shell (buildspec Path B) is a small native
#   macOS app with no Xcode project. This script compiles the Swift shell,
#   builds the devcore-api binary the shell launches as a subprocess, and
#   assembles both — plus the prototype web assets — into the .app bundle.
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

echo "building the devcore-api binary..."
# Build from the repo root (one level above desktop/) so go can see the module.
(cd .. && go build -o "desktop/$app/Contents/MacOS/devcore-api" ./cmd/devcore-api)

echo "bundling metadata and web assets..."
cp Shell/Info.plist "$app/Contents/Info.plist"
cp web/DevCore.app.html web/styles.css web/*.jsx "$app/Contents/Resources/web/"

echo "built: desktop/$app"
echo "run:   open desktop/$app"
