package adb

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/ghost-hid/ghid/internal/hid"
)

const (
	PackageName           = "com.sj010.ghid"
	ServiceName           = "com.sj010.ghid/.service.HidService"
	ReceiverName          = "com.sj010.ghid/.service.HidCommandReceiver"
	ActionCmd             = "com.sj010.ghid.ACTION_CMD"
	ActionLock            = "com.sj010.ghid.ACTION_LOCK"
	StatusFileRemote      = "/sdcard/Android/data/com.sj010.ghid/files/status.json"
	LogFileRemote         = "/sdcard/Android/data/com.sj010.ghid/files/gateway.log"
	AppMainActivity       = "com.sj010.ghid/.MainActivity"
	AppCombosFileRemote   = "/sdcard/Android/data/com.sj010.ghid/files/combos.json"
	AppPayloadsFileRemote = "/sdcard/Android/data/com.sj010.ghid/files/payloads.json"
)

type Device struct {
	Serial string `json:"serial"`
	State  string `json:"state"`
	Model  string `json:"model,omitempty"`
}

// GatewayStatus mirrors status.json on the Android host. OS fields come from
// the passive fingerprinting engine (ADR-007) and stay empty on older builds.
type GatewayStatus struct {
	Registered      bool   `json:"registered"`
	Connected       bool   `json:"connected"`
	State           string `json:"state"`
	Target          string `json:"target"`
	Address         string `json:"address"`
	OS              string `json:"os"`
	OSConfidence    string `json:"os_confidence,omitempty"`
	DetectionMethod string `json:"detection_method,omitempty"`
	Updated         string `json:"updated"`
}

type Manager struct {
	AdbPath string
	Serial  string
	Root    string
	runner  Runner
}

func NewManager(serial string) (*Manager, error) {
	runner, err := NewExecRunner()
	if err != nil {
		return nil, err
	}

	root, _ := os.Getwd()
	return &Manager{
		AdbPath: runner.AdbPath(),
		Serial:  serial,
		Root:    root,
		runner:  runner,
	}, nil
}

// IsAvailable reports whether this manager is initialized with an active device serial.
func (m *Manager) IsAvailable() bool {
	return m != nil && m.Serial != ""
}

func (m *Manager) Run(args ...string) (string, error) {
	if m.runner == nil {
		r, err := NewExecRunner()
		if err != nil {
			return "", err
		}
		m.runner = r
		m.AdbPath = r.AdbPath()
	}
	return m.runner.Run(m.Serial, args...)
}

func (m *Manager) GetDevices() ([]Device, error) {
	if m.runner == nil {
		r, err := NewExecRunner()
		if err != nil {
			return nil, err
		}
		m.runner = r
		m.AdbPath = r.AdbPath()
	}

	out, err := m.runner.Run("", "devices", "-l")
	if err != nil {
		return nil, err
	}

	var devices []Device
	lines := strings.Split(out, "\n")
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

// RediscoverDevice re-runs ADB discovery and adopts the first attached device.
// It clears any serial left over from a previous link, so the interactive UI can
// recover when the phone reconnects on a new port or transport.
func (m *Manager) RediscoverDevice() (*Device, error) {
	m.Serial = ""
	return m.SelectDevice()
}

func (m *Manager) ConnectWireless(addr string) error {
	out, err := m.runner.Run("", "connect", addr)
	s := strings.ToLower(out)
	if strings.Contains(s, "connected to") {
		m.Serial = addr
		return nil
	}
	if err != nil {
		return fmt.Errorf("%s (%w)", strings.TrimSpace(out), err)
	}
	return fmt.Errorf("%s", strings.TrimSpace(out))
}

func (m *Manager) PairWireless(addr, code string) error {
	out, err := m.runner.Run("", "pair", addr, code)
	s := strings.ToLower(out)
	if strings.Contains(s, "successfully paired") {
		return nil
	}
	if err != nil {
		return fmt.Errorf("%s (%w)", strings.TrimSpace(out), err)
	}
	return fmt.Errorf("%s", strings.TrimSpace(out))
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
	if err != nil {
		return err
	}
	// Force-stop to prevent Android Package Manager from retaining stale process memory
	_, _ = m.Run("shell", "am", "force-stop", PackageName)
	return nil
}

func (m *Manager) BuildApk() error {
	buildScript := filepath.Join(m.Root, "build.sh")
	if _, err := os.Stat(buildScript); err != nil {
		buildScript = "build.sh"
	}

	if m.runner == nil {
		r, err := NewExecRunner()
		if err != nil {
			return err
		}
		m.runner = r
	}
	return m.runner.RunCommand(filepath.Dir(buildScript), "bash", buildScript)
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

// GetDeviceName retrieves the dynamic, user-facing Bluetooth or device name of the connected phone.
func (m *Manager) GetDeviceName() string {
	cmd := `for k in "secure bluetooth_name" "global bluetooth_name" "global device_name" "system bluetooth_name"; do v=$(settings get $k 2>/dev/null); if [ -n "$v" ] && [ "$v" != "null" ]; then echo "$v"; exit 0; fi; done; m=$(getprop ro.product.marketname); if [ -n "$m" ]; then echo "$m"; exit 0; fi; getprop ro.product.model`
	out, err := m.Run("shell", cmd)
	if err == nil {
		name := strings.TrimSpace(out)
		if name != "" && name != "null" {
			return name
		}
	}

	model := m.GetDeviceModel()
	if model != "" && model != "Android Device" {
		return model
	}

	return "Android Device"
}

type BondedDevice struct {
	Address string `json:"address"`
	Name    string `json:"name"`
	Type    string `json:"type"`
}

var (
	macRegex  = regexp.MustCompile(`(?i)\b([0-9A-Fa-f]{2}[:-]){5}([0-9A-Fa-f]{2})\b`)
	typeRegex = regexp.MustCompile(`\[\s*([^\]]+)\s*\]`)
)

// GetBondedBluetoothDevices parses bonded/paired devices from dumpsys bluetooth_manager.
func (m *Manager) GetBondedBluetoothDevices() ([]BondedDevice, error) {
	out, err := m.Run("shell", "dumpsys", "bluetooth_manager")
	if err != nil {
		return nil, err
	}
	return parseBondedDevices(out), nil
}

func parseBondedDevices(out string) []BondedDevice {
	var devices []BondedDevice
	lines := strings.Split(out, "\n")
	inBonded := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "Bonded devices:") {
			inBonded = true
			continue
		}
		if inBonded {
			if trimmed == "" || strings.HasPrefix(trimmed, "AdapterState:") || strings.HasPrefix(trimmed, "mSnoopLogSetting") || strings.HasPrefix(trimmed, "Bond Events:") {
				break
			}
			macMatch := macRegex.FindString(trimmed)
			if macMatch == "" {
				continue
			}
			devType := "BR/EDR"
			if tMatch := typeRegex.FindStringSubmatch(trimmed); len(tMatch) > 1 {
				devType = strings.TrimSpace(tMatch[1])
			}
			name := ""
			if idx := strings.Index(trimmed, "]"); idx != -1 && idx+1 < len(trimmed) {
				name = strings.TrimSpace(trimmed[idx+1:])
			} else {
				name = strings.TrimSpace(strings.Replace(trimmed, macMatch, "", 1))
			}
			if name == "" {
				name = "Bluetooth Device"
			}
			devices = append(devices, BondedDevice{
				Address: strings.ToUpper(macMatch),
				Name:    name,
				Type:    devType,
			})
		}
	}
	return devices
}

func (m *Manager) StartGateway() error {
	_, _ = m.Run("shell", "dumpsys", "deviceidle", "whitelist", "+"+PackageName)
	_, err := m.Run("shell", "am", "start-foreground-service", "-n", ServiceName)
	return err
}

func (m *Manager) StopGateway() error {
	_, _ = m.Run("shell", "dumpsys", "deviceidle", "whitelist", "+"+PackageName)
	_, _ = m.Run("shell", "am", "broadcast", "-n", ReceiverName, "-a", ActionCmd, "--es", "cmd", "stop")
	_, err := m.Run("shell", "am", "stopservice", "-n", ServiceName)
	return err
}

func (m *Manager) RestartAppAndReconnect() error {
	_, _ = m.Run("shell", "am", "force-stop", PackageName)
	_, _ = m.Run("shell", "am", "start", "-n", AppMainActivity)
	time.Sleep(1500 * time.Millisecond)
	_, err := m.Run("shell", "am", "broadcast", "-a", ActionCmd, "-n", ReceiverName, "--es", "cmd", "connect")
	return err
}

func (m *Manager) GetStatus() GatewayStatus {
	fallback := GatewayStatus{
		Registered: false,
		Connected:  false,
		State:      "OFFLINE",
		Target:     "none",
		Address:    "none",
		OS:         "unknown",
		Updated:    "none",
	}

	out, err := m.Run("shell", "cat", StatusFileRemote)
	if err != nil || !strings.HasPrefix(strings.TrimSpace(out), "{") {
		return fallback
	}

	out = strings.TrimSpace(out)
	if strings.HasPrefix(out, "{") && strings.HasSuffix(out, "}") {
		var status GatewayStatus
		if err := json.Unmarshal([]byte(out), &status); err == nil {
			if strings.TrimSpace(status.OS) == "" {
				status.OS = "unknown"
			}
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

// SendCombo executes a sequence of key chords, mouse actions, and typed strings.
func (m *Manager) SendCombo(sequence string) error {
	return m.SendComboWithResolver(sequence, nil)
}

// SendComboWithResolver executes a combo sequence, resolving any {PAYLOAD}
// references using the supplied resolver function.
func (m *Manager) SendComboWithResolver(sequence string, resolver hid.PayloadResolver) error {
	steps, err := hid.ParseSequence(sequence)
	if err != nil {
		return err
	}
	for _, step := range steps {
		if step.Payload != "" {
			if resolver == nil {
				return fmt.Errorf("payload {%s} requires a resolver", step.Payload)
			}
			val, isRaw, err := resolver(step.Payload)
			if err != nil {
				return err
			}
			if strings.TrimSpace(val) == "" {
				return fmt.Errorf("payload {%s} is empty", step.Payload)
			}
			cleanVal := strings.TrimSpace(val)
			isExplicitRaw := strings.HasPrefix(strings.ToLower(cleanVal), "raw:")
			if isRaw || isExplicitRaw {
				rawHex := cleanVal
				if isExplicitRaw {
					rawHex = strings.TrimSpace(cleanVal[4:])
				}
				rawHex = strings.ReplaceAll(rawHex, " ", "")
				if _, rawErr := hid.ValidateRaw(rawHex); rawErr != nil {
					return fmt.Errorf("invalid raw hex in payload {%s}: %w", step.Payload, rawErr)
				}
				if err := m.SendRawWithHold(rawHex, step.Hold); err != nil {
					return err
				}
				if step.Hold > 0 {
					time.Sleep(step.Hold)
				}
			} else {
				if err := m.TypeText(val); err != nil {
					return err
				}
			}
		} else if step.Raw != "" {
			if err := m.SendRawWithHold(step.Raw, step.Hold); err != nil {
				return err
			}
			if step.Hold > 0 {
				time.Sleep(step.Hold)
			}
		} else if step.Text != "" {
			if err := m.TypeText(step.Text); err != nil {
				return err
			}
		} else if step.Mouse != nil {
			if err := m.sendMouseAction(*step.Mouse); err != nil {
				return err
			}
		} else if step.Modifier != 0 || step.KeyCode != 0 || step.Combo != "" {
			if step.Delay > 0 && step.Modifier == 0 && step.KeyCode == 0 {
				// Pure delay step
			} else {
				clean := hid.NormalizeComboForAndroid(step.Combo)
				args := []string{"shell", "am", "broadcast", "-n", ReceiverName, "-a", ActionCmd, "--es", "cmd", "combo", "--es", "keys", clean}
				if step.Hold > 0 {
					args = append(args, "--el", "hold_ms", strconv.FormatInt(step.Hold.Milliseconds(), 10))
				}
				if _, err := m.Run(args...); err != nil {
					return err
				}
				if step.Hold > 0 {
					time.Sleep(step.Hold)
				}
			}
		}
		if step.Delay > 0 {
			time.Sleep(step.Delay)
		}
	}
	return nil
}

// SendMouse dispatches a single mouse gesture through the phone.
func (m *Manager) SendMouse(action string) error {
	parsed, ok, err := hid.ParseMouseAction(action)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("unknown mouse action %q (use %s)", action, strings.Join(hid.MouseActions(), ", "))
	}
	return m.sendMouseAction(parsed)
}

func (m *Manager) sendMouseAction(action hid.MouseAction) error {
	_, err := m.Run("shell", "am", "broadcast", "-n", ReceiverName, "-a", ActionCmd, "--es", "cmd", "mouse", "--es", "action", action.Name)
	return err
}

func (m *Manager) TypeText(text string) error {
	_, err := m.Run("shell", "am", "broadcast", "-n", ReceiverName, "-a", ActionCmd, "--es", "cmd", "type", "--es", "text", text)
	return err
}

func (m *Manager) SendRaw(hexReport string) error {
	return m.SendRawWithHold(hexReport, 0)
}

func (m *Manager) SendRawWithHold(hexReport string, hold time.Duration) error {
	cleanHex := strings.ReplaceAll(hexReport, " ", "")
	if _, err := hid.ValidateRaw(cleanHex); err != nil {
		return err
	}
	args := []string{"shell", "am", "broadcast", "-n", ReceiverName, "-a", ActionCmd, "--es", "cmd", "raw", "--es", "hex", cleanHex}
	if hold > 0 {
		args = append(args, "--el", "hold_ms", strconv.FormatInt(hold.Milliseconds(), 10))
	}
	_, err := m.Run(args...)
	return err
}

func (m *Manager) SendInstantSleep() error {
	_, err := m.Run("shell", "am", "broadcast", "-n", ReceiverName, "-a", ActionCmd, "--es", "cmd", "sleep")
	return err
}

func (m *Manager) SendMediaKey(key string) error {
	_, err := m.Run("shell", "am", "broadcast", "-n", ReceiverName, "-a", ActionCmd, "--es", "cmd", "media", "--es", "key", key)
	return err
}

func (m *Manager) SendSleepSequence() error {
	return m.SendInstantSleep()
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
	_ = m.SendCombo("{ctrl+alt+del}")
	time.Sleep(getDelay("wake", 1.0))

	if stepCb != nil {
		stepCb(2, "Dismissing lock overlay (Escape)...")
	}
	_ = m.SendCombo("{esc}")
	time.Sleep(getDelay("overlay", 1.0))

	if stepCb != nil {
		stepCb(3, fmt.Sprintf("Typing username (%d chars)...", len(user)))
	}
	_ = m.TypeText(user)
	time.Sleep(getDelay("user", 1.2))

	if stepCb != nil {
		stepCb(4, "Navigating to password field (Tab)...")
	}
	_ = m.SendCombo("{tab}")
	time.Sleep(getDelay("tab", 0.8))

	if stepCb != nil {
		stepCb(5, fmt.Sprintf("Injecting password (%d chars)...", len(pass)))
	}
	_ = m.TypeText(pass)
	time.Sleep(getDelay("pass", 1.2))

	if stepCb != nil {
		stepCb(6, "Submitting authentication credentials (Enter)...")
	}
	_ = m.SendCombo("{enter}")
	time.Sleep(getDelay("submit", 1.0))

	return nil
}
