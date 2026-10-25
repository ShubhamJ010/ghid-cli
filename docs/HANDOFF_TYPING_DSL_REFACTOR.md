# Handoff: Unified Bracket Sequence DSL & Payload Subsystem

## Mission
Full codebase refactor to replace the legacy combo/snippet syntax (`->`, `%`, `~`, `!`, `type:`, `raw:`) with a unified bracket sequence DSL (`{...}`), rename "Snippet" to "Payload" throughout all layers (Go, Android, CLI, Docs), and support auto-provisioning raw payloads on combo saves.

**Constraint:** ZERO backward compatibility required. Completely remove deprecated tokens, legacy shims, and old syntax.

---

## 1. Sequence DSL Specification (`{...}`)

### Core Grammar
- **Plain text outside `{}`**: Emitted directly as virtual keyboard typing.
- **Inside `{...}`**: Evaluated in deterministic priority order:
  1. `{{` -> Literal `{`.
  2. Number + `ms` or `s` (`{500ms}`, `{1s}`) -> Execution delay / sleep.
  3. Contains `+` (`{mod+s}`, `{ctrl+alt+q}`) -> Keyboard chord.
     - Modifiers: `mod` (Cmd on macOS, Ctrl on Win/Linux), `ctrl`, `alt`/`option`, `shift`, `win`/`meta`.
  4. Standard W3C / HID key name (`{Tab}`, `{Enter}`, `{Escape}`, `{VolumeUp}`) -> Single keypress or consumer media event.
  5. Alphanumeric / Identifier (`{EMAIL}`, `{PASSWORD}`, `{WIN_SLEEP}`) -> Named **Payload** reference.

### Canonical Target Example
```text
samxn81@gmail.com{Tab}{PASSWORD}{Enter}{500ms}{mod+s}{100ms}{WIN_SLEEP}
```

---

## 2. Payload Subsystem Architecture (Rename "Snippet" -> "Payload")

### Rationale
"Snippet" traditionally implies text only. In GhostHID, records hold text credentials, secure secrets, AND raw HID hex report bytes (e.g. ACPI sleep `02`, consumer play/pause `cd00`). "Payload" accurately describes the USB HID transmission unit.

### Schema
```go
type Payload struct {
    Name      string    `json:"name"`      // Identifier (e.g. "EMAIL", "WIN_SLEEP")
    Type      string    `json:"type"`      // "type" (text keystrokes) or "raw" (raw HID hex report)
    Value     string    `json:"value"`     // Plain text string or hex bytes (e.g. "02", "cd00")
    Secure    bool      `json:"secure"`    // Local Keychain/Keystore only; NEVER synced across network
    CreatedAt time.Time `json:"created_at"`
    UpdatedAt time.Time `json:"updated_at"`
}
```

### Auto-Provisioning on Combo Save
Combos bundle payload definitions:
```json
{
  "name": "LockAndSleep",
  "sequence": "{mod+l}{500ms}{WIN_SLEEP}",
  "payloads": {
    "WIN_SLEEP": { "type": "raw", "value": "02" }
  }
}
```
When a combo is saved:
1. Engine extracts all `{NAME}` tokens.
2. If token is not in `PayloadStore` but present in bundled `payloads`, it is automatically validated and saved into `PayloadStore`.
3. Sequence remains clean with readable `{NAME}` tokens instead of embedded hex.

---

## 3. Scope of Work by Layer

### Layer 1: Go Core (`internal/hid`, `internal/payloads`, `internal/combos`, `internal/gateway`)
1. **`internal/hid`**:
   - Implement single-pass regex tokenizer for `\{\{|\{([^}]+)\}`.
   - Delete `ParseActionSequence`, `->` arrow splitting, `type:`, `text:`, and `raw:` prefixes.
   - Map `mod` dynamically: macOS -> `Cmd`, Windows/Linux -> `Ctrl`.
2. **`internal/payloads` (formerly `internal/snippets`)**:
   - Rename package and structs: `Snippet` -> `Payload`, `SnippetStore` -> `PayloadStore`.
   - Update Keychain service names and store persistence (`payloads.json`).
3. **`internal/combos`**:
   - Update `Combo` struct: field `Keys` -> `Sequence`.
   - Add `Payloads map[string]Payload` for auto-provisioning.
   - Update default combos (`Login`, `Sleep`, `PlayPause`).
4. **`internal/gateway` & `internal/syncer`**:
   - Dispatch `Payload` based on `Type == "raw"` (raw HID report) vs `Type == "type"` (keystroke typing).
   - Sync only non-secure `Payload` entries.

### Layer 2: CLI & TUI (`internal/cli`, `internal/tui`)
1. **CLI Commands**:
   - `ghid snippets ...` -> `ghid payload [list|get|set|delete]`.
   - Update flags and descriptions.
2. **TUI (Bubble Tea)**:
   - "Snippet Studio" -> "Payload Studio".
   - Combo Studio: update live preview and step cards to render `{...}` sequence tokens.

### Layer 3: Android App (`android/app/...`)
1. **Domain & Data**:
   - Rename `Snippet` entity, DAO, and repository to `Payload`.
   - Update Room schema / migrations.
2. **UI (Jetpack Compose & Material 3 Expressive)**:
   - `SnippetsShade` -> `PayloadsShade`.
   - `SnippetEditorPage` -> `PayloadEditorPage`.
   - Maintain `RAW` vs `TEXT` badges, "Send" (raw) vs "Type" (text) buttons.
3. **DSL Engine**:
   - Re-implement sequence parser in Kotlin to match the Go `{...}` tokenizer.

### Layer 4: Documentation & ADRs
1. Create `docs/decisions/ADR-015-unified-bracket-sequence-dsl-and-payloads.md`.
2. Update `README.md`, `man/ghid.1`, `docs/architecture.md`, `docs/flows.md`.

---

## 4. Verification & Testing
- Go tests: `go test -v ./...`
- Android tests: `./gradlew test` (or build apk: `./gradlew assembleDebug`)
- End-to-end verification:
  - Sequence `samxn81@gmail.com{Tab}{PASSWORD}{Enter}{500ms}{mod+s}{100ms}{WIN_SLEEP}` executes without syntax errors.
