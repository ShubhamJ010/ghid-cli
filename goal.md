# GOAL.md -- GhostHID Standalone Android App (M3 Expressive)

## Objective
Phone = standalone HID controller. No Mac needed on desk.
Mac CLI intact. ADB IPC intact. Zero regression.
Phone get full Material 3 Expressive UI.
Feature parity with CLI + TUI: status, quick-send, combo studio, snippet studio, mouse trackpad.
Shared state: combos + snippets sync phone <-> Mac. Secrets locked in hardware keystore.

## Key Targets

### 1. Standalone Device UI
- Phone run app direct. Tap button -> send Win+L, type text, run combo, steer mouse.
- Background `HidService` stay alive via foreground service + notification.
- UI attach to service local via Binder / Flow. No ADB needed on device.
- Remote ADB broadcasts still work identical when Mac plug in. Dual control.

### 2. Material 3 Expressive UI
- Compose M3 Expressive design system.
- Dynamic color (Monet palette, API 31+). Fallback custom deep tonal dark/light scheme.
- Shapes: M3 rounded (large-increased 20dp, extra-large 28dp, full pill buttons).
- Motion: Spring physics on card click, list item reorder, trackpad feedback.
- Navigation: Adaptive scaffold (Bottom Nav on phone, Nav Rail on foldable/tablet).
- 5 Screens:
  1. **Dashboard**: BT status, target device card, connection pill, quick actions.
  2. **Quick Send**: Direct text input, chord builder, sleep countdown, instant lock.
  3. **Combo Studio**: Card list with favorites top, AST visualizer, step editor, run trigger.
  4. **Snippet Studio**: Key-value list, eye toggle for masking, secure badge.
  5. **Mouse Trackpad**: Full-screen touch surface, left/right buttons, scroll wheel strip.

### 3. Shared State & Secure Vault
- **Combos**:
  - Format: JSON array identical to Mac `~/.config/ghid/combos.json`.
  - Path phone: `/sdcard/Android/data/com.poc.hiddebug/files/combos.json` (also internal cache).
  - Sync: `ghid sync` push/pull over ADB, or auto-detect on connection.
- **Snippets**:
  - Format: JSON array matching Mac `snippets.json`.
  - Non-secure: stored in JSON file.
  - Secure (`secure: true`, e.g. `$PASSWORD`):
    - Mac -> macOS Keychain (`/usr/bin/security`).
    - Phone -> Android Keystore (`EncryptedSharedPreferences` / AES-256-GCM MasterKey).
    - Plaintext never touch disk or git. Wire transfer over ADB encrypted or manual prompt.

### 4. Monorepo Architecture & Maintainability
- Monorepo layout:
  - `android/` -> Standard modern Gradle multi-module / clean app module (Kotlin + Jetpack Compose).
  - `cmd/ghid`, `internal/` -> Go CLI (unchanged core + add sync command).
  - `docs/` -> Specs, ADRs, guides.
- Build system:
  - Support Gradle CLI (`./gradlew assembleDebug`) for full Compose APK.
  - Keep `build.sh` updated or bridge to Gradle wrapper.
  - Zero break to Go test / Go build pipeline.

## Success Criteria
- [ ] Phone APK installable, launchable from Android launcher.
- [ ] Full local control: pair BT, connect target, send chords, mouse move without Mac.
- [ ] Mac `ghid` CLI + TUI continue to work seamlessly over ADB without code changes.
- [ ] Combos created on Mac appear on phone after sync; combos made on phone work on Mac.
- [ ] Passwords stored in Android hardware Keystore; resolved locally on phone inside `HidService`.
- [ ] `go test ./...` pass 100%. Android unit tests pass 100%.
