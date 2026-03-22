#!/usr/bin/env bash
# Build script for the binder sandbox security test APK.
#
# Requirements:
#   - Android SDK at ~/Android/Sdk
#   - Go toolchain (for cross-compiling the native binary)
#   - JDK with javac, keytool
#
# Output: build/binder_sectest.apk
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
BUILD_DIR="$PROJECT_ROOT/build/apk_build"
APK_SRC="$SCRIPT_DIR/apk"
OUTPUT_APK="$PROJECT_ROOT/build/binder_sectest.apk"

# Android SDK paths.
SDK_HOME="${ANDROID_HOME:-$HOME/Android/Sdk}"
BUILD_TOOLS="$SDK_HOME/build-tools/35.0.0"
PLATFORM="$SDK_HOME/platforms/android-35"
AAPT2="$BUILD_TOOLS/aapt2"
D8="$BUILD_TOOLS/d8"
ZIPALIGN="$BUILD_TOOLS/zipalign"
APKSIGNER="$BUILD_TOOLS/apksigner"
ANDROID_JAR="$PLATFORM/android.jar"

# Detect emulator architecture.
GOARCH="${TARGET_ARCH:-arm64}"
echo "=== Step 1: Cross-compile Go binary for $GOARCH ==="
GOOS=linux GOARCH="$GOARCH" CGO_ENABLED=0 go build \
    -ldflags="-s -w" \
    -o "$PROJECT_ROOT/build/libsecurity_test.so" \
    "$PROJECT_ROOT/cmd/security_test_apk/"
echo "Binary: $PROJECT_ROOT/build/libsecurity_test.so ($(du -h "$PROJECT_ROOT/build/libsecurity_test.so" | cut -f1))"

echo ""
echo "=== Step 2: Prepare build directory ==="
rm -rf "$BUILD_DIR"
mkdir -p "$BUILD_DIR"/{gen,obj,compiled_res}

# Place the binary as a native library so it's in an exec-mountable directory.
case "$GOARCH" in
    arm64) ABI_DIR="arm64-v8a" ;;
    amd64) ABI_DIR="x86_64" ;;
    arm)   ABI_DIR="armeabi-v7a" ;;
    386)   ABI_DIR="x86" ;;
    *)     echo "Unknown GOARCH: $GOARCH"; exit 1 ;;
esac
mkdir -p "$BUILD_DIR/lib/$ABI_DIR"
cp "$PROJECT_ROOT/build/libsecurity_test.so" "$BUILD_DIR/lib/$ABI_DIR/libsecurity_test.so"

echo ""
echo "=== Step 3: Compile resources with aapt2 ==="
"$AAPT2" compile \
    --dir "$APK_SRC/res" \
    -o "$BUILD_DIR/compiled_res/"

echo ""
echo "=== Step 4: Link resources into base APK ==="
"$AAPT2" link \
    -I "$ANDROID_JAR" \
    --manifest "$APK_SRC/AndroidManifest.xml" \
    --java "$BUILD_DIR/gen" \
    --auto-add-overlay \
    -o "$BUILD_DIR/base.apk" \
    "$BUILD_DIR/compiled_res/"*.flat

echo ""
echo "=== Step 5: Compile Java source ==="
R_JAVA=$(find "$BUILD_DIR/gen" -name "R.java" | head -1)
echo "R.java at: $R_JAVA"

mkdir -p "$BUILD_DIR/classes"
javac \
    -source 11 -target 11 \
    -classpath "$ANDROID_JAR" \
    -d "$BUILD_DIR/classes" \
    "$R_JAVA" \
    "$APK_SRC/src/SecurityTestActivity.java"

echo ""
echo "=== Step 6: Dex the classes ==="
mkdir -p "$BUILD_DIR/dex_output"
"$D8" \
    --min-api 28 \
    --output "$BUILD_DIR/dex_output" \
    "$BUILD_DIR/classes/com/sectest/bindersandbox/"*.class
echo "DEX file: $BUILD_DIR/dex_output/classes.dex"

echo ""
echo "=== Step 7: Build final APK ==="
cp "$BUILD_DIR/base.apk" "$BUILD_DIR/unsigned.apk"

# Add classes.dex.
cd "$BUILD_DIR/dex_output"
zip -j "$BUILD_DIR/unsigned.apk" classes.dex
cd "$PROJECT_ROOT"

# Add native library.
cd "$BUILD_DIR"
zip -r "$BUILD_DIR/unsigned.apk" lib/
cd "$PROJECT_ROOT"

echo ""
echo "=== Step 8: Zipalign ==="
"$ZIPALIGN" -f 4 "$BUILD_DIR/unsigned.apk" "$BUILD_DIR/aligned.apk"

echo ""
echo "=== Step 9: Generate signing key ==="
KEYSTORE="$BUILD_DIR/debug.keystore"
if [ ! -f "$KEYSTORE" ]; then
    keytool -genkeypair \
        -v \
        -keystore "$KEYSTORE" \
        -alias sectest \
        -keyalg RSA \
        -keysize 2048 \
        -validity 10000 \
        -storepass android \
        -keypass android \
        -dname "CN=SecurityTest, OU=Test, O=Test, L=Test, ST=Test, C=US"
fi

echo ""
echo "=== Step 10: Sign APK ==="
"$APKSIGNER" sign \
    --ks "$KEYSTORE" \
    --ks-key-alias sectest \
    --ks-pass pass:android \
    --key-pass pass:android \
    --out "$OUTPUT_APK" \
    "$BUILD_DIR/aligned.apk"

echo ""
echo "=== Done ==="
echo "APK: $OUTPUT_APK"
echo "Size: $(du -h "$OUTPUT_APK" | cut -f1)"
echo ""
echo "Install: adb install $OUTPUT_APK"
echo "Run:     adb shell am start -n com.sectest.bindersandbox/.SecurityTestActivity"
echo "Logs:    adb logcat -s BinderSecTest:V"
