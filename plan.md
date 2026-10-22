# PLAN.md -- GhostHID Standalone Android App & M3 Expressive

## Phase 1: Architecture, ADR & Seams
- [ ] Write `ADR-006` in `docs/decisions/`: Compose M3 Expressive UI, Service Binder, Keystore Vault, State Sync.
- [ ] Write `design.md`: Deep module boundaries, Mermaid sequence/flowcharts, M3 token spec, data models.
- [ ] Define seams:
  - `BluetoothHidBridge`: abstracts `BluetoothHidDevice` calls.
  - `ComboRepository`: abstracts disk read/write for combos.
  - `SnippetRepository`: abstracts plaintext + secure secret retrieval.
  - `Vault`: Android Keystore adapter matching Go `vault.Vault`.

## Phase 2: Monorepo Restructure & Build Engine
- [ ] Migrate raw `src/` to standard Android Gradle structure: `android/app/src/main/java/` & `android/app/src/main/kotlin/`.
- [ ] Setup Gradle wrapper (`gradlew`), AGP 8.x+, Kotlin 2.0+, Jetpack Compose BOM 2024+.
- [ ] Add dependencies:
  - `androidx.compose.material3:material3`
  - `androidx.compose.material:material-icons-extended`
  - `androidx.navigation:navigation-compose`
  - `androidx.security:security-crypto` (Keystore AES-256)
  - `androidx.lifecycle:lifecycle-viewmodel-compose`
- [ ] Update `build.sh` -> invoke `./gradlew :app:assembleDebug` or fallback.
- [ ] Update `AndroidManifest.xml` -> add `MainActivity` with `android.intent.action.MAIN` + `LAUNCHER` intent filter.

## Phase 3: Kotlin HID Domain Core (Parity with Go)
- [ ] Port `internal/hid` AST parser to Kotlin:
  - Action sequence parser (`step ->{delay} step`).
  - Key token validator (Windows US-ASCII keys).
  - Variable extractor (`$USERNAME`, `$PASSWORD`).
  - Mouse command serializer.
- [ ] Implement `AndroidVault`:
  - Backed by `EncryptedSharedPreferences` + Android `KeyStore`.
  - Store `$PASSWORD` & secure snippet values with hardware-backed encryption.
- [ ] Implement `ComboStore` & `SnippetStore` in Kotlin:
  - File path: app external files dir (`combos.json`, `snippets.json`).
  - Public readable by ADB shell (`Context.getExternalFilesDir`).
  - JSON schema 100% identical to Go struct.

## Phase 4: Service Deepening & Dual IPC
- [ ] Refactor `HidService`:
  - Keep `BroadcastReceiver` for ADB commands (Mac CLI intact).
  - Add `LocalBinder` / Kotlin `StateFlow` for Compose UI.
  - Expose reactive flows: `connectionState`, `targetDevice`, `registered`, `logs`.
  - Support direct method invocation: `executeCombo(name/keys)`, `typeText(text)`, `sendMouse(action)`.
  - Wire Kotlin `SnippetStore` resolver into `typeText` / `executeCombo` for local variable expansion.

## Phase 5: Material 3 Expressive UI
- [ ] **Theme**:
  - `Theme.kt`: Dynamic color via `dynamicDarkColorScheme` / `dynamicLightColorScheme`.
  - Expressive custom color palette fallback (Deep Purple / Neon Cyan accents).
  - Shape tokens: Extra-large corners (28dp dialogs, 20dp cards, full-pill buttons).
  - Spring motion specs: `spring(dampingRatio = LowBouncy, stiffness = Medium)`.
- [ ] **Navigation**:
  - Adaptive scaffold (`NavigationSuiteScaffold` or BottomNav for compact, NavRail for wide/foldable).
- [ ] **Screen 1: Dashboard**:
  - Live Bluetooth status card with pulsing status dot.
  - Paired targets dropdown / connection switcher.
  - Hero action buttons: Win+L Lock, Sleep countdown, Connect/Disconnect.
  - Real-time terminal log viewer.
- [ ] **Screen 2: Quick Send**:
  - Text input box with auto-type button.
  - Common chords grid: Win+D, Alt+Tab, Ctrl+Shift+Esc, Win+R.
  - Custom chord builder with interactive modifier chips (Win, Ctrl, Alt, Shift).
- [ ] **Screen 3: Combo Studio**:
  - Card list of combos with favorite toggle (❤️).
  - Visual step pills (`[Win+R] -> [type:notepad] -> [Enter]`).
  - Floating Action Button (FAB) -> Combo Editor with live grammar validator.
  - One-tap "Run" trigger with visual step progress.
- [ ] **Screen 4: Snippet Studio**:
  - Snippet cards with secure shield icon for encrypted items.
  - Masked value display (`••••••••`) with tap-to-reveal toggle.
  - Add/Edit dialog with "Mark as Secure (Hardware Keystore)" toggle.
- [ ] **Screen 5: Virtual Mouse / Trackpad**:
  - Full-screen touch surface reporting delta X/Y to `sendMouseReport`.
  - Left click, right click, middle click buttons.
  - Side scroll slider with haptic ticks.

## Phase 6: State Sync & Mac Go CLI Parity
- [ ] Create `internal/gateway/sync.go` in Go:
  - Add `ghid sync` CLI subcommand:
    - `ghid sync pull`: read phone `/sdcard/Android/data/com.poc.hiddebug/files/` -> merge local `~/.config/ghid/`.
    - `ghid sync push`: write local -> phone external storage.
    - `ghid sync status`: compare checksums / timestamps.
- [ ] Add secure snippet sync protocol:
  - If syncing secure snippet, prompt or use authenticated ADB transfer.
- [ ] Update `README.md`, `man/ghid.1`, and `docs/agent-integration.md`.

## Phase 7: Verification & Hardening
- [ ] Run `gofmt` + `go test ./... -count=1`.
- [ ] Compile Android app via Gradle, ensure 0 warnings/errors.
- [ ] Run Android local unit tests for AST parser, Keystore vault, Combo repository.
- [ ] Test dual mode:
  1. Trigger combo from Android app button.
  2. Trigger combo from Mac `ghid combo send Login`.
  3. Verify phone service handles both concurrently without race conditions.
