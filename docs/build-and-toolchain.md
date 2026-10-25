# Android Build Toolchain (Gradle Release Pipeline)

GhostHID builds its Android APK with the Gradle wrapper inside [`android/`](../android) — Android Gradle Plugin 8.7.2, Kotlin 2.0.21, Jetpack Compose. The build always produces a **release** artifact; there is no debug build in the deployment path.

The former zero-Gradle pipeline (`aapt2` → `javac` → `d8` → `zipalign` → `apksigner`) and the legacy `com.sj010.ghid_headless` sources it compiled were removed along with the headless app. Nothing in the repo invokes those tools anymore.

---

## 1. Entry points

| Command | What it runs | Output |
|---|---|---|
| `./build.sh` | `android/gradlew :app:assembleRelease` | `build/ghost-hid.apk` |
| `ghid build` | `bash build.sh` (falls back to `gradlew` if `build.sh` is absent) | `build/ghost-hid.apk` |
| `ghid install-apk` | `adb install -r` of `build/ghost-hid.apk` | APK on the phone |

`ghid install-apk` resolves the artifact in this order:

1. `build/ghost-hid.apk` (repo root, published by `build.sh`)
2. `android/app/build/outputs/apk/release/app-release.apk` (raw Gradle output)
3. `ghost-hid.apk` (explicit path passed as an argument)

An explicit path always wins: `ghid install-apk /path/to/app.apk`.

---

## 2. What Gradle does

`android/app/build.gradle.kts`:

- **Application ID** `com.sj010.ghid`, `minSdk 28`, `targetSdk 35`, `compileSdk 35`.
- **Release build type:** `isMinifyEnabled = true` (R8 code shrinking), `isShrinkResources = true`,
  `proguard-android-optimize.txt` plus `android/app/proguard-rules.pro` (which currently only
  silences AndroidX security-annotation warnings).
- **Signing:** both debug and release sign with the repo's `debug.keystore`
  (`androiddebugkey`, password `android`), so a release APK installs over a debug install
  without re-pairing Bluetooth.
- **Compose:** enabled through the Compose compiler plugin and the Compose BOM
  (`2024.10.00`), plus `androidx.graphics:graphics-shapes` for the Material 3 Expressive
  shapes.
- **Targets the release artifact only.** `assembleDebug` still exists as a local development
  convenience, but it is never published to `build/` and never installed by `ghid`.

Build stages:

```text
:app:compileReleaseKotlin → :app:compileReleaseJavaWithJavac
    → :app:minifyReleaseWithR8 → :app:shrinkReleaseResources
    → :app:packageRelease → :app:signRelease
    → android/app/build/outputs/apk/release/app-release.apk
    → cp build/ghost-hid.apk
```

---

## 3. Environment prerequisites

1. **JDK 17** (`JAVA_HOME` pointing at it).
2. **Android SDK** with platform `android-35` and current build-tools; `android/local.properties`
   must contain `sdk.dir=...` (the file is gitignored — create it once).
3. **Network access on the first build** so Gradle can resolve AGP, Kotlin, and AndroidX
   artifacts; subsequent builds use the wrapper caches in `android/.gradle` and
   `~/.gradle`.

On macOS with Homebrew:

```bash
brew install openjdk@17 android-commandlinetools
export ANDROID_HOME="/opt/homebrew/share/android-commandlinetools"
export JAVA_HOME="/opt/homebrew/opt/openjdk@17/libexec/openjdk.jdk/Contents/Home"
```

Build and deploy in a single command:

```bash
./build.sh && ghid install-apk
```

After installing, restart the gateway so the new package state is picked up:

```bash
ghid restart && ghid connect && ghid status
```

---

## 4. Why release, not debug

- R8 strips unused Compose/AndroidX code, so the shipped APK stays small despite the UI stack.
- One artifact: what `ghid build` produces is byte-for-byte what `ghid install-apk` deploys,
  which keeps `ghid doctor --json` results reproducible across machines.
- The signed release build keeps the same package identity (`com.sj010.ghid`), so
  `adb install -r` upgrades in place and the existing Windows Bluetooth pairing survives.

> Changing the HID descriptor, report IDs, or HID subclasses still invalidates a cached
> Windows pairing — that is a protocol change, not a build change. Re-pair GhostHID on
> Windows after any descriptor edit.
