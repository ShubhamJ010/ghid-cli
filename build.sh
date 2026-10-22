#!/bin/bash
set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

SDK_DIR="/opt/homebrew/share/android-commandlinetools"
BUILD_TOOLS="$SDK_DIR/build-tools/35.0.0"
PLATFORM="$SDK_DIR/platforms/android-34/android.jar"
JAVA_HOME="/opt/homebrew/opt/openjdk@17/libexec/openjdk.jdk/Contents/Home"
export JAVA_HOME
export PATH="$JAVA_HOME/bin:$PATH"

echo "=== Building GhostHID ==="
echo "Cleaning build artifacts..."
rm -rf build gen bin *.apk
mkdir -p build gen bin

echo "1. Compiling resources..."
"$BUILD_TOOLS/aapt2" compile --dir res -o build/resources.zip

echo "2. Linking resources..."
"$BUILD_TOOLS/aapt2" link -I "$PLATFORM" \
    --manifest AndroidManifest.xml \
    -o build/unaligned.apk \
    --java gen \
    --auto-add-overlay \
    build/resources.zip

echo "3. Compiling Java sources with JDK 17..."
javac -source 17 -target 17 \
    -classpath "$PLATFORM" \
    -d bin \
    $(find gen src -name "*.java")

echo "4. Dexing with D8..."
"$BUILD_TOOLS/d8" --lib "$PLATFORM" --output bin $(find bin -name "*.class")

echo "5. Packaging classes.dex into APK..."
cd bin
zip -u ../build/unaligned.apk classes.dex
cd ..

echo "6. Aligning APK..."
"$BUILD_TOOLS/zipalign" -f -p 4 build/unaligned.apk build/ghost-hid.apk

if [ ! -f debug.keystore ]; then
    echo "7. Generating debug keystore..."
    keytool -genkeypair -v -keystore debug.keystore -storepass android -alias androiddebugkey -keypass android -keyalg RSA -keysize 2048 -validity 10000 -dname "CN=GhostHID,O=GhostHID,C=US"
fi

echo "8. Signing APK..."
"$BUILD_TOOLS/apksigner" sign --ks debug.keystore --ks-pass pass:android --ks-key-alias androiddebugkey build/ghost-hid.apk

echo "=== SUCCESS: build/ghost-hid.apk ready! ==="
