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
  "updated": "2026-10-23 01:15:30.122"
}
```

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
  "checks": [
    {"name": "ADB Host Binary", "pass": true, "detail": "/opt/homebrew/bin/adb"},
    {"name": "Device Transport", "pass": true, "detail": "KB2001 (192.168.1.80:41839)"},
    {"name": "Gateway Package", "pass": true, "detail": "com.poc.hiddebug installed"},
    {"name": "Phone Bluetooth Radio", "pass": true, "detail": "Radio active"},
    {"name": "HID SDP Registration", "pass": true, "detail": "SDP registered"},
    {"name": "Host Connection", "pass": true, "detail": "Connected to LP-TZD-0X114650"},
    {"name": "macOS Keychain Vault", "pass": true, "detail": "Vault accessible, user Administrator cached"}
  ]
}
```

Exit code: `0` if all essential checks pass, `1` if any critical component is missing.

---

### `ghid combo schema --json`
Returns the complete list of valid key tokens, modifier names, and mouse actions.

#### Response
```json
{
  "syntax": "<step> [->{delay}] <step> ...",
  "modifiers": ["win", "gui", "ctrl", "control", "alt", "shift"],
  "keys": ["enter", "esc", "tab", "space", "backspace", "del", "insert", "up", "down", "left", "right", "home", "end", "pgup", "pgdn", "a", "b", "c", "..."],
  "mouse_actions": ["click-left", "click-right", "click-middle", "double-click", "scroll-up", "scroll-down"],
  "text_prefix": "type:<text> or type:\"<text with spaces>\""
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
    "name": "Open Terminal",
    "keys": "win+x ->{0.3} i",
    "created_at": "2026-10-23T01:00:00Z"
  },
  {
    "name": "Refresh Page",
    "keys": "ctrl+f5",
    "created_at": "2026-10-23T01:05:00Z"
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
