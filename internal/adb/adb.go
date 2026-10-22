package adb

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	PackageName      = "com.poc.hiddebug"
	ServiceName      = "com.poc.hiddebug/.HidService"
	ReceiverName     = "com.poc.hiddebug/.HidCommandReceiver"
	ActionCmd        = "com.poc.hiddebug.ACTION_CMD"
	ActionLock       = "com.poc.hiddebug.ACTION_LOCK"
	StatusFileRemote = "/sdcard/Android/data/com.poc.hiddebug/files/status.json"
	LogFileRemote    = "/sdcard/Android/data/com.poc.hiddebug/files/gateway.log"
)

type Device struct {
	Serial string
	State  string
	Model  string
}

type GatewayStatus struct {
	Registered bool   `json:"registered"`
	Connected  bool   `json:"connected"`
	State      string `json:"state"`
	Target     string `json:"target"`
	Address    string `json:"address"`
	Updated    string `json:"updated"`
}

type Manager struct {
	AdbPath string
	Serial  string
	Root    string
}

func NewManager(serial string) (*Manager, error) {
	adbPath, err := LocateAdb()
	if err != nil {
		return nil, err
	}

	root, _ := os.Getwd()
	return &Manager{
		AdbPath: adbPath,
		Serial:  serial,
		Root:    root,
	}, nil
}

func LocateAdb() (string, error) {
	if env := os.Getenv("ADB_PATH"); env != "" {
		if fi, err := os.Stat(env); err == nil && !fi.IsDir() {
			return env, nil
		}
	}

	if path, err := exec.LookPath("adb"); err == nil {
		return path, nil
	}

	home, _ := os.UserHomeDir()
	commonPaths := []string{
		"/opt/homebrew/bin/adb",
		filepath.Join(home, "Library/Android/sdk/platform-tools/adb"),
		"/usr/local/bin/adb",
		"/usr/bin/adb",
	}

	for _, p := range commonPaths {
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			return p, nil
		}
	}

	return "", errors.New("Android Debug Bridge ('adb') was not found in PATH or standard directories")
}

func (m *Manager) Run(args ...string) (string, error) {
	var fullArgs []string
	if m.Serial != "" {
		fullArgs = append(fullArgs, "-s", m.Serial)
	}
	fullArgs = append(fullArgs, args...)

	cmd := exec.Command(m.AdbPath, fullArgs...)
	out, err := cmd.CombinedOutput()
	outputStr := strings.TrimSpace(string(out))
	if err != nil {
		return outputStr, fmt.Errorf("adb %s failed: %w (output: %s)", strings.Join(args, " "), err, outputStr)
	}
	return outputStr, nil
}

func (m *Manager) GetDevices() ([]Device, error) {
	cmd := exec.Command(m.AdbPath, "devices", "-l")
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var devices []Device
	lines := strings.Split(string(out), "\n")
	if len(lines) <= 1 {
		return devices, nil
	}

	for _, line := range lines[1:] {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		dev := Device{
			Serial: fields[0],
			State:  fields[1],
		}
		for _, f := range fields[2:] {
			if strings.HasPrefix(f, "model:") {
				dev.Model = strings.TrimPrefix(f, "model:")
			}
		}
		devices = append(devices, dev)
	}
	return devices, nil
}

func (m *Manager) SelectDevice() (*Device, error) {
	devices, err := m.GetDevices()
	if err != nil {
		return nil, err
	}
	if len(devices) == 0 {
		return nil, errors.New("no Android device detected via USB or Wireless ADB")
	}

	if m.Serial != "" {
		for _, d := range devices {
			if d.Serial == m.Serial {
				if d.State == "unauthorized" {
					return &d, errors.New("device is unauthorized. Please accept the USB debugging prompt on your phone screen")
				}
				if d.State == "offline" {
					return &d, errors.New("device is offline")
				}
				return &d, nil
			}
		}
		return nil, fmt.Errorf("device serial '%s' not found", m.Serial)
	}

	for _, d := range devices {
		if d.State == "device" {
			m.Serial = d.Serial
			return &d, nil
		}
	}

	first := devices[0]
	if first.State == "unauthorized" {
		return &first, errors.New("device is unauthorized. Please unlock phone and accept 'Allow USB debugging'")
	}
	return &first, fmt.Errorf("device in state '%s'", first.State)
}

func (m *Manager) ConnectWireless(addr string) error {
	cmd := exec.Command(m.AdbPath, "connect", addr)
	out, err := cmd.CombinedOutput()
	s := strings.ToLower(string(out))
	if strings.Contains(s, "connected to") {
		m.Serial = addr
		return nil
	}
	if err != nil {
		return fmt.Errorf("%s (%w)", strings.TrimSpace(string(out)), err)
	}
	return fmt.Errorf("%s", strings.TrimSpace(string(out)))
}

func (m *Manager) PairWireless(addr, code string) error {
	cmd := exec.Command(m.AdbPath, "pair", addr, code)
	out, err := cmd.CombinedOutput()
	s := strings.ToLower(string(out))
	if strings.Contains(s, "successfully paired") {
		return nil
	}
	if err != nil {
		return fmt.Errorf("%s (%w)", strings.TrimSpace(string(out)), err)
	}
	return fmt.Errorf("%s", strings.TrimSpace(string(out)))
}

func (m *Manager) IsPackageInstalled() bool {
	out, err := m.Run("shell", "pm", "list", "packages", PackageName)
	if err != nil {
		return false
	}
	return strings.Contains(out, "package:"+PackageName)
}

func (m *Manager) InstallApk(customPath string) error {
	apkPath := customPath
	if apkPath == "" {
		candidates := []string{
			filepath.Join(m.Root, "build", "ghost-hid.apk"),
			"build/ghost-hid.apk",
		}
		for _, c := range candidates {
			if _, err := os.Stat(c); err == nil {
				apkPath = c
				break
			}
		}
	}
	if apkPath == "" {
		return errors.New("ghost-hid.apk not found. Build it with 'ghid build' first")
	}

	_, err := m.Run("install", "-r", apkPath)
	return err
}

func (m *Manager) BuildApk() error {
	buildScript := filepath.Join(m.Root, "build.sh")
	if _, err := os.Stat(buildScript); err != nil {
		buildScript = "build.sh"
	}

	cmd := exec.Command("bash", buildScript)
	cmd.Dir = filepath.Dir(buildScript)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (m *Manager) GetBluetoothState() bool {
	out, err := m.Run("shell", "settings", "get", "global", "bluetooth_on")
	if err != nil {
		return false
	}
	return strings.TrimSpace(out) == "1"
}

func (m *Manager) EnableBluetooth() error {
	_, err := m.Run("shell", "cmd", "bluetooth_manager", "enable")
	time.Sleep(1 * time.Second)
	return err
}

func (m *Manager) GetDeviceModel() string {
	out, err := m.Run("shell", "getprop", "ro.product.model")
	if err != nil || strings.TrimSpace(out) == "" {
		return "Android Device"
	}
	return strings.TrimSpace(out)
}

func (m *Manager) StartGateway() error {
	_, _ = m.Run("shell", "dumpsys", "deviceidle", "whitelist", "+"+PackageName)
	_, err := m.Run("shell", "am", "start-foreground-service", "-n", ServiceName)
	return err
}

func (m *Manager) StopGateway() error {
	_, _ = m.Run("shell", "am", "broadcast", "-n", ReceiverName, "-a", ActionCmd, "--es", "cmd", "stop")
	_, err := m.Run("shell", "am", "stopservice", "-n", ServiceName)
	return err
}

func (m *Manager) GetStatus() GatewayStatus {
	fallback := GatewayStatus{
		Registered: false,
		Connected:  false,
		State:      "OFFLINE",
		Target:     "none",
		Address:    "none",
		Updated:    "none",
	}

	out, err := m.Run("shell", "cat", StatusFileRemote)
	if err != nil {
		return fallback
	}

	out = strings.TrimSpace(out)
	if strings.HasPrefix(out, "{") && strings.HasSuffix(out, "}") {
		var status GatewayStatus
		if err := json.Unmarshal([]byte(out), &status); err == nil {
			return status
		}
	}
	return fallback
}

func (m *Manager) ConnectTarget(address string) error {
	args := []string{"shell", "am", "broadcast", "-n", ReceiverName, "-a", ActionCmd, "--es", "cmd", "connect"}
	if address != "" {
		args = append(args, "--es", "address", address)
	}
	_, err := m.Run(args...)
	return err
}

func (m *Manager) DisconnectTarget() error {
	_, err := m.Run("shell", "am", "broadcast", "-n", ReceiverName, "-a", ActionCmd, "--es", "cmd", "disconnect")
	return err
}

func (m *Manager) UnpairTarget() error {
	_, err := m.Run("shell", "am", "broadcast", "-n", ReceiverName, "-a", ActionCmd, "--es", "cmd", "unpair")
	return err
}

func (m *Manager) SendLock() error {
	_, err := m.Run("shell", "am", "broadcast", "-n", ReceiverName, "-a", ActionLock)
	return err
}

func (m *Manager) SendCombo(keys string) error {
	_, err := m.Run("shell", "am", "broadcast", "-n", ReceiverName, "-a", ActionCmd, "--es", "cmd", "combo", "--es", "keys", keys)
	return err
}

func (m *Manager) TypeText(text string) error {
	_, err := m.Run("shell", "am", "broadcast", "-n", ReceiverName, "-a", ActionCmd, "--es", "cmd", "type", "--es", "text", text)
	return err
}

func (m *Manager) SendRaw(hexReport string) error {
	if len(hexReport) != 16 {
		return fmt.Errorf("raw report must be exactly 16 hex characters (8 bytes), got %d", len(hexReport))
	}
	_, err := m.Run("shell", "am", "broadcast", "-n", ReceiverName, "-a", ActionCmd, "--es", "cmd", "raw", "--es", "hex", hexReport)
	return err
}

func (m *Manager) SendSleepSequence() error {
	_ = m.SendCombo("win+x")
	time.Sleep(600 * time.Millisecond)
	_ = m.SendCombo("u")
	time.Sleep(400 * time.Millisecond)
	return m.SendCombo("s")
}

func (m *Manager) GetLogs(lines int) (string, error) {
	out, err := m.Run("logcat", "-d")
	if err != nil {
		return "", err
	}
	allLines := strings.Split(out, "\n")
	var matched []string
	for _, l := range allLines {
		if strings.Contains(strings.ToUpper(l), "GHOST_HID") {
			matched = append(matched, l)
		}
	}
	if len(matched) > lines {
		matched = matched[len(matched)-lines:]
	}
	return strings.Join(matched, "\n"), nil
}

func (m *Manager) GetFileLog() (string, error) {
	out, err := m.Run("shell", "cat", LogFileRemote)
	if err != nil || strings.TrimSpace(out) == "" {
		return "Remote gateway.log empty or missing.", nil
	}
	return strings.TrimSpace(out), nil
}

func (m *Manager) ExecuteLogin(user, pass string, delays map[string]float64, stepCb func(int, string)) error {
	getDelay := func(key string, def float64) time.Duration {
		if v, ok := delays[key]; ok && v > 0 {
			return time.Duration(v * float64(time.Second))
		}
		return time.Duration(def * float64(time.Second))
	}

	if stepCb != nil {
		stepCb(1, "Waking lock screen (Ctrl + Alt + Delete)...")
	}
	_ = m.SendCombo("ctrl+alt+del")
	time.Sleep(getDelay("wake", 1.0))

	if stepCb != nil {
		stepCb(2, "Dismissing lock overlay (Escape)...")
	}
	_ = m.SendCombo("esc")
	time.Sleep(getDelay("overlay", 1.0))

	if stepCb != nil {
		stepCb(3, fmt.Sprintf("Typing username (%d chars)...", len(user)))
	}
	_ = m.TypeText(user)
	time.Sleep(getDelay("user", 1.2))

	if stepCb != nil {
		stepCb(4, "Navigating to password field (Tab)...")
	}
	_ = m.SendCombo("tab")
	time.Sleep(getDelay("tab", 0.8))

	if stepCb != nil {
		stepCb(5, fmt.Sprintf("Injecting password (%d chars)...", len(pass)))
	}
	_ = m.TypeText(pass)
	time.Sleep(getDelay("pass", 1.2))

	if stepCb != nil {
		stepCb(6, "Submitting authentication credentials (Enter)...")
	}
	_ = m.SendCombo("enter")
	time.Sleep(getDelay("submit", 1.0))

	return nil
}
