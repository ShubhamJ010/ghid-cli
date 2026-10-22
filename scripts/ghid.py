#!/usr/bin/env python3
"""
GhostHID CLI (ghid)
Headless Android Bluetooth HID Keyboard Controller & Automation Gateway
"""

from __future__ import annotations

import argparse
import json
import os
import re
import shutil
import signal
import subprocess
import sys
import time
from getpass import getpass
from pathlib import Path
from typing import Any, Dict, List, Optional, Tuple

VERSION = "1.0.0"
PACKAGE_NAME = "com.poc.hiddebug"
SERVICE_NAME = "com.poc.hiddebug/.HidService"
RECEIVER_NAME = "com.poc.hiddebug/.HidCommandReceiver"
ACTION_CMD = "com.poc.hiddebug.ACTION_CMD"
ACTION_LOCK = "com.poc.hiddebug.ACTION_LOCK"
STATUS_FILE_REMOTE = "/sdcard/Android/data/com.poc.hiddebug/files/status.json"
LOG_FILE_REMOTE = "/sdcard/Android/data/com.poc.hiddebug/files/gateway.log"
KEYCHAIN_SERVICE = "ghost-hid"

CONFIG_DIR = Path.home() / ".config" / "ghid"
CONFIG_FILE = CONFIG_DIR / "config.json"
LOCAL_BIN_DIR = Path.home() / ".local" / "bin"
LOCAL_MAN_DIR = Path.home() / ".local" / "share" / "man" / "man1"


# ==============================================================================
# Terminal Colors & Styling
# ==============================================================================

class Style:
    USE_COLOR = sys.stdout.isatty() and os.environ.get("NO_COLOR") is None

    @classmethod
    def _c(cls, code: str, text: str) -> str:
        return f"\033[{code}m{text}\033[0m" if cls.USE_COLOR else text

    @classmethod
    def bold(cls, text: str) -> str: return cls._c("1", text)
    @classmethod
    def dim(cls, text: str) -> str: return cls._c("2", text)
    @classmethod
    def red(cls, text: str) -> str: return cls._c("31", text)
    @classmethod
    def green(cls, text: str) -> str: return cls._c("32", text)
    @classmethod
    def yellow(cls, text: str) -> str: return cls._c("33", text)
    @classmethod
    def blue(cls, text: str) -> str: return cls._c("34", text)
    @classmethod
    def magenta(cls, text: str) -> str: return cls._c("35", text)
    @classmethod
    def cyan(cls, text: str) -> str: return cls._c("36", text)


RAW_BANNER = r"""   ____ _               _   _   _ ___ ____  
  / ___| |__   ___  ___| |_| | | |_ _|  _ \ 
 | |  _| '_ \ / _ \/ __| __| |_| || || | | |
 | |_| | | | | (_) \__ \ |_|  _  || || |_| |
  \____|_| |_|\___/|___/\__|_| |_|___|____/ """


def banner() -> str:
    art = Style.cyan(Style.bold(RAW_BANNER))
    sub = f"  {Style.dim('GhostHID Bluetooth Keyboard Gateway')} {Style.green('v' + VERSION)}"
    return f"{art}\n{sub}\n"


# ==============================================================================
# Custom Exceptions
# ==============================================================================

class GhostHIDError(Exception):
    """Base exception for all GhostHID errors."""
    pass


class AdbNotFoundError(GhostHIDError):
    """Raised when the adb binary cannot be found."""
    pass


class NoDeviceConnectedError(GhostHIDError):
    """Raised when no Android device is attached to ADB."""
    pass


class DeviceUnauthorizedError(GhostHIDError):
    """Raised when an Android device is connected but unauthorized."""
    pass


class DeviceOfflineError(GhostHIDError):
    """Raised when an Android device is detected in offline state."""
    pass


class PackageNotInstalledError(GhostHIDError):
    """Raised when com.poc.hiddebug is not installed on device."""
    pass


class BluetoothDisabledError(GhostHIDError):
    """Raised when Bluetooth is disabled on the phone."""
    pass


class ConnectionTimeoutError(GhostHIDError):
    """Raised when connection to target laptop fails or times out."""
    pass


# ==============================================================================
# Configuration & Keychain Vault
# ==============================================================================

def load_config() -> Dict[str, Any]:
    default_config: Dict[str, Any] = {
        "default_user": None,
        "target_address": "38:FC:98:E8:E3:87",
        "wireless_adb_address": "192.168.1.80:41839",
        "device_serial": None,
        "delays": {
            "wake": 1.0,
            "overlay": 1.0,
            "user": 1.2,
            "tab": 0.8,
            "pass": 1.2,
            "submit": 1.0,
        },
    }
    if not CONFIG_FILE.exists():
        return default_config
    try:
        with open(CONFIG_FILE, "r", encoding="utf-8") as f:
            data = json.load(f)
            default_config.update(data)
            return default_config
    except Exception:
        return default_config


def save_config(config: Dict[str, Any]) -> None:
    try:
        CONFIG_DIR.mkdir(parents=True, exist_ok=True)
        os.chmod(CONFIG_DIR, 0o700)
        with open(CONFIG_FILE, "w", encoding="utf-8") as f:
            json.dump(config, f, indent=2)
        os.chmod(CONFIG_FILE, 0o600)
    except Exception as e:
        print(f"{Style.yellow('Warning: Failed to save config:')} {e}", file=sys.stderr)


class CredentialVault:
    """Secure credential storage leveraging the macOS Keychain with file fallback."""

    @staticmethod
    def is_keychain_available() -> bool:
        return sys.platform == "darwin" and shutil.which("security") is not None

    @classmethod
    def get_password(cls, username: str) -> Optional[str]:
        if not username:
            return None
        if cls.is_keychain_available():
            try:
                cmd = [
                    "/usr/bin/security",
                    "find-generic-password",
                    "-a", username,
                    "-s", KEYCHAIN_SERVICE,
                    "-w",
                ]
                res = subprocess.run(cmd, stdout=subprocess.PIPE, stderr=subprocess.DEVNULL, text=True, check=True)
                return res.stdout.strip()
            except subprocess.CalledProcessError:
                return None
            except Exception:
                return None
        return None

    @classmethod
    def set_password(cls, username: str, password: str) -> bool:
        if not username or password is None:
            return False
        if cls.is_keychain_available():
            try:
                cmd = [
                    "/usr/bin/security",
                    "add-generic-password",
                    "-a", username,
                    "-s", KEYCHAIN_SERVICE,
                    "-w", password,
                    "-U",
                ]
                subprocess.run(cmd, stdout=subprocess.DEVNULL, stderr=subprocess.PIPE, check=True)
                return True
            except subprocess.CalledProcessError as e:
                print(f"{Style.red('Keychain error:')} {e.stderr.decode()}", file=sys.stderr)
                return False
        return False

    @classmethod
    def delete_password(cls, username: str) -> bool:
        if not username:
            return False
        if cls.is_keychain_available():
            try:
                cmd = [
                    "/usr/bin/security",
                    "delete-generic-password",
                    "-a", username,
                    "-s", KEYCHAIN_SERVICE,
                ]
                subprocess.run(cmd, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, check=True)
                return True
            except subprocess.CalledProcessError:
                return False
        return False


# ==============================================================================
# ADB Manager & Device Operations
# ==============================================================================

class AdbManager:
    def __init__(self, serial: Optional[str] = None):
        self.adb_path = self._locate_adb()
        self.serial = serial or os.environ.get("ANDROID_SERIAL")
        self._project_root = Path(__file__).resolve().parent

    def _locate_adb(self) -> str:
        env_adb = os.environ.get("ADB_PATH")
        if env_adb and os.path.isfile(env_adb) and os.access(env_adb, os.X_OK):
            return env_adb

        which_adb = shutil.which("adb")
        if which_adb:
            return which_adb

        common_paths = [
            "/opt/homebrew/bin/adb",
            os.path.expanduser("~/Library/Android/sdk/platform-tools/adb"),
            "/usr/local/bin/adb",
            "/usr/bin/adb",
        ]
        for p in common_paths:
            if os.path.isfile(p) and os.access(p, os.X_OK):
                return p

        raise AdbNotFoundError(
            "Android Debug Bridge ('adb') was not found in PATH or standard directories.\n"
            "Please install Android Platform Tools (e.g. 'brew install android-platform-tools')."
        )

    def run(
        self,
        args: List[str],
        check: bool = True,
        timeout: float = 20.0,
        capture_output: bool = True,
    ) -> subprocess.CompletedProcess:
        cmd = [self.adb_path]
        if self.serial:
            cmd.extend(["-s", self.serial])
        cmd.extend(args)

        try:
            return subprocess.run(
                cmd,
                check=check,
                timeout=timeout,
                capture_output=capture_output,
                text=True,
            )
        except subprocess.TimeoutExpired as e:
            raise GhostHIDError(f"ADB command timed out after {timeout}s: {' '.join(cmd)}") from e
        except subprocess.CalledProcessError as e:
            err = (e.stderr or "").strip() or (e.stdout or "").strip()
            raise GhostHIDError(f"ADB command failed ({e.returncode}): {err}") from e

    def get_devices(self) -> List[Dict[str, str]]:
        cmd = [self.adb_path, "devices", "-l"]
        res = subprocess.run(cmd, capture_output=True, text=True, check=True)
        devices: List[Dict[str, str]] = []

        for line in res.stdout.strip().splitlines()[1:]:
            line = line.strip()
            if not line:
                continue
            parts = line.split()
            serial = parts[0]
            state = parts[1] if len(parts) > 1 else "unknown"

            details = {"serial": serial, "state": state}
            for token in parts[2:]:
                if ":" in token:
                    k, v = token.split(":", 1)
                    details[k] = v
            devices.append(details)

        return devices

    def select_device(self) -> Dict[str, str]:
        devices = self.get_devices()
        if not devices:
            raise NoDeviceConnectedError(
                "No Android device detected via USB or Wireless ADB.\n"
                "Please connect your device via USB or run 'ghid' interactive mode to connect over Wi-Fi."
            )

        if self.serial:
            for d in devices:
                if d["serial"] == self.serial:
                    if d["state"] == "unauthorized":
                        raise DeviceUnauthorizedError(
                            f"Device {self.serial} is unauthorized. Please accept the USB debugging prompt on your phone screen."
                        )
                    if d["state"] == "offline":
                        raise DeviceOfflineError(f"Device {self.serial} is offline.")
                    return d
            raise GhostHIDError(f"Specified device serial '{self.serial}' not found in active devices.")

        # Filter available devices
        ready_devices = [d for d in devices if d["state"] == "device"]
        if ready_devices:
            chosen = ready_devices[0]
            self.serial = chosen["serial"]
            return chosen

        # Handle unauthorized / offline
        first = devices[0]
        if first["state"] == "unauthorized":
            raise DeviceUnauthorizedError(
                f"Device {first['serial']} is unauthorized. Please unlock your phone and accept 'Allow USB debugging'."
            )
        if first["state"] == "offline":
            raise DeviceOfflineError(f"Device {first['serial']} is offline. Try reconnecting.")
        raise GhostHIDError(f"Device {first['serial']} is in unexpected state: {first['state']}")

    def connect_wireless(self, address: str) -> str:
        cmd = [self.adb_path, "connect", address]
        res = subprocess.run(cmd, capture_output=True, text=True, timeout=10.0)
        out = (res.stdout + res.stderr).strip()
        if "connected to" in out.lower():
            self.serial = address
            return out
        raise GhostHIDError(f"Wireless ADB connection failed: {out}")

    def pair_wireless(self, address: str, code: str) -> str:
        cmd = [self.adb_path, "pair", address, code]
        res = subprocess.run(cmd, capture_output=True, text=True, timeout=10.0)
        out = (res.stdout + res.stderr).strip()
        if "successfully paired" in out.lower():
            return out
        raise GhostHIDError(f"Wireless ADB pairing failed: {out}")

    def is_package_installed(self) -> bool:
        res = self.run(["shell", "pm", "list", "packages", PACKAGE_NAME], check=False)
        return f"package:{PACKAGE_NAME}" in (res.stdout or "")

    def install_apk(self, apk_path: Optional[Path] = None) -> bool:
        target_apk = apk_path
        if not target_apk:
            candidate = self._project_root / "build" / "ghost-hid.apk"
            if candidate.exists():
                target_apk = candidate
            else:
                candidate2 = Path.cwd() / "build" / "ghost-hid.apk"
                if candidate2.exists():
                    target_apk = candidate2

        if not target_apk or not target_apk.exists():
            raise GhostHIDError(
                f"GhostHID APK not found. Looked in: {self._project_root / 'build' / 'ghost-hid.apk'}.\n"
                "Please run 'ghid build' or provide the path with '--path'."
            )

        print(f"{Style.cyan('Installing APK onto device:')} {target_apk}")
        self.run(["install", "-r", str(target_apk)], timeout=60.0)
        return True

    def build_apk(self) -> bool:
        build_script = self._project_root / "build.sh"
        if not build_script.exists():
            build_script = Path.cwd() / "build.sh"
        if not build_script.exists():
            raise GhostHIDError(f"Build script not found at {build_script}")

        print(f"{Style.cyan('Executing build script:')} {build_script}")
        res = subprocess.run(["bash", str(build_script)], cwd=build_script.parent)
        if res.returncode != 0:
            raise GhostHIDError("Build failed! Review compiler / toolchain output above.")
        return True

    def get_bluetooth_state(self) -> bool:
        res = self.run(["shell", "settings", "get", "global", "bluetooth_on"], check=False)
        return (res.stdout or "").strip() == "1"

    def enable_bluetooth(self) -> bool:
        self.run(["shell", "cmd", "bluetooth_manager", "enable"], check=False)
        time.sleep(1.0)
        return self.get_bluetooth_state()

    def get_device_model(self) -> str:
        res = self.run(["shell", "getprop", "ro.product.model"], check=False)
        return (res.stdout or "").strip() or "Android Device"

    def exempt_battery_optimization(self) -> None:
        self.run(["shell", "dumpsys", "deviceidle", "whitelist", f"+{PACKAGE_NAME}"], check=False)

    def start_gateway(self) -> None:
        self.exempt_battery_optimization()
        self.run(
            ["shell", "am", "start-foreground-service", "-n", SERVICE_NAME],
            check=False,
        )

    def stop_gateway(self) -> None:
        self.run(
            ["shell", "am", "broadcast", "-n", RECEIVER_NAME, "-a", ACTION_CMD, "--es", "cmd", "stop"],
            check=False,
        )
        self.run(["shell", "am", "stopservice", "-n", SERVICE_NAME], check=False)

    def get_status(self) -> Dict[str, Any]:
        res = self.run(["shell", "cat", STATUS_FILE_REMOTE], check=False)
        text = (res.stdout or "").strip()
        if text.startswith("{") and text.endswith("}"):
            try:
                return json.loads(text)
            except Exception:
                pass
        return {
            "registered": False,
            "connected": False,
            "state": "OFFLINE",
            "target": "none",
            "address": "none",
            "updated": "none",
        }

    def connect_target(self, address: Optional[str] = None) -> None:
        cmd = ["shell", "am", "broadcast", "-n", RECEIVER_NAME, "-a", ACTION_CMD, "--es", "cmd", "connect"]
        if address:
            cmd.extend(["--es", "address", address])
        self.run(cmd, check=False)

    def disconnect_target(self) -> None:
        self.run(
            ["shell", "am", "broadcast", "-n", RECEIVER_NAME, "-a", ACTION_CMD, "--es", "cmd", "disconnect"],
            check=False,
        )

    def unpair_target(self) -> None:
        self.run(
            ["shell", "am", "broadcast", "-n", RECEIVER_NAME, "-a", ACTION_CMD, "--es", "cmd", "unpair"],
            check=False,
        )

    def send_lock(self) -> None:
        self.run(["shell", "am", "broadcast", "-n", RECEIVER_NAME, "-a", ACTION_LOCK], check=False)

    def send_combo(self, keys: str) -> None:
        self.run(
            ["shell", "am", "broadcast", "-n", RECEIVER_NAME, "-a", ACTION_CMD, "--es", "cmd", "combo", "--es", "keys", keys],
            check=False,
        )

    def type_text(self, text: str) -> None:
        self.run(
            ["shell", "am", "broadcast", "-n", RECEIVER_NAME, "-a", ACTION_CMD, "--es", "cmd", "type", "--es", "text", text],
            check=False,
        )

    def send_raw(self, hex_report: str) -> None:
        if len(hex_report) != 16:
            raise GhostHIDError(f"Raw report must be exactly 16 hex characters (8 bytes), got {len(hex_report)}")
        self.run(
            ["shell", "am", "broadcast", "-n", RECEIVER_NAME, "-a", ACTION_CMD, "--es", "cmd", "raw", "--es", "hex", hex_report],
            check=False,
        )

    def send_sleep_sequence(self, delays: Optional[Dict[str, float]] = None) -> None:
        self.send_combo("win+x")
        time.sleep(0.6)
        self.send_combo("u")
        time.sleep(0.4)
        self.send_combo("s")

    def get_logs(self, lines: int = 35) -> str:
        cmd = [self.adb_path]
        if self.serial:
            cmd.extend(["-s", self.serial])
        cmd.extend(["logcat", "-d"])
        proc1 = subprocess.Popen(cmd, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
        proc2 = subprocess.Popen(["grep", "-i", "GHOST_HID"], stdin=proc1.stdout, stdout=subprocess.PIPE, text=True)
        if proc1.stdout:
            proc1.stdout.close()
        proc3 = subprocess.Popen(["tail", "-n", str(lines)], stdin=proc2.stdout, stdout=subprocess.PIPE, text=True)
        if proc2.stdout:
            proc2.stdout.close()
        out, _ = proc3.communicate()
        return out.strip()

    def stream_logs(self) -> None:
        cmd = [self.adb_path]
        if self.serial:
            cmd.extend(["-s", self.serial])
        cmd.extend(["logcat", "-v", "time"])
        proc1 = subprocess.Popen(cmd, stdout=subprocess.PIPE, stderr=subprocess.DEVNULL)
        proc2 = subprocess.Popen(["grep", "--line-buffered", "-i", "GHOST_HID"], stdin=proc1.stdout)
        if proc1.stdout:
            proc1.stdout.close()
        try:
            proc2.wait()
        except KeyboardInterrupt:
            proc1.terminate()
            proc2.terminate()

    def get_filelog(self) -> str:
        res = self.run(["shell", "cat", LOG_FILE_REMOTE], check=False)
        return (res.stdout or "").strip() or "Remote gateway.log empty or missing."


# ==============================================================================
# Login Engine
# ==============================================================================

class LoginEngine:
    @staticmethod
    def execute_login(
        adb: AdbManager,
        username: str,
        password: str,
        delays: Optional[Dict[str, float]] = None,
        verbose: bool = True,
    ) -> None:
        d = {
            "wake": 1.0,
            "overlay": 1.0,
            "user": 1.2,
            "tab": 0.8,
            "pass": 1.2,
            "submit": 1.0,
        }
        if delays:
            d.update(delays)

        def log_step(step: int, msg: str) -> None:
            if verbose:
                print(f"  {Style.cyan(f'[{step}/6]')} {msg}")

        if verbose:
            print(f"\n{Style.bold('=== Executing Windows Auto-Login Sequence ===')}")

        log_step(1, "Waking lock screen (Ctrl + Alt + Delete)...")
        adb.send_combo("ctrl+alt+del")
        time.sleep(d["wake"])

        log_step(2, "Dismissing lock overlay (Escape)...")
        adb.send_combo("esc")
        time.sleep(d["overlay"])

        log_step(3, f"Typing username ({len(username)} chars)...")
        adb.type_text(username)
        time.sleep(d["user"])

        log_step(4, "Navigating to password field (Tab)...")
        adb.send_combo("tab")
        time.sleep(d["tab"])

        log_step(5, f"Injecting password ({len(password)} chars)...")
        adb.type_text(password)
        time.sleep(d["pass"])

        log_step(6, "Submitting authentication credentials (Enter)...")
        adb.send_combo("enter")
        time.sleep(d["submit"])

        if verbose:
            print(f"{Style.green(Style.bold('✔ Login sequence complete! Windows unlocked.'))}\n")


# ==============================================================================
# Interactive Shell & TUI
# ==============================================================================

class InteractiveSession:
    def __init__(self, adb: AdbManager, config: Dict[str, Any]):
        self.adb = adb
        self.config = config

    def start(self) -> None:
        os.system("clear" if sys.platform != "win32" else "cls")
        print(banner())
        print(f"{Style.dim('Initializing GhostHID environment...')}\n")

        # 1. Device detection with recovery
        self._ensure_device_ready()

        # 2. Package verification
        self._ensure_package_installed()

        # 3. Bluetooth state check
        if not self.adb.get_bluetooth_state():
            print(f"{Style.yellow('⚠ Bluetooth is disabled on the phone.')}")
            choice = input("Enable phone Bluetooth now? [Y/n]: ").strip().lower()
            if choice not in ("n", "no"):
                print("Enabling Bluetooth...")
                self.adb.enable_bluetooth()

        # 4. Service start & Registration
        print(f"{Style.cyan('→')} Starting GhostHID Android Foreground Service...")
        self.adb.start_gateway()

        print(f"{Style.cyan('→')} Verifying Bluetooth HID SDP registration...", end="", flush=True)
        registered = False
        for _ in range(10):
            time.sleep(0.5)
            status = self.adb.get_status()
            if status.get("registered"):
                registered = True
                break
            print(".", end="", flush=True)
        print()

        if not registered:
            print(f"{Style.yellow('⚠ Gateway did not confirm SDP registration immediately. Checking status...')}")

        # 5. Connection check & Auto-connect
        status = self.adb.get_status()
        if not status.get("connected"):
            self._handle_initial_connection()

        # 6. Main Interactive Loop
        self.menu_loop()

    def _ensure_device_ready(self) -> None:
        while True:
            try:
                device = self.adb.select_device()
                model = self.adb.get_device_model()
                print(f"{Style.green('✔')} Device connected: {Style.bold(model)} ({device['serial']})")
                return
            except NoDeviceConnectedError:
                print(f"\n{Style.red('✖ No Android device detected.')}")
                print(f"  [1] Connect Wireless ADB ({self.config.get('wireless_adb_address', '192.168.1.80:41839')})")
                print("  [2] Connect custom Wireless ADB address")
                print("  [3] Pair Wireless ADB (pair code)")
                print("  [4] Rescan USB / Wi-Fi devices")
                print("  [0] Exit")
                c = input(f"\n{Style.bold('Select recovery action [1-4, 0]: ')}").strip()
                if c == "1":
                    addr = self.config.get("wireless_adb_address", "192.168.1.80:41839")
                    try:
                        print(f"Connecting to {addr}...")
                        self.adb.connect_wireless(addr)
                    except Exception as e:
                        print(f"{Style.red('Error:')} {e}")
                elif c == "2":
                    addr = input("Enter device IP:PORT: ").strip()
                    if addr:
                        try:
                            self.adb.connect_wireless(addr)
                            self.config["wireless_adb_address"] = addr
                            save_config(self.config)
                        except Exception as e:
                            print(f"{Style.red('Error:')} {e}")
                elif c == "3":
                    addr = input("Enter device IP:PORT for pairing: ").strip()
                    code = input("Enter 6-digit pairing code: ").strip()
                    if addr and code:
                        try:
                            self.adb.pair_wireless(addr, code)
                        except Exception as e:
                            print(f"{Style.red('Error:')} {e}")
                elif c == "4":
                    continue
                else:
                    sys.exit(0)
            except DeviceUnauthorizedError as e:
                print(f"\n{Style.yellow(str(e))}")
                input("Press [Enter] after accepting the authorization prompt on phone...")
            except Exception as e:
                print(f"\n{Style.red('Device error:')} {e}")
                input("Press [Enter] to retry or Ctrl+C to abort...")

    def _ensure_package_installed(self) -> None:
        if self.adb.is_package_installed():
            return

        print(f"\n{Style.yellow(f'⚠ GhostHID APK ({PACKAGE_NAME}) is NOT installed on your phone.')}")
        choice = input("Would you like to install it now? [Y/n]: ").strip().lower()
        if choice in ("n", "no"):
            raise PackageNotInstalledError(f"APK {PACKAGE_NAME} is required to run GhostHID.")

        try:
            self.adb.install_apk()
            print(f"{Style.green('✔ APK installed successfully!')}")
        except GhostHIDError:
            print("Pre-built APK missing. Attempting to build from source...")
            self.adb.build_apk()
            self.adb.install_apk()
            print(f"{Style.green('✔ APK built and installed successfully!')}")

    def _handle_initial_connection(self) -> None:
        print(f"\n{Style.cyan('→')} Connecting to target laptop via Bluetooth HID...")
        target_addr = self.config.get("target_address")
        self.adb.connect_target(target_addr)

        connected = False
        print(f"{Style.dim('Negotiating Bluetooth L2CAP channels...')}", end="", flush=True)
        for _ in range(8):
            time.sleep(0.7)
            status = self.adb.get_status()
            if status.get("connected"):
                connected = True
                break
            print(".", end="", flush=True)
        print()

        if connected:
            print(f"{Style.green('✔ Connected to target:')} {Style.bold(status.get('target', 'Laptop'))} [{status.get('address')}]")
        else:
            print(f"\n{Style.yellow('⚠ Could not establish Bluetooth HID link automatically.')}")
            print(f"{Style.bold('Note:')} Wake-on-Bluetooth is NOT active when laptop is asleep.")
            print("Please ensure your target laptop is awake (tap power button or physical key).")
            print("  [1] Retry connection now")
            print("  [2] Specify different Bluetooth MAC address")
            print("  [3] Continue to interactive menu anyway")
            c = input(f"{Style.bold('Selection [1-3]: ')}").strip()
            if c == "1":
                self._handle_initial_connection()
            elif c == "2":
                new_addr = input("Enter Bluetooth MAC address (e.g. 38:FC:98:E8:E3:87): ").strip()
                if new_addr:
                    self.config["target_address"] = new_addr
                    save_config(self.config)
                    self.adb.connect_target(new_addr)
                    time.sleep(2.0)

    def print_status_box(self) -> None:
        status = self.adb.get_status()
        model = self.adb.get_device_model()
        is_conn = status.get("connected", False)
        is_reg = status.get("registered", False)
        target = status.get("target", "none")
        addr = status.get("address", "none")
        cached_user = self.config.get("default_user")

        conn_badge = Style.green("● CONNECTED") if is_conn else Style.red("○ DISCONNECTED")
        reg_badge = Style.green("READY") if is_reg else Style.yellow("UNREGISTERED")
        user_badge = Style.cyan(cached_user) if cached_user else Style.dim("Not configured")

        box = (
            f"\n{Style.dim('┌────────────────────────────────────────────────────────┐')}\n"
            f"  {Style.bold('Phone Device:')}    {model} ({self.adb.serial})\n"
            f"  {Style.bold('Gateway HID:')}     {reg_badge}\n"
            f"  {Style.bold('Target Laptop:')}   {target} [{addr}] -> {conn_badge}\n"
            f"  {Style.bold('Cached Login:')}    {user_badge}\n"
            f"{Style.dim('└────────────────────────────────────────────────────────┘')}"
        )
        print(box)

    def menu_loop(self) -> None:
        while True:
            self.print_status_box()
            print(f"{Style.bold('Actions:')}")
            print(f"  {Style.green('[1] login')}      - Automated Windows unlock & login")
            print(f"  {Style.cyan('[2] lock')}       - Lock Windows screen (Win + L)")
            print(f"  {Style.yellow('[3] sleep')}      - Windows Sleep sequence + countdown exit")
            print(f"  {Style.cyan('[4] type')}       - Inject text string via keyboard")
            print(f"  {Style.cyan('[5] combo')}      - Send custom key combination (win+d, alt+tab)")
            print(f"  {Style.dim('[6] raw')}        - Inject raw 16-hex byte HID report")
            print(f"  {Style.blue('[7] creds')}      - View / update cached login credentials")
            print(f"  {Style.blue('[8] reconnect')}  - Re-trigger Bluetooth connection")
            print(f"  {Style.dim('[9] logs')}       - View gateway diagnostics logcat")
            print(f"  {Style.red('[0] exit')}       - Disconnect Bluetooth & terminate phone service")

            try:
                choice = input(f"\n{Style.bold('ghid')} > ").strip().lower()
            except (KeyboardInterrupt, EOFError):
                print()
                self.action_exit()
                return

            if not choice:
                continue

            if choice in ("1", "login"):
                self.action_login()
            elif choice in ("2", "lock"):
                self.action_lock()
            elif choice in ("3", "sleep"):
                self.action_sleep()
                return
            elif choice in ("4", "type"):
                self.action_type()
            elif choice in ("5", "combo"):
                self.action_combo()
            elif choice in ("6", "raw"):
                self.action_raw()
            elif choice in ("7", "creds"):
                self.action_creds()
            elif choice in ("8", "reconnect", "connect"):
                self.action_reconnect()
            elif choice in ("9", "logs"):
                print(f"\n{Style.dim(self.adb.get_logs(25))}")
            elif choice in ("0", "exit", "quit", "q"):
                self.action_exit()
                return
            else:
                print(f"{Style.red('Unknown command:')} {choice}")

    def action_login(self) -> None:
        cached_user = self.config.get("default_user")
        user = cached_user
        password = None

        if cached_user:
            password = CredentialVault.get_password(cached_user)

        if user and password:
            print(f"\nFound cached credentials for {Style.bold(user)}.")
            c = input(f"Press [Enter] to log in as '{user}', or 'c' to enter different credentials: ").strip().lower()
            if c == "c":
                user = None
                password = None

        if not user or not password:
            user = input("\nEnter Windows Username: ").strip()
            password = getpass("Enter Windows Password (hidden): ")
            if not user or not password:
                print(f"{Style.red('Login aborted: username or password empty.')}")
                return

            save_c = input("Cache credentials securely in macOS Keychain for future sessions? [Y/n]: ").strip().lower()
            if save_c not in ("n", "no"):
                CredentialVault.set_password(user, password)
                self.config["default_user"] = user
                save_config(self.config)
                print(f"{Style.green('✔ Credentials saved securely in Keychain.')}")

        LoginEngine.execute_login(
            adb=self.adb,
            username=user,
            password=password,
            delays=self.config.get("delays"),
            verbose=True,
        )

    def action_lock(self) -> None:
        print(f"\n{Style.cyan('→')} Sending Win + L screen lock sequence...")
        self.adb.send_lock()
        print(f"{Style.green('✔ Windows locked.')}")

    def action_sleep(self) -> None:
        print(f"\n{Style.yellow(Style.bold('Preparing to put target laptop to sleep...'))}")
        confirm = input("Are you sure? [Y/n]: ").strip().lower()
        if confirm in ("n", "no"):
            print("Sleep canceled.")
            return

        print(f"{Style.cyan('→')} Dispatching Windows Sleep sequence (Win+X -> u -> s)...")
        self.adb.send_sleep_sequence()
        print(f"{Style.green('✔ Windows sleep sequence dispatched.')}")

        print(f"\n{Style.dim('Target laptop is now entering ACPI sleep mode.')}")
        print(f"{Style.yellow('Reminder: Wake-on-Bluetooth is not supported when laptop is asleep.')}")
        print("Bluetooth connection will disconnect now.")

        for i in range(5, 0, -1):
            print(f"\rClosing GhostHID gateway in {Style.bold(str(i))}s...", end="", flush=True)
            time.sleep(1.0)
        print("\n")

        print("Disconnecting Bluetooth and stopping Android background service...")
        self.adb.disconnect_target()
        self.adb.stop_gateway()
        print(f"{Style.green('✔ GhostHID gateway stopped cleanly. Goodbye!')}")

    def action_type(self) -> None:
        text = input("\nEnter text to type on target laptop: ")
        if not text:
            return
        print(f"{Style.cyan('→')} Typing {len(text)} characters...")
        self.adb.type_text(text)
        print(f"{Style.green('✔ Done.')}")

    def action_combo(self) -> None:
        combo = input("\nEnter key combo (e.g. 'win+d', 'ctrl+alt+del', 'esc', 'alt+tab'): ").strip()
        if not combo:
            return
        print(f"{Style.cyan('→')} Sending combo: {combo}...")
        self.adb.send_combo(combo)
        print(f"{Style.green('✔ Done.')}")

    def action_raw(self) -> None:
        raw_hex = input("\nEnter 16 hex characters (8 bytes, e.g. 08000f0000000000 for Win+L): ").strip()
        if not raw_hex:
            return
        try:
            self.adb.send_raw(raw_hex)
            print(f"{Style.green('✔ Raw report sent.')}")
        except GhostHIDError as e:
            print(f"{Style.red('Error:')} {e}")

    def action_creds(self) -> None:
        print(f"\n{Style.bold('--- Credential Vault Management ---')}")
        current_user = self.config.get("default_user")
        has_pass = CredentialVault.get_password(current_user) if current_user else None

        if current_user and has_pass:
            print(f"Current default user: {Style.cyan(current_user)} (Password stored in Keychain)")
        elif current_user:
            print(f"Current default user: {Style.cyan(current_user)} (Password missing from Keychain)")
        else:
            print("No cached credentials currently configured.")

        print("  [1] Set new username and password in Keychain")
        print("  [2] Clear / delete stored credentials")
        print("  [0] Back to main menu")
        c = input("Selection: ").strip()

        if c == "1":
            u = input("Username: ").strip()
            p = getpass("Password: ")
            if u and p:
                CredentialVault.set_password(u, p)
                self.config["default_user"] = u
                save_config(self.config)
                print(f"{Style.green('✔ Stored credentials for ' + u + ' in Keychain.')}")
        elif c == "2":
            if current_user:
                CredentialVault.delete_password(current_user)
            self.config["default_user"] = None
            save_config(self.config)
            print(f"{Style.green('✔ Stored credentials cleared.')}")

    def action_reconnect(self) -> None:
        print(f"\n{Style.cyan('→')} Reconnecting Bluetooth HID...")
        self.adb.connect_target(self.config.get("target_address"))
        time.sleep(2.0)
        status = self.adb.get_status()
        if status.get("connected"):
            print(f"{Style.green('✔ Connected to ' + status.get('target', 'Laptop'))}")
        else:
            print(f"{Style.yellow('⚠ Connection in progress or target unreachable.')}")

    def action_exit(self) -> None:
        print(f"\n{Style.cyan('→')} Disconnecting Bluetooth HID and stopping GhostHID Gateway...")
        try:
            self.adb.disconnect_target()
            self.adb.stop_gateway()
        except Exception:
            pass
        print(f"{Style.green('✔ GhostHID service stopped. Zero background resources active. Goodbye!')}")


# ==============================================================================
# Doctor Diagnostics
# ==============================================================================

def run_doctor(adb: AdbManager, config: Dict[str, Any], as_json: bool = False) -> int:
    report: Dict[str, Any] = {
        "version": VERSION,
        "adb_path": None,
        "adb_available": False,
        "device_connected": False,
        "device_model": None,
        "device_serial": None,
        "apk_installed": False,
        "bluetooth_on": False,
        "gateway_registered": False,
        "gateway_connected": False,
        "target_laptop": None,
        "target_address": None,
        "keychain_available": CredentialVault.is_keychain_available(),
        "cached_user": config.get("default_user"),
        "has_cached_password": False,
        "issues": [],
    }

    try:
        report["adb_path"] = adb.adb_path
        report["adb_available"] = True
    except Exception as e:
        report["issues"].append(f"ADB not found: {e}")

    if report["adb_available"]:
        try:
            device = adb.select_device()
            report["device_connected"] = True
            report["device_serial"] = device["serial"]
            report["device_model"] = adb.get_device_model()
        except Exception as e:
            report["issues"].append(f"ADB device issue: {e}")

    if report["device_connected"]:
        try:
            installed = adb.is_package_installed()
            report["apk_installed"] = installed
            if not installed:
                report["issues"].append(f"Package {PACKAGE_NAME} is not installed.")
        except Exception as e:
            report["issues"].append(f"Package check error: {e}")

        try:
            bt_on = adb.get_bluetooth_state()
            report["bluetooth_on"] = bt_on
            if not bt_on:
                report["issues"].append("Bluetooth is currently turned OFF on phone.")
        except Exception as e:
            report["issues"].append(f"Bluetooth check error: {e}")

        try:
            status = adb.get_status()
            report["gateway_registered"] = status.get("registered", False)
            report["gateway_connected"] = status.get("connected", False)
            report["target_laptop"] = status.get("target")
            report["target_address"] = status.get("address")
            if not report["gateway_connected"]:
                report["issues"].append("Not currently connected to target laptop.")
        except Exception as e:
            report["issues"].append(f"Gateway status error: {e}")

    cached_u = report["cached_user"]
    if cached_u:
        report["has_cached_password"] = CredentialVault.get_password(cached_u) is not None

    if as_json:
        print(json.dumps(report, indent=2))
        return 0 if not report["issues"] else 1

    print(f"\n{Style.bold('=== GhostHID Doctor Health Check ===')}")
    def check_line(ok: bool, title: str, detail: str = "") -> None:
        mark = Style.green("✔") if ok else Style.red("✖")
        extra = f" {Style.dim(detail)}" if detail else ""
        print(f"  {mark} {title}{extra}")

    check_line(report["adb_available"], "ADB Executable", report["adb_path"] or "Missing")
    check_line(
        report["device_connected"],
        "Android Device",
        f"{report['device_model']} [{report['device_serial']}]" if report["device_connected"] else "None",
    )
    check_line(report["apk_installed"], f"GhostHID APK ({PACKAGE_NAME})")
    check_line(report["bluetooth_on"], "Phone Bluetooth State", "Enabled" if report["bluetooth_on"] else "Disabled")
    check_line(report["gateway_registered"], "Bluetooth HID Profile (SDP)", "Registered" if report["gateway_registered"] else "Not registered")
    check_line(
        report["gateway_connected"],
        "Target Laptop Connection",
        f"{report['target_laptop']} [{report['target_address']}]" if report["gateway_connected"] else "Disconnected",
    )
    check_line(report["keychain_available"], "macOS Keychain Vault")
    check_line(
        report["has_cached_password"],
        "Cached Windows Credentials",
        f"User: {report['cached_user']}" if report["has_cached_password"] else "None",
    )

    if report["issues"]:
        print(f"\n{Style.yellow(Style.bold('Diagnostic Issues:'))}")
        for issue in report["issues"]:
            print(f"  - {issue}")
        print()
        return 1
    else:
        print(f"\n{Style.green(Style.bold('All systems operational! GhostHID is ready.'))}\n")
        return 0


# ==============================================================================
# Symlink / Installation Helpers
# ==============================================================================

def install_binary(bin_dir: Optional[Path] = None) -> None:
    dest_dir = bin_dir or LOCAL_BIN_DIR
    dest_dir.mkdir(parents=True, exist_ok=True)
    source_script = Path(__file__).resolve()

    target_symlink = dest_dir / "ghid"
    if target_symlink.is_symlink() or target_symlink.exists():
        target_symlink.unlink()

    target_symlink.symlink_to(source_script)
    source_script.chmod(0o755)

    print(f"{Style.green('✔')} Created symlink: {Style.bold(str(target_symlink))} -> {source_script}")

    # Install man page
    project_root = source_script.parent
    man_source = project_root / "man" / "ghid.1"
    if man_source.exists():
        LOCAL_MAN_DIR.mkdir(parents=True, exist_ok=True)
        target_man = LOCAL_MAN_DIR / "ghid.1"
        if target_man.is_symlink() or target_man.exists():
            target_man.unlink()
        target_man.symlink_to(man_source)
        print(f"{Style.green('✔')} Installed man page: {Style.bold(str(target_man))} -> {man_source}")

    print(f"\nGhostHID CLI is now available as {Style.bold('ghid')} across all terminal sessions!")


def uninstall_binary(bin_dir: Optional[Path] = None) -> None:
    dest_dir = bin_dir or LOCAL_BIN_DIR
    target_symlink = dest_dir / "ghid"
    if target_symlink.is_symlink() or target_symlink.exists():
        target_symlink.unlink()
        print(f"{Style.green('✔')} Removed {target_symlink}")

    target_man = LOCAL_MAN_DIR / "ghid.1"
    if target_man.is_symlink() or target_man.exists():
        target_man.unlink()
        print(f"{Style.green('✔')} Removed {target_man}")


def show_man_page() -> None:
    source_script = Path(__file__).resolve()
    man_path = source_script.parent / "man" / "ghid.1"
    if not man_path.exists():
        print("Man page source not found at man/ghid.1", file=sys.stderr)
        return

    if shutil.which("mandoc"):
        proc = subprocess.Popen(["mandoc", str(man_path)], stdout=subprocess.PIPE)
        subprocess.run(["less", "-R"], stdin=proc.stdout)
        if proc.stdout:
            proc.stdout.close()
    elif shutil.which("man"):
        subprocess.run(["man", str(man_path)])
    else:
        with open(man_path, "r", encoding="utf-8") as f:
            print(f.read())


# ==============================================================================
# Non-Interactive CLI Dispatcher
# ==============================================================================

def build_parser() -> argparse.ArgumentParser:
    common_parser = argparse.ArgumentParser(add_help=False)
    common_parser.add_argument("-s", "--serial", help="Target specific Android device serial or IP:PORT")
    common_parser.add_argument("--json", action="store_true", help="Output machine-readable JSON format")

    parser = argparse.ArgumentParser(
        parents=[common_parser],
        prog="ghid",
        description="GhostHID - Headless Android Bluetooth HID Keyboard Controller & Automation Engine",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog=(
            "Examples:\n"
            "  ghid                     # Start interactive mode\n"
            "  ghid login               # Automated Windows unlock sequence\n"
            "  ghid lock                # Lock Windows immediately (Win + L)\n"
            "  ghid sleep               # Put Windows to sleep and countdown to exit\n"
            "  ghid type 'Hello World'  # Type text through virtual keyboard\n"
            "  ghid combo 'win+d'       # Inject shortcut combo\n"
            "  ghid status --json       # Query machine-readable status for agents\n"
            "  ghid doctor              # Comprehensive hardware & network diagnostic\n"
            "  ghid install-bin         # Symlink ghid into ~/.local/bin\n"
        ),
    )
    parser.add_argument("-v", "--version", action="version", version=f"GhostHID ghid {VERSION}")

    subparsers = parser.add_subparsers(dest="subcommand", metavar="<command>")

    # interactive
    subparsers.add_parser("interactive", parents=[common_parser], aliases=["i"], help="Launch interactive control console (default)")

    # status
    subparsers.add_parser("status", parents=[common_parser], help="Get gateway and Bluetooth HID connection status")

    # start / stop / restart
    subparsers.add_parser("start", parents=[common_parser], help="Start GhostHID Android Foreground Service")
    subparsers.add_parser("stop", parents=[common_parser], help="Stop GhostHID Gateway and release Bluetooth resources")
    subparsers.add_parser("restart", parents=[common_parser], help="Restart GhostHID Foreground Service")

    # connect / disconnect / unpair
    p_conn = subparsers.add_parser("connect", parents=[common_parser], help="Connect Bluetooth HID to target laptop")
    p_conn.add_argument("address", nargs="?", help="Target Bluetooth MAC address (optional)")

    subparsers.add_parser("disconnect", parents=[common_parser], help="Disconnect Bluetooth HID from target laptop")
    subparsers.add_parser("unpair", parents=[common_parser], help="Unpair target laptop Bluetooth bond")

    # login
    p_login = subparsers.add_parser("login", parents=[common_parser], help="Execute automated Windows wake & login sequence")
    p_login.add_argument("-u", "--user", help="Windows Username (overrides cached username)")
    p_login.add_argument("-p", "--pass", dest="password", help="Windows Password (overrides Keychain)")
    p_login.add_argument("--save", action="store_true", help="Save supplied credentials to macOS Keychain")
    p_login.add_argument("-f", "--force", action="store_true", help="Skip any confirmation prompt")

    # lock
    subparsers.add_parser("lock", parents=[common_parser], help="Instantly lock Windows machine (Win + L)")

    # sleep
    p_sleep = subparsers.add_parser("sleep", parents=[common_parser], help="Put Windows machine to sleep (Win+X -> u -> s) and exit")
    p_sleep.add_argument("--countdown", type=int, default=5, help="Countdown seconds before exit (default: 5)")
    p_sleep.add_argument("--no-exit", action="store_true", help="Do not stop gateway service or exit after sleeping")

    # type
    p_type = subparsers.add_parser("type", parents=[common_parser], help="Type arbitrary text string through virtual keyboard")
    p_type.add_argument("text", nargs="+", help="Text to type")

    # combo
    p_combo = subparsers.add_parser("combo", parents=[common_parser], help="Send key combination (e.g. 'win+d', 'ctrl+alt+del')")
    p_combo.add_argument("keys", help="Key combination string")

    # raw
    p_raw = subparsers.add_parser("raw", parents=[common_parser], help="Send raw 8-byte (16 hex chars) HID keyboard report")
    p_raw.add_argument("hex", help="16 hexadecimal characters (e.g. 08000f0000000000)")

    # creds
    p_creds = subparsers.add_parser("creds", parents=[common_parser], help="Manage stored credentials in macOS Keychain")
    p_creds.add_argument("action", choices=["status", "set", "clear"], help="Credential action")
    p_creds.add_argument("-u", "--user", help="Username")
    p_creds.add_argument("-p", "--pass", dest="password", help="Password")

    # doctor
    subparsers.add_parser("doctor", parents=[common_parser], help="Run comprehensive system & connection diagnostics")

    # build & apk
    p_inst = subparsers.add_parser("install-apk", parents=[common_parser], help="Install GhostHID APK onto connected phone")
    p_inst.add_argument("--path", type=Path, help="Path to custom APK file")
    p_inst.add_argument("--build", action="store_true", help="Recompile APK before installing")

    subparsers.add_parser("build", parents=[common_parser], help="Build APK from source using build.sh")

    # logs & diagnostics
    p_logs = subparsers.add_parser("logs", parents=[common_parser], help="View recent GhostHID logs from Logcat")
    p_logs.add_argument("-n", "--lines", type=int, default=35, help="Number of log lines (default: 35)")

    subparsers.add_parser("tail", parents=[common_parser], help="Stream live GhostHID logs in real-time")
    subparsers.add_parser("filelog", parents=[common_parser], help="Read persistent gateway.log file from device")

    # installation
    p_bin = subparsers.add_parser("install-bin", parents=[common_parser], help="Symlink ghid into ~/.local/bin and install man page")
    p_bin.add_argument("--dir", type=Path, help="Target bin directory (default: ~/.local/bin)")

    subparsers.add_parser("uninstall-bin", parents=[common_parser], help="Remove ghid symlink and man page")
    subparsers.add_parser("man", parents=[common_parser], help="Display formatted ghid manual page")

    return parser


def main() -> None:
    signal.signal(signal.SIGINT, lambda sig, frame: sys.exit(0))
    parser = build_parser()
    args = parser.parse_args()

    config = load_config()
    serial = args.serial or config.get("device_serial")

    try:
        adb = AdbManager(serial=serial)
    except AdbNotFoundError as e:
        if args.json:
            print(json.dumps({"error": "adb_not_found", "message": str(e)}))
        else:
            print(f"{Style.red('Error:')} {e}", file=sys.stderr)
        sys.exit(2)

    cmd = args.subcommand

    # If no subcommand supplied: default to interactive mode if TTY, else print help
    if not cmd:
        if sys.stdin.isatty():
            cmd = "interactive"
        else:
            parser.print_help()
            sys.exit(0)

    try:
        if cmd in ("interactive", "i"):
            session = InteractiveSession(adb=adb, config=config)
            session.start()

        elif cmd == "status":
            adb.select_device()
            status = adb.get_status()
            if args.json:
                print(json.dumps(status, indent=2))
            else:
                is_conn = status.get("connected", False)
                status_str = Style.green("CONNECTED") if is_conn else Style.red("DISCONNECTED")
                reg_str = Style.green("YES") if status.get("registered") else Style.yellow("NO")
                print(f"GhostHID Gateway Status:")
                print(f"  SDP Registered: {reg_str}")
                print(f"  Connection:     {status_str}")
                print(f"  Target:         {status.get('target')} [{status.get('address')}]")
                print(f"  Updated:        {status.get('updated')}")
            sys.exit(0 if status.get("connected") else 1)

        elif cmd == "start":
            adb.select_device()
            print("Starting GhostHID Gateway...")
            adb.start_gateway()
            time.sleep(1.5)
            status = adb.get_status()
            if args.json:
                print(json.dumps(status))
            else:
                print(f"Service started. Registered: {status.get('registered')}")

        elif cmd == "stop":
            adb.select_device()
            print("Stopping GhostHID Gateway and releasing Bluetooth resources...")
            adb.stop_gateway()
            print("Done. Gateway stopped.")

        elif cmd == "restart":
            adb.select_device()
            adb.stop_gateway()
            time.sleep(1.0)
            adb.start_gateway()
            time.sleep(1.5)
            print("GhostHID Gateway restarted.")

        elif cmd == "connect":
            adb.select_device()
            target_addr = args.address or config.get("target_address")
            print(f"Connecting to target ({target_addr or 'auto'})...")
            adb.connect_target(target_addr)
            time.sleep(2.0)
            status = adb.get_status()
            if args.json:
                print(json.dumps(status))
            else:
                conn = status.get("connected", False)
                print(f"State: {status.get('state')} (Connected: {conn})")
            sys.exit(0 if status.get("connected") else 1)

        elif cmd == "disconnect":
            adb.select_device()
            print("Disconnecting target...")
            adb.disconnect_target()
            print("Done.")

        elif cmd == "unpair":
            adb.select_device()
            print("Unpairing target laptop...")
            adb.unpair_target()
            print("Done.")

        elif cmd == "login":
            adb.select_device()
            user = args.user or config.get("default_user")
            password = args.password

            if not password and user:
                password = CredentialVault.get_password(user)

            if not user or not password:
                if not sys.stdin.isatty():
                    raise GhostHIDError("Credentials not provided or cached. Supply with --user and --pass.")
                if not user:
                    user = input("Windows Username: ").strip()
                if not password:
                    password = getpass("Windows Password (hidden): ")

            if args.save and user and password:
                CredentialVault.set_password(user, password)
                config["default_user"] = user
                save_config(config)

            LoginEngine.execute_login(
                adb=adb,
                username=user,
                password=password,
                delays=config.get("delays"),
                verbose=not args.json,
            )
            if args.json:
                print(json.dumps({"status": "success", "user": user}))

        elif cmd == "lock":
            adb.select_device()
            adb.send_lock()
            if args.json:
                print(json.dumps({"action": "lock", "status": "sent"}))
            else:
                print("⚡ Win + L lock command sent.")

        elif cmd == "sleep":
            adb.select_device()
            adb.send_sleep_sequence()
            if not args.no_exit:
                if not args.json:
                    print(f"💤 Windows Sleep sequence sent (Win+X -> u -> s).")
                    print(f"Note: Wake-on-Bluetooth is inactive on target laptop during sleep.")
                    print(f"Target is sleeping. Disconnecting and stopping gateway in {args.countdown}s...")
                    for i in range(args.countdown, 0, -1):
                        print(f"\rExiting in {i}...", end="", flush=True)
                        time.sleep(1.0)
                    print()
                adb.disconnect_target()
                adb.stop_gateway()
                if not args.json:
                    print("Service stopped cleanly.")
            if args.json:
                print(json.dumps({"action": "sleep", "status": "sent", "stopped": not args.no_exit}))

        elif cmd == "type":
            adb.select_device()
            text_str = " ".join(args.text)
            adb.type_text(text_str)
            if args.json:
                print(json.dumps({"action": "type", "chars": len(text_str)}))
            else:
                print(f"⌨ Typed {len(text_str)} characters.")

        elif cmd == "combo":
            adb.select_device()
            adb.send_combo(args.keys)
            if args.json:
                print(json.dumps({"action": "combo", "keys": args.keys}))
            else:
                print(f"🎹 Combo '{args.keys}' sent.")

        elif cmd == "raw":
            adb.select_device()
            adb.send_raw(args.hex)
            if args.json:
                print(json.dumps({"action": "raw", "hex": args.hex}))
            else:
                print(f"📦 Raw report '{args.hex}' sent.")

        elif cmd == "creds":
            action = args.action
            if action == "status":
                u = config.get("default_user")
                has_pass = CredentialVault.get_password(u) if u else False
                res = {"default_user": u, "has_keychain_password": bool(has_pass)}
                if args.json:
                    print(json.dumps(res))
                else:
                    print(f"Default user: {u or 'None'}")
                    print(f"Keychain password present: {bool(has_pass)}")
            elif action == "set":
                u = args.user or input("Username: ").strip()
                p = args.password or getpass("Password: ")
                if u and p:
                    CredentialVault.set_password(u, p)
                    config["default_user"] = u
                    save_config(config)
                    print(f"✔ Saved credentials for '{u}' in Keychain.")
            elif action == "clear":
                u = config.get("default_user")
                if u:
                    CredentialVault.delete_password(u)
                config["default_user"] = None
                save_config(config)
                print("✔ Cleared stored credentials.")

        elif cmd == "doctor":
            code = run_doctor(adb=adb, config=config, as_json=args.json)
            sys.exit(code)

        elif cmd == "install-apk":
            adb.select_device()
            if args.build:
                adb.build_apk()
            adb.install_apk(apk_path=args.path)
            print("✔ APK installation completed.")

        elif cmd == "build":
            adb.build_apk()
            print("✔ Build completed.")

        elif cmd == "logs":
            adb.select_device()
            print(adb.get_logs(lines=args.lines))

        elif cmd == "tail":
            adb.select_device()
            print("Streaming logs (Ctrl+C to stop)...")
            adb.stream_logs()

        elif cmd == "filelog":
            adb.select_device()
            print(adb.get_filelog())

        elif cmd == "install-bin":
            install_binary(bin_dir=args.dir)

        elif cmd == "uninstall-bin":
            uninstall_binary()

        elif cmd == "man":
            show_man_page()

    except GhostHIDError as e:
        if args.json:
            print(json.dumps({"error": e.__class__.__name__, "message": str(e)}))
        else:
            print(f"{Style.red('Error:')} {e}", file=sys.stderr)
        sys.exit(1)


if __name__ == "__main__":
    main()
