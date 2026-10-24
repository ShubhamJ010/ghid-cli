#!/bin/bash
set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

if [ ! -f "android/gradlew" ]; then
    echo "ERROR: android/gradlew not found. The GhostHID Android app lives in android/ (package com.sj010.ghid)." >&2
    exit 1
fi

echo "=== Building GhostHID release APK (Gradle + Jetpack Compose, Material 3 Expressive) ==="
cd android
./gradlew :app:assembleRelease
cd ..

mkdir -p build
cp android/app/build/outputs/apk/release/app-release.apk build/ghost-hid.apk
echo "=== SUCCESS: build/ghost-hid.apk (release, package com.sj010.ghid) ready! ==="
