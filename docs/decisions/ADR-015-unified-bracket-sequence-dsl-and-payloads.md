# ADR-015: Unified Bracket Sequence DSL and Payload Subsystem

## Status
Accepted

## Date
2026-10-25

## Context
Macro sequences were previously split across ad-hoc, divergent syntax tokens: arrow step separators (`->`, `->{0.5}`), prefix verbs (`type:`, `text:`, `raw:`), sigils (`%`, `~`, `!`), and dollar-prefixed variable references (`$VAR`, `${VAR}`). This design had severe flaws:
1. Typing text with spaces, punctuation, or dashes frequently triggered edge-case delimiter collisions.
2. The Go and Kotlin sequence engines had subtle syntactic divergences.
3. Terminology was fragmented between "Snippets" (variables) and macro steps.
4. Exporting or syncing combos that referenced low-level raw HID reports (such as ACPI sleep or consumer media) required manual out-of-band setup of snippets on every device.

## Decision

1. **Zero Backward Compatibility & Unified Bracket Sequence DSL**:
   - Completely dropped legacy syntax: `->`, `->{...}`, `%`, `~`, `!`, `type:`, `raw:`, `$VAR`, and `${VAR}`.
   - Plain text outside curly braces `{}` is typed literally as HID keystrokes.
   - Escaped opening braces `{{` evaluate to a literal `{`.
   - All macro directives are enclosed within single `{...}` brackets and evaluated according to deterministic 5-tier priority:
     1. **Escaped Bracket**: `{{` evaluates to literal `{` in typed text.
     2. **Execution Delay**: Numbers followed by `ms` or `s` (e.g. `{500ms}`, `{1s}`, `{100ms}`) trigger synchronous execution pauses.
     3. **Keyboard Chord**: Tokens containing `+` (e.g. `{mod+s}`, `{ctrl+alt+del}`, `{shift+mod+p}`) trigger simultaneous modifier + key combinations. The `mod` modifier dynamically maps to GUI/Command on macOS and Ctrl on Windows/Linux.
     4. **Standard & Consumer Keys**: W3C and standard HID key names (e.g. `{Tab}`, `{Enter}`, `{Escape}`, `{Del}`, `{Space}`, `{Up}`, `{F1}`-`{F12}`) or consumer media actions (e.g. `{VolumeUp}`, `{VolumeDown}`, `{Mute}`, `{PlayPause}`) and mouse actions (e.g. `{click-left}`).
     5. **Named Payload Reference**: Alphanumeric/identifier tokens (e.g. `{PASSWORD}`, `{EMAIL}`, `{WIN_SLEEP}`) resolve dynamically against the payload vault. If the payload is raw hex, it dispatches raw HID reports; if text, it types the resolved string.
   - **Canonical Macro Example**:
     ```text
     samxn81@gmail.com{Tab}{PASSWORD}{Enter}{500ms}{mod+s}{100ms}{WIN_SLEEP}
     ```

2. **Subsystem Renaming: "Snippet" -> "Payload"**:
   - Renamed the concept of "Snippet" to "Payload" across the entire stack:
     - **Go**: `internal/payloads` package, `Payload` model, `PayloadStore`, CLI command `ghid payload [list|get|set|delete]`.
     - **TUI**: Replaced "Snippet Studio" with "Payload Studio".
     - **Android**: `Payload` model, `PayloadRepository`, storage in `payloads.json`, `PayloadsShade`, `PayloadStudioScreen`.

3. **Self-Contained Combos & Auto-Provisioning**:
   - Combos now store `sequence` (replacing legacy `keys`) and bundle `payloads: map[string]Payload`.
   - When saving or syncing combos, if a referenced payload (such as raw report `{WIN_SLEEP}` or `{PLAY_PAUSE}`) does not exist in the recipient's local payload store, it is automatically provisioned into the `PayloadStore`/`PayloadRepository`.

4. **Synchronizer and ADB Transport**:
   - Gateway and syncer endpoints transport `Payload` structures and `sequence` fields.
   - Synchronization pushes and pulls payload files (`payloads.json`) while maintaining strict local security enclave boundaries for sensitive credentials.

## Consequences
- Single, consistent, and intuitive DSL shared identically across Go and Kotlin engines.
- Combos are completely portable and self-sufficient across systems without manual snippet prerequisite setup.
- Clean and unambiguous separation between typed literals and macro directives.
