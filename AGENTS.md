# AGENTS.md

GhostHID = Android phone as Bluetooth HID keyboard + mouse.
`ghid` (Go, this Mac) → ADB → `com.sj010.ghid` on phone → Bluetooth HID → target PC.

One Android app only: the non-headless Material 3 app in `android/` (launcher UI + always-on
`HidService`). The old `com.sj010.ghid_headless` app, its `src/`, `res/`, root
`AndroidManifest.xml` and the zero-Gradle build were deleted with no backward compatibility —
Go, docs and build all speak `com.sj010.ghid` only. Want a UI-less gateway? Code your own from
`docs/prompts/headless-android-app.md`.

Status: alpha. Big refactors welcome. **No tests (unit or integration) — this repo has none, do not add any.** Verify by building, then exercising the CLI/JSON contract and the real device. Update docs, keep JSON contract stable in same change.

## Repo map

| Path | Job |
|---|---|
| `cmd/ghid` | binary entry |
| `internal/cli` | arg parsing, commands, JSON output, TUI launch |
| `internal/gateway` | `Controller` interface, validation, ADB calls |
| `internal/adb` | adb transport, broadcasts |
| `internal/hid` | key tokens, combo/mouse/text parsers, step descriptions |
| `internal/combos` | combo library at `~/.config/ghid/combos.json` |
| `internal/tui` | bubbletea dashboard + Combo Studio |
| `internal/config`, `vault`, `keychain` | config, credential storage |
| `android/app/src/main/java/com/sj010/ghid` | Android app: Compose UI + `service/HidService` + `service/HidCommandReceiver` |
| `build.sh`, `android/` | Gradle release build (`:app:assembleRelease` → `build/ghost-hid.apk`) |
| `docs/` | architecture, flows, protocol, agent integration, ADRs, prompts (`docs/prompts/`) |

Layer rule: `cli → gateway → adb → broadcast → Kotlin`. No direct `adb` calls from CLI/TUI. No business logic in `cmd/ghid`.

## Core commands

```bash
cd /Users/sj/Desktop/ws/ghost-hid

gofmt -w cmd internal
go vet ./...
go build ./...
./build.sh          # release APK -> build/ghost-hid.apk (Gradle; first run needs network)
```

## Update CLI on this Mac

```bash
cd /Users/sj/Desktop/ws/ghost-hid
gofmt -w cmd internal
go vet ./...
go build -o ghid ./cmd/ghid
go install ./cmd/ghid
./ghid install-bin
hash -r
ghid --version
```

Two install paths exist:

- `go install` → `~/go/bin/ghid` (copy)
- `./ghid install-bin` → `~/.local/bin/ghid` (symlink to repo binary) + man page

`~/go/bin` usually wins in PATH. Update both, or `rm ~/go/bin/ghid` once to use symlink only.
Resolve with `which ghid`.

## Update APK on this Mac

Phone must be visible to ADB first: `ghid doctor --json`.

```bash
cd /Users/sj/Desktop/ws/ghost-hid
./build.sh
ghid install-apk
ghid restart
sleep 2
ghid connect
ghid status
```

Want `SDP Registered: true` and `Connection: CONNECTED`.

If HID descriptor, report IDs, subclass, or mouse support changed:
**unpair GhostHID on Windows, then pair again.** Old pairing caches old descriptor. Kotlin-only or UI changes need install + restart, not re-pair.

## Update both

```bash
cd /Users/sj/Desktop/ws/ghost-hid
gofmt -w cmd internal
go vet ./...
./build.sh
ghid install-apk
ghid restart
go build -o ghid ./cmd/ghid
go install ./cmd/ghid
./ghid install-bin
sleep 2
ghid connect
ghid status
```

## Agent JSON contract

`--json` = non-interactive. No prompts, no ANSI, JSON on stdout, diagnostics on stderr.

- Exit `0` success, `1` error.
- Combo errors: `{"status":"error","error":"..."}` on stdout + exit 1.
- `combo schema --json` = grammar + every valid token. Agents discover, no hard-code.
- `combo validate <keys> --json` = parse without send/save.
- `combo save` = upsert, returns `updated: true|false`.
- `combo delete --json` = no confirmation.

```bash
ghid combo schema --json
ghid combo validate '{win+s}{700ms}rustdesk{400ms}{enter}' --json
ghid combo save "Open RustDesk" '{win+s}{700ms}rustdesk{400ms}{enter}' --json
ghid combo list --json
ghid combo send "Open RustDesk" --json
ghid combo delete "Open RustDesk" --json
```

Combo grammar: unified bracket sequence DSL `{chord}`, `{delay}`, `literal text`, `{PAYLOAD}`.
Delays: `{100ms}`, `{1s}`, `{0.5s}`.
Parameterized duration: `{TOKEN:duration}` (e.g. `{WIN_SLEEP:3s}`, `{VolumeUp:1.5s}`, `{Backspace:800ms}`, `{mod+s:500ms}`). Omitted duration defaults to safe momentary 40-60ms tap.
Cross-platform modifiers: `mod` (resolves to `cmd` on macOS, `win` on Windows/Linux), `ctrl`, `alt`/`opt`, `shift`.
Payload references: `{NAME}` (e.g. `{PASSWORD}`, `{WIN_SLEEP}`, `{PLAY_PAUSE}`).

## Engineering rules

- **No tests, unit or integration.** Do not write `_test.go`, `src/test`, `src/androidTest`, mocks/fakes for tests, or test dependencies. Verify behavior by building and running the real thing (CLI, JSON output, ADB device).
- One source of truth per concept. Tokens in `hid`, storage in `combos`, transport in `adb`.
- Prefer pure parsers/validators. Side effects at edges.
- Keep `gateway.Controller` small. Add method only when CLI truly needs it.
- New command → update `printHelp`, `man/ghid.1`, `README.md`, and `docs/agent-integration.md` if JSON shape changes.
- Preserve one-shot `ghid combo "win+d"`.
- Commit style: `feat:`, `fix:`, `refactor:`, `docs:`.
- Alpha means large refactors OK. Do not leave half-migrated layers, dead code, or duplicate parsers.
- **Android APK deployment & testing rule**: Android Package Manager (`adb install -r`) preserves existing processes in memory. Whenever deploying an APK, ALWAYS run `adb shell am force-stop com.sj010.ghid` before starting the app or testing via ADB/CLI so that newly compiled bytecode is executed rather than stale code in memory.
- **Momentary HID report auto-release & hold semantics**: HID raw reports are stateful switch reports. Momentary actions like Consumer Control Report 4 (e.g. `cd00` Play/Pause) or System Control Report 3 (e.g. `02` System Sleep) MUST be released with a zero report (`0000` / `00`) after ~60ms. `HidService.sendRawReport` automatically sends this zero release for non-zero Report 3 and Report 4 payloads. To hold buttons (e.g. Mac/Windows power/sleep hold for 3-5 seconds, Volume ramp, key repeat), use the parameterized duration suffix `{TOKEN:duration}` in sequence strings or `--hold <duration>` in CLI `ghid raw <hex> --hold 3s` / `--el hold_ms <ms>` in ADB.

## Android app design & iteration rules

- **Super new app & fast evolution**: Actively listen to user feedback and iterate immediately on the go.
- **Zero backward compatibility constraints for Android UI**: Do not preserve deprecated UI elements or hold onto legacy screens when the user asks to remove or change them (e.g. gateway terminal log completely removed from dashboard).
- **Material 3 Expressive shapes & styling**: Use official Google M3 Expressive shape catalog (4-petal clovers, scallops, soft diamonds, arches, soft triangles, hearts, pentagons, concave squircles) and vibrant high-contrast cyberpunk dark palettes.
- **Always verify on real connected device**: When APK is deployed to ADB device, take screenshots, review visual alignment, and polish touch targets directly.

## Gotchas

- Flags stripped globally (`--json`, `-s`). Text containing those tokens can break.
- Combo timestamps stored UTC. Local clock may show behind.
- `build.sh` only shells out to `android/gradlew :app:assembleRelease` and copies the release APK to `build/ghost-hid.apk`. `ghid install-apk` deploys release builds only (never debug).
- TUI is one bubbletea app. `esc` returns to dashboard, `ctrl+c` exits.
- Mouse needs report ID `2`; keyboard report ID `1`. Wrong descriptor = no mouse.
- `ghid combo` bare is interactive only on a TTY. Non-TTY falls back to list.

## Done checklist

- [ ] `gofmt` clean
- [ ] `go vet ./...` + `go build ./...` clean
- [ ] CLI rebuilt + installed in both paths
- [ ] APK rebuilt + installed if Kotlin/res/manifest changed
- [ ] service restarted + reconnected
- [ ] docs/man/help updated
- [ ] JSON output still parseable, exit codes unchanged
