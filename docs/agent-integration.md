# Automation and Agent Integration Guide

This guide details how AI agents, CI runners, and automated scripts interface with GhostHID without human intervention.

---

## 1. Integration Principles

GhostHID provides a non-interactive interface designed for programmatic execution:
- **`--json` Flag:** When passed to any command, interactive terminal prompts and ANSI escape codes are disabled. Output is emitted strictly as valid JSON to standard output.
- **Exit Codes:**
  - `0`: Operation succeeded or condition verified.
  - `1`: Operation failed, device disconnected, validation error, or resource unavailable.
- **Standard Error Isolation:** Fatal process diagnostics or ADB stderr messages are isolated to stderr so JSON parsers on stdout remain intact.

---

## 2. Complete JSON Schemas

### `ghid status --json`
Inspects live connection status between phone and target computer.

#### Response
```json
{
  "registered": true,
  "connected": true,
  "state": "CONNECTED",
  "target": "LP-TZD-0X114650",
  "address": "38:FC:98:E8:E3:87",
  "os": "windows",
  "os_confidence": "high",
  "detection_method": "hid_led_handshake",
  "updated": "2026-10-23 01:15:30.122"
}
```

`os` is the passively fingerprinted host (`windows`, `macos`, `linux`, or `unknown`).
`os_confidence` is `high`, `medium`, `low`, or `none`; `detection_method` is one of
`apple_oui`, `apple_sdp_uuid`, `hid_led_handshake`, `name_heuristic`, `fallback`,
or `none`. Older Android builds omit all three fields; treat them as absent, not
as an error.

Exit code: `0` if `connected == true`, `1` if disconnected.

---

### `ghid doctor --json`
Runs a complete hardware, transport, and credentials health check.

#### Response
```json
{
  "version": "1.0.0",
  "adb_path": "/opt/homebrew/bin/adb",
  "adb_available": true,
  "device_connected": true,
  "device_model": "KB2001",
  "device_name": "SJ's OnePlus 8T",
  "device_serial": "192.168.1.80:41839",
  "apk_installed": true,
  "bluetooth_on": true,
  "gateway_registered": true,
  "gateway_connected": true,
  "target_laptop": "LP-TZD-0X114650",
  "target_address": "38:FC:98:E8:E3:87",
  "selected_device": {
    "id": "dev-1",
    "name": "LP-TZD-0X114650",
    "address": "38:FC:98:E8:E3:87",
    "platform": "windows",
    "selected": true
  },
  "registered_devices": 1,
  "keychain_available": true,
  "cached_user": "Administrator",
  "has_cached_password": true,
  "snippets_count": 3
}
```

`apk_installed` reflects package `com.sj010.ghid`. Exit code: `0` whenever a report is
produced (judge health from the booleans), `2` if the ADB gateway cannot be constructed — then
stdout carries `{"error": "..."}` instead. The human-readable `ghid doctor` run prints the
per-check list; the JSON form carries the same facts as flat booleans.

---

### `ghid combo schema --json`
Returns the complete list of valid key tokens, modifier names, and mouse actions.

#### Response
```json
{
  "grammar": "step ->{seconds} step ->{seconds} step",
  "step_types": [
    "keyboard chord: ctrl+shift+esc",
    "mouse gesture: click-left, scroll-up",
    "variable injection: $USERNAME, $PASSWORD, ${PIN}",
    "typed text: type:rustdesk or type:\"rust desk\""
  ],
  "keyboard": ["win", "ctrl", "alt", "shift", "enter", "esc", "tab", "space", "..."],
  "mouse": ["click-left", "click-right", "click-middle", "double-click", "scroll-up", "scroll-down"],
  "os_tags": ["windows", "macos", "linux"],
  "default_os": "windows",
  "emoji": "optional single emoji shown as the combo icon on Android and instead of the OS badge in the CLI",
  "os_note": "The win token is the HID GUI modifier: Windows/Linux Super, macOS Command."
}
```

---

### `ghid combo validate "<sequence>" --json`
Parses and validates a chained combo sequence without executing it.

#### Success Response
```json
{
  "valid": true,
  "input": "win+r ->{0.5} type:notepad ->{0.3} enter",
  "steps": [
    {"type": "keyboard", "value": "win+r", "delay_ms": 500},
    {"type": "text", "value": "notepad", "delay_ms": 300},
    {"type": "keyboard", "value": "enter", "delay_ms": 0}
  ]
}
```

#### Error Response
```json
{
  "valid": false,
  "input": "cmd+space",
  "error": "unsupported key token: \"cmd\" (macOS-only alias, use Windows tokens)"
}
```

Exit code: `0` if valid, `1` if invalid.

---

### `ghid combo list --json`
Returns all saved combos in the local library (`~/.config/ghid/combos.json`).

#### Response
```json
[
  {
    "name": "Login",
    "keys": "ctrl+alt+del ->{1.0} esc ->{0.8} $USERNAME ->{0.8} tab ->{0.8} $PASSWORD ->{0.8} enter",
    "os": "windows",
    "emoji": "🔑",
    "description": "Windows unlock & credential injection",
    "favorite": true,
    "created_at": "2026-10-23T01:00:00Z"
  },
  {
    "name": "Lock",
    "keys": "win+l",
    "os": "windows",
    "emoji": "🔒",
    "description": "Instant Windows lock screen (Win + L)",
    "favorite": true,
    "created_at": "2026-10-23T01:00:00Z"
  },
  {
    "name": "Sleep",
    "keys": "raw:02 ->{0.06} raw:00",
    "os": "windows",
    "emoji": "😴",
    "description": "Hardware ACPI sleep sequence",
    "favorite": true,
    "created_at": "2026-10-23T01:00:00Z"
  }
]
```

Filter the library by target OS with `ghid combo list --os macos --json`.
Combos saved before OS tagging existed report `os: "windows"`. Tag a combo at
save time with `--os windows|macos|linux`; without the flag, updates keep the
existing tag. The same `os` value selects the matching filters and profile in
the Android app.

The optional `emoji` string is combo icon metadata. When present it overrides the
OS logo on the Android card and replaces the `[win]` / `[mac]` / `[linux]` badge
in `ghid combo list` and the TUI. Set it at save time with `--emoji <e>`; without
the flag, updates keep the existing emoji. Values must be an emoji (no ASCII
letters, digits or spaces, at most 12 code points) or the save fails with
`{"status":"error","error":"..."}` and exit 1.

---

### `ghid combo fav <name> --json`
Toggles the pinned favorite status (`❤️`) of a combo.

#### Response
```json
{
  "status": "success",
  "name": "Open Terminal",
  "favorite": true
}
```

---

### `ghid quick <text...> [--raw] --json`
Unified ad-hoc transmission. Types text strings through the virtual keyboard by default, or transmits raw 8-byte HID reports when `--raw` is specified.

> **Note:** Replaces legacy root `ghid type` and `ghid raw` commands (which have been removed without backward-compatibility aliases).

#### Default Type Mode
```bash
ghid quick "Hello World" --json
```
```json
{
  "status": "success",
  "mode": "type",
  "typed": "Hello World"
}
```

#### Raw 16-Hex HID Report Mode
```bash
ghid quick --raw "08000f0000000000" --json
```
```json
{
  "status": "success",
  "mode": "raw",
  "raw": "08000f0000000000"
}
```

---

### `ghid snippets <action> --json`
Inspects and manages text variables and encrypted secrets stored in `~/.config/ghid/snippets.json` (and macOS Keychain).

#### List Snippets
```bash
ghid snippets list --json
```
```json
[
  {
    "name": "username",
    "value": "Administrator",
    "is_secure": false
  },
  {
    "name": "password",
    "value": "••••••••",
    "is_secure": true
  }
]
```

#### Get Snippet
```bash
ghid snippets get username --json
```
```json
{
  "name": "username",
  "value": "Administrator",
  "is_secure": false
}
```

#### Set Snippet
```bash
ghid snippets set api_token "secret_abc123" --secure --json
```
```json
{
  "status": "success",
  "name": "api_token",
  "is_secure": true
}
```

#### Delete Snippet
```bash
ghid snippets delete api_token --json
```
```json
{
  "status": "success",
  "deleted": "api_token"
}
```

---

### `ghid devices scan --json`
Queries the phone directly over ADB (`dumpsys bluetooth_manager`) for bonded Bluetooth devices. Enables zero-touch registration without needing to manually transcribe MAC addresses.

#### Response
```json
[
  {
    "address": "A0:9A:8E:57:36:DA",
    "name": "Shubham’s MacBook Air",
    "type": "DUAL"
  },
  {
    "address": "38:FC:98:E8:E3:87",
    "name": "LP-TZD-0X114650",
    "type": "BR/EDR"
  }
]
```

---

### `ghid devices list --json`
Lists all configured target hosts.

#### Response
```json
[
  {
    "id": "dev-1",
    "name": "LP-TZD-0X114650",
    "address": "38:FC:98:E8:E3:87",
    "platform": "windows",
    "selected": true
  },
  {
    "id": "dev-2",
    "name": "MacBook Pro M3",
    "address": "F8:FF:C2:11:22:33",
    "platform": "macos",
    "selected": false
  }
]
```

---

## 3. Automation Recipes

### Bash: Safe Pre-Flight and Keystroke Injection
```bash
#!/usr/bin/env bash
set -euo pipefail

# 1. Verify connection status
if ! ghid status --json >/dev/null 2>&1; then
    echo "Host disconnected. Attempting reconnect..."
    ghid reconnect || {
        echo "Failed to connect to target host" >&2
        exit 1
    }
    sleep 2
fi

# 2. Inject command
ghid combo "win+r ->{0.5} type:cmd ->{0.3} enter"
```

### Python: Automated Diagnostic and Action Dispatch
```python
import json
import subprocess
import sys

def run_ghid(cmd: list[str]) -> dict:
    result = subprocess.run(["ghid", *cmd, "--json"], capture_output=True, text=True)
    if result.returncode != 0:
        raise RuntimeError(f"Command failed (exit {result.returncode}): {result.stderr or result.stdout}")
    return json.loads(result.stdout)

# Run health check
doctor = run_ghid(["doctor"])
if not doctor.get("gateway_connected"):
    print("Gateway not connected, attempting reconnection...")
    subprocess.run(["ghid", "reconnect"], check=True)

# Dispatch sequence
print("Dispatching combo sequence...")
subprocess.run(["ghid", "combo", "ctrl+alt+del ->{1.0} esc"], check=True)
```

### Node.js: Agent Tool Definition
```typescript
import { execFile } from "node:child_process";
import { promisify } from "node:util";

const exec = promisify(execFile);

export async function sendHostCombo(keys: string): Promise<{ success: boolean; output: string }> {
  // 1. Validate combo grammar
  const { stdout: validationOut } = await exec("ghid", ["combo", "validate", keys, "--json"]);
  const validation = JSON.parse(validationOut);
  if (!validation.valid) {
    throw new Error(`Invalid combo syntax: ${validation.error}`);
  }

  // 2. Dispatch sequence
  const { stdout } = await exec("ghid", ["combo", keys]);
  return { success: true, output: stdout.trim() };
}
```
