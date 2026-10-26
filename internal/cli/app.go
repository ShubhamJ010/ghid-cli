package cli

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ghost-hid/ghid/internal/adb"
	"github.com/ghost-hid/ghid/internal/combos"
	"github.com/ghost-hid/ghid/internal/config"
	"github.com/ghost-hid/ghid/internal/gateway"
	"github.com/ghost-hid/ghid/internal/hid"
	"github.com/ghost-hid/ghid/internal/payloads"
	"github.com/ghost-hid/ghid/internal/syncer"
	"github.com/ghost-hid/ghid/internal/tui"
	"github.com/ghost-hid/ghid/internal/vault"
	"github.com/mattn/go-runewidth"
)

// App manages command line execution and dependency lifecycle.
type App struct {
	Version        string
	ConfigStore    *config.Store
	ComboStore     *combos.Store
	PayloadStore   *payloads.Store
	Vault          vault.Vault
	GatewayFactory func(serial string) (gateway.Controller, error)
	InteractiveFn  func(mgr *adb.Manager, cfg config.Config) error
	ComboStudioFn  func(store *combos.Store, send tui.ComboSendFunc) error
	Stdout         io.Writer
	Stderr         io.Writer
}

// NewApp creates a default production App.
func NewApp(version string) *App {
	store := config.NewStore("")
	vlt := vault.NewKeychainVault("")
	pStore := payloads.NewStore("", vlt)
	cStore := combos.NewStore("", pStore)

	return &App{
		Version:      version,
		ConfigStore:  store,
		ComboStore:   cStore,
		PayloadStore: pStore,
		Vault:        vlt,
		GatewayFactory: func(serial string) (gateway.Controller, error) {
			mgr, err := adb.NewManager(serial)
			if err != nil {
				return nil, err
			}
			_, err = mgr.SelectDevice()
			if err != nil {
				return nil, err
			}
			return gateway.New(mgr), nil
		},
		InteractiveFn: func(mgr *adb.Manager, cfg config.Config) error {
			model := tui.NewModelWithStores(mgr, cfg, cStore, pStore)
			p := tea.NewProgram(model, tea.WithAltScreen())
			_, err := p.Run()
			return err
		},
		ComboStudioFn: func(store *combos.Store, send tui.ComboSendFunc) error {
			return tui.RunComboStudio(store, send)
		},
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	}
}

// Run parses arguments and executes the appropriate command, returning an exit code.
func (a *App) Run(args []string) int {
	cfg := a.ConfigStore.Load()

	if len(args) == 0 {
		return a.runInteractive(cfg, "")
	}

	var jsonOutput bool
	var serial string
	var filteredArgs []string
	subcommandSeen := false

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--json":
			jsonOutput = true
		case arg == "--serial":
			if i+1 < len(args) {
				serial = args[i+1]
				i++
			}
		case strings.HasPrefix(arg, "--serial="):
			serial = strings.TrimPrefix(arg, "--serial=")
		case arg == "-s" && !subcommandSeen:
			// Serial short flag is only global before the subcommand. After it,
			// -s belongs to the subcommand (payload set uses it for --secure).
			if i+1 < len(args) {
				serial = args[i+1]
				i++
			}
		default:
			filteredArgs = append(filteredArgs, arg)
			subcommandSeen = true
		}
	}

	// Android is Single Source of Truth: if connected, auto-pull latest phone state on start
	pulled := false
	if mgr, err := adb.NewManager(serial); err == nil {
		pulled = syncer.AutoPullIfConnected(mgr, a.ComboStore, a.PayloadStore)
	}
	if !pulled {
		if a.ComboStore != nil {
			_, _ = a.ComboStore.EnsureDefaults()
		}
		if a.PayloadStore != nil {
			_, _ = a.PayloadStore.EnsureDefaults()
		}
	}

	if len(filteredArgs) == 0 {
		return a.runInteractive(cfg, serial)
	}

	cmd := filteredArgs[0]
	cmdArgs := filteredArgs[1:]

	switch cmd {
	case "interactive", "i":
		return a.runInteractive(cfg, serial)

	case "version", "-v", "--version":
		fmt.Fprintf(a.Stdout, "GhostHID ghid %s (Go/Charm TUI)\n", a.Version)
		return 0

	case "help", "-h", "--help":
		a.printHelp()
		return 0

	case "status":
		return a.handleStatus(cfg, serial, jsonOutput)

	case "doctor":
		return a.handleDoctor(cfg, serial, jsonOutput)

	case "login":
		return a.handleLogin(cfg, serial, cmdArgs, jsonOutput)

	case "lock":
		return a.handleLock(serial, jsonOutput)

	case "sleep":
		return a.handleSleep(serial, cmdArgs, jsonOutput)

	case "combo":
		return a.handleCombo(cfg, serial, cmdArgs, jsonOutput)

	case "quick", "q":
		return a.handleQuick(serial, cmdArgs, jsonOutput)

	case "payload", "payloads":
		return a.handlePayloads(serial, cmdArgs, jsonOutput)

	case "devices", "device":
		return a.handleDevices(cfg, serial, cmdArgs, jsonOutput)

	case "start":
		return a.handleService("start", serial, jsonOutput)

	case "stop":
		return a.handleService("stop", serial, jsonOutput)

	case "restart":
		return a.handleService("restart", serial, jsonOutput)

	case "connect":
		return a.handleConnect(cfg, serial, cmdArgs, jsonOutput)

	case "disconnect":
		return a.handleDisconnect(serial, jsonOutput)

	case "unpair":
		return a.handleUnpair(serial, jsonOutput)

	case "logs":
		return a.handleLogs(serial, cmdArgs)

	case "filelog":
		return a.handleFileLog(serial)

	case "sync":
		return a.handleSync(serial, cmdArgs, jsonOutput)

	case "build":
		return a.handleBuild()

	case "install-apk":
		return a.handleInstallApk(serial, cmdArgs)

	case "install-bin":
		return a.handleInstallBin()

	case "uninstall-bin":
		return a.handleUninstallBin()

	case "man":
		return a.handleMan()

	case "click":
		return a.handleClick(serial, cmdArgs, jsonOutput)

	case "click-left", "left-click":
		return a.handleClick(serial, append([]string{"left"}, cmdArgs...), jsonOutput)

	case "click-right", "right-click":
		return a.handleClick(serial, append([]string{"right"}, cmdArgs...), jsonOutput)

	case "click-middle", "middle-click":
		return a.handleClick(serial, append([]string{"middle"}, cmdArgs...), jsonOutput)

	case "double-click", "doubleclick":
		return a.handleClick(serial, append([]string{"double"}, cmdArgs...), jsonOutput)

	case "mouse":
		return a.handleMouse(serial, cmdArgs, jsonOutput)

	case "raw":
		return a.handleRaw(serial, cmdArgs, jsonOutput)

	default:
		fmt.Fprintf(a.Stderr, "Unknown command %q. Run 'ghid help' for available commands.\n", cmd)
		return 1
	}
}

func (a *App) runInteractive(cfg config.Config, serial string) int {
	mgr, err := adb.NewManager(serial)
	if err != nil {
		fmt.Fprintf(a.Stderr, "Warning: %v\n", err)
	}

	// Pin the ADB serial up front when a phone is already attached. The TUI keeps
	// retrying on its own when the link appears later.
	if mgr != nil {
		_, _ = mgr.SelectDevice()
	}

	if a.InteractiveFn != nil {
		if err := a.InteractiveFn(mgr, cfg); err != nil {
			fmt.Fprintf(a.Stderr, "Interactive error: %v\n", err)
			return 1
		}
	}
	return 0
}

func (a *App) handleStatus(cfg config.Config, serial string, jsonOutput bool) int {
	var status adb.GatewayStatus
	adbConnected := false
	gatewaySerial := ""
	statusErr := ""

	if gw, err := a.GatewayFactory(serial); err != nil {
		statusErr = err.Error()
	} else {
		adbConnected = true
		if dev, devErr := gw.EnsureDevice(); devErr == nil && dev != nil {
			gatewaySerial = dev.Serial
		}
		var statusFetchErr error
		status, statusFetchErr = gw.Status()
		if statusFetchErr != nil {
			statusErr = statusFetchErr.Error()
		}
	}

	if status.State == "" {
		status.State = "OFFLINE"
	}
	if status.Target == "" {
		status.Target = "none"
	}
	if status.Address == "" {
		status.Address = "none"
	}
	if status.Updated == "" {
		status.Updated = "none"
	}

	osName, osSource := resolveTargetOS(cfg, status)
	canSend := adbConnected && status.Connected

	if jsonOutput {
		out := map[string]interface{}{
			"registered":      status.Registered,
			"connected":       status.Connected,
			"state":           status.State,
			"target":          status.Target,
			"address":         status.Address,
			"os":              osName,
			"os_source":       osSource,
			"updated":         status.Updated,
			"can_send":        canSend,
			"secure_payloads": "device_only",
			"gateway": map[string]interface{}{
				"adb_connected": adbConnected,
				"serial":        gatewaySerial,
			},
			"hid_link": map[string]interface{}{
				"registered": status.Registered,
				"connected":  status.Connected,
				"state":      status.State,
				"target":     status.Target,
				"address":    status.Address,
			},
		}
		if status.OSConfidence != "" {
			out["os_confidence"] = status.OSConfidence
		}
		if status.DetectionMethod != "" {
			out["detection_method"] = status.DetectionMethod
		}
		if statusErr != "" {
			out["error"] = statusErr
		}
		writeJSONIndent(a.Stdout, out)
		if !status.Connected {
			return 1
		}
		return 0
	}

	if statusErr != "" && !adbConnected {
		fmt.Fprintf(a.Stderr, "Error: %s\n", statusErr)
		return 1
	}

	fmt.Fprintln(a.Stdout, "\n=== GhostHID Gateway Status ===")
	phoneName := cfg.PhoneName
	if phoneName == "" {
		phoneName = "Android Device"
	}
	fmt.Fprintf(a.Stdout, "  Phone:        %s\n", phoneName)
	if adbConnected {
		fmt.Fprintf(a.Stdout, "  ADB gateway:  connected (%s)\n", gatewaySerial)
	} else {
		fmt.Fprintln(a.Stdout, "  ADB gateway:  DISCONNECTED")
	}
	fmt.Fprintf(a.Stdout, "  HID link:     %s (registered=%v)\n", status.State, status.Registered)
	fmt.Fprintf(a.Stdout, "  Target:       %s (%s)\n", status.Target, status.Address)
	fmt.Fprintf(a.Stdout, "  Target OS:    %s [%s]\n", combos.OSLabel(osName), osSource)
	if status.OSConfidence != "" && osSource == "hid_fingerprint" {
		fmt.Fprintf(a.Stdout, "  OS confidence: %s (%s)\n", status.OSConfidence, status.DetectionMethod)
	}
	if dev := cfg.GetSelectedDevice(); dev != nil {
		fmt.Fprintf(a.Stdout, "  Selected:     %s [%s] (%s)\n", dev.Name, dev.Address, dev.Platform)
	}
	if canSend {
		fmt.Fprintln(a.Stdout, "  Can send:     yes")
	} else if !adbConnected {
		fmt.Fprintln(a.Stdout, "  Can send:     no (no Android device over ADB)")
	} else {
		fmt.Fprintln(a.Stdout, "  Can send:     no (HID link not connected; run 'ghid connect')")
	}
	fmt.Fprintln(a.Stdout, "  Secure:       secure values are device-only (entries sync with value scrubbed)")
	fmt.Fprintln(a.Stdout)
	if !status.Connected {
		return 1
	}
	return 0
}

// resolveTargetOS prefers the phone's HID fingerprint and falls back to the
// configured target device so status can name the real grammar.
func resolveTargetOS(cfg config.Config, status adb.GatewayStatus) (string, string) {
	fingerprinted := strings.ToLower(strings.TrimSpace(status.OS))
	if fingerprinted != "" && fingerprinted != "unknown" {
		return fingerprinted, "hid_fingerprint"
	}
	if dev := cfg.GetSelectedDevice(); dev != nil && strings.TrimSpace(dev.Platform) != "" {
		if normalized, err := combos.NormalizeOS(dev.Platform); err == nil {
			return normalized, "selected_device"
		}
	}
	return "unknown", "none"
}

func (a *App) handleDoctor(cfg config.Config, serial string, jsonOutput bool) int {
	gw, err := a.GatewayFactory(serial)
	if err != nil {
		if jsonOutput {
			res, _ := json.Marshal(map[string]interface{}{"error": err.Error()})
			fmt.Fprintln(a.Stdout, string(res))
		} else {
			fmt.Fprintf(a.Stderr, "Error: %v\n", err)
		}
		return 2
	}

	report, err := gw.Doctor(a.Version, cfg, a.Vault)
	if err != nil {
		if jsonOutput {
			res, _ := json.Marshal(map[string]interface{}{"error": err.Error()})
			fmt.Fprintln(a.Stdout, string(res))
		} else {
			fmt.Fprintf(a.Stderr, "Error: %v\n", err)
		}
		return 2
	}

	pCount := 0
	if pStore := a.payloadStore(); pStore != nil {
		if list, _ := pStore.List(); len(list) > 0 {
			pCount = len(list)
		}
	}

	if jsonOutput {
		reportMap := map[string]interface{}{
			"version":             report.Version,
			"adb_path":            report.AdbPath,
			"adb_available":       report.AdbAvailable,
			"device_connected":    report.DeviceConnected,
			"device_model":        report.DeviceModel,
			"device_name":         report.DeviceName,
			"device_serial":       report.DeviceSerial,
			"apk_installed":       report.ApkInstalled,
			"bluetooth_on":        report.BluetoothOn,
			"gateway_registered":  report.GatewayRegistered,
			"gateway_connected":   report.GatewayConnected,
			"target_laptop":       report.TargetLaptop,
			"target_address":      report.TargetAddress,
			"selected_device":     report.SelectedDevice,
			"registered_devices":  report.RegisteredDevices,
			"keychain_available":  report.KeychainAvailable,
			"cached_user":         report.CachedUser,
			"has_cached_password": report.HasCachedPassword,
			"payloads_count":      pCount,
		}
		data, _ := json.MarshalIndent(reportMap, "", "  ")
		fmt.Fprintln(a.Stdout, string(data))
		return 0
	}

	fmt.Fprintln(a.Stdout, "\n=== GhostHID Doctor Health Check ===")
	for _, chk := range report.Checks {
		a.printCheck(chk.Name, chk.Pass, chk.Detail)
	}
	a.printCheck("Payloads Subsystem", pCount > 0, fmt.Sprintf("%d payloads loaded", pCount))

	if report.GatewayConnected {
		fmt.Fprintln(a.Stdout, "\nAll systems operational! GhostHID is ready.")
	} else {
		fmt.Fprintln(a.Stdout, "\nNote: Target laptop is not currently connected via Bluetooth.")
	}
	return 0
}

func (a *App) printCheck(name string, pass bool, detail string) {
	if pass {
		fmt.Fprintf(a.Stdout, "  \033[32m✔\033[0m %-28s %s\n", name, detail)
	} else {
		fmt.Fprintf(a.Stdout, "  \033[31m✖\033[0m %-28s %s\n", name, detail)
	}
}

func (a *App) handleLogin(cfg config.Config, serial string, args []string, jsonOutput bool) int {
	fs := flag.NewFlagSet("login", flag.ContinueOnError)
	userFlag := fs.String("u", "", "Username")
	passFlag := fs.String("p", "", "Password")
	saveFlag := fs.Bool("save", false, "Save credentials securely in Keychain")
	fs.SetOutput(a.Stderr)

	if err := fs.Parse(args); err != nil {
		return 1
	}

	username := *userFlag
	password := *passFlag

	// Resolve from PayloadStore first
	pStore := a.payloadStore()
	var userFromPayload, passFromPayload string
	if pStore != nil {
		if u, ok, _ := pStore.Get("username"); ok {
			userFromPayload = u.Value
		}
		if p, ok, _ := pStore.Get("password"); ok {
			passFromPayload = p.Value
		}
	}

	if username == "" {
		username = userFromPayload
	}
	if username == "" {
		username = cfg.DefaultUser
	}
	if password == "" {
		password = passFromPayload
	}
	if password == "" && username != "" && a.Vault != nil && a.Vault.IsAvailable() {
		p, err := a.Vault.Get(username)
		if err == nil {
			password = p
		}
	}

	if username == "" || password == "" {
		if jsonOutput {
			res, _ := json.Marshal(map[string]interface{}{
				"status":  "error",
				"message": "Username and password payloads are empty. Please set them using: ghid payload set username <val> and ghid payload set password <val> --secure",
			})
			fmt.Fprintln(a.Stdout, string(res))
		} else {
			fmt.Fprintln(a.Stderr, "✖ Error: Username and password payloads are empty.")
			fmt.Fprintln(a.Stderr, "  Please set your payload values:")
			fmt.Fprintln(a.Stderr, "    ghid payload set username <your-username>")
			fmt.Fprintln(a.Stderr, "    ghid payload set password <your-password> --secure")
			fmt.Fprintln(a.Stderr, "  Or pass them directly: ghid login -u <user> -p <pass>")
		}
		return 1
	}

	if *saveFlag {
		if pStore != nil {
			_, _ = pStore.Set("username", username, false, "type")
			_, _ = pStore.Set("password", password, true, "type")
		}
		if a.Vault != nil && a.Vault.IsAvailable() {
			_ = a.Vault.Set(username, password)
		}
		cfg.DefaultUser = username
		_ = a.ConfigStore.Save(cfg)
	}

	gw, err := a.GatewayFactory(serial)
	if err != nil {
		fmt.Fprintf(a.Stderr, "Error: %v\n", err)
		return 1
	}

	status, _ := gw.Status()
	if !status.Connected {
		if jsonOutput {
			res, _ := json.Marshal(map[string]interface{}{
				"status":  "error",
				"message": "Gateway is not connected to target laptop",
			})
			fmt.Fprintln(a.Stdout, string(res))
		} else {
			fmt.Fprintln(a.Stderr, "Error: Gateway is not connected to target laptop. Run 'ghid connect' first.")
		}
		return 1
	}

	if !jsonOutput {
		fmt.Fprintf(a.Stdout, "⚡ Initiating Windows wake & login sequence for user '%s'...\n", username)
	}

	err = gw.Login(gateway.LoginRequest{
		Username: username,
		Password: password,
		Delays:   cfg.Delays,
	}, func(step int, desc string) {
		if !jsonOutput {
			fmt.Fprintf(a.Stdout, "  [%d/6] %s\n", step, desc)
		}
	})
	if err != nil {
		if jsonOutput {
			res, _ := json.Marshal(map[string]interface{}{"status": "error", "message": err.Error()})
			fmt.Fprintln(a.Stdout, string(res))
		} else {
			fmt.Fprintf(a.Stderr, "Login sequence failed: %v\n", err)
		}
		return 1
	}

	if jsonOutput {
		res, _ := json.Marshal(map[string]interface{}{"status": "success", "user": username})
		fmt.Fprintln(a.Stdout, string(res))
	} else {
		fmt.Fprintln(a.Stdout, "✔ Login sequence completed successfully!")
	}
	return 0
}

func (a *App) handleLock(serial string, jsonOutput bool) int {
	lockSeq := combos.DefaultLockSequence
	if c, ok := a.comboStore().Find("Lock"); ok && c.Sequence != "" {
		lockSeq = c.Sequence
	}
	return a.dispatchCombo(serial, "Lock", lockSeq, jsonOutput)
}

func (a *App) handleSleep(serial string, args []string, jsonOutput bool) int {
	sleepSeq := combos.DefaultSleepSequence
	if c, ok := a.comboStore().Find("Sleep"); ok && c.Sequence != "" {
		sleepSeq = c.Sequence
	}
	return a.dispatchCombo(serial, "Sleep", sleepSeq, jsonOutput)
}

func (a *App) handleRaw(serial string, args []string, jsonOutput bool) int {
	var holdDuration time.Duration
	var hexParts []string

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--hold" || arg == "-h" {
			if i+1 < len(args) {
				i++
				durStr := args[i]
				var d time.Duration
				var err error
				if strings.HasSuffix(strings.ToLower(durStr), "ms") {
					val, parseErr := strconv.ParseFloat(strings.TrimSuffix(strings.ToLower(durStr), "ms"), 64)
					if parseErr == nil {
						d = time.Duration(val * float64(time.Millisecond))
					} else {
						err = parseErr
					}
				} else if strings.HasSuffix(strings.ToLower(durStr), "s") {
					val, parseErr := strconv.ParseFloat(strings.TrimSuffix(strings.ToLower(durStr), "s"), 64)
					if parseErr == nil {
						d = time.Duration(val * float64(time.Second))
					} else {
						err = parseErr
					}
				} else {
					d, err = time.ParseDuration(durStr)
				}
				if err != nil {
					if jsonOutput {
						res, _ := json.Marshal(map[string]interface{}{"status": "error", "error": fmt.Sprintf("invalid hold duration %q: %v", durStr, err)})
						fmt.Fprintln(a.Stdout, string(res))
					} else {
						fmt.Fprintf(a.Stderr, "Error: invalid hold duration %q\n", durStr)
					}
					return 1
				}
				holdDuration = d
			}
		} else {
			hexParts = append(hexParts, arg)
		}
	}

	if len(hexParts) == 0 {
		if jsonOutput {
			res, _ := json.Marshal(map[string]interface{}{"status": "error", "error": "usage: ghid raw <hex> [--hold <duration>]"})
			fmt.Fprintln(a.Stdout, string(res))
		} else {
			fmt.Fprintf(a.Stderr, "Usage: ghid raw <hex> [--hold <duration>]\n  2 hex chars (1 byte)  -> Report 3 (System Control / Sleep)\n  4 hex chars (2 bytes) -> Report 4 (Consumer / Media)\n  8 hex chars (4 bytes) -> Report 2 (Mouse)\n  16 hex chars (8 bytes)-> Report 1 (Keyboard)\n")
		}
		return 1
	}
	cleanHex := strings.ReplaceAll(strings.Join(hexParts, ""), " ", "")
	rawBytes, err := hid.ValidateRaw(cleanHex)
	if err != nil {
		if jsonOutput {
			res, _ := json.Marshal(map[string]interface{}{"status": "error", "error": err.Error()})
			fmt.Fprintln(a.Stdout, string(res))
		} else {
			fmt.Fprintf(a.Stderr, "Error: %v\n", err)
		}
		return 1
	}
	reportID, _ := hid.ReportIDForLength(len(rawBytes))

	gw, err := a.GatewayFactory(serial)
	if err != nil {
		if jsonOutput {
			res, _ := json.Marshal(map[string]interface{}{"status": "error", "error": err.Error()})
			fmt.Fprintln(a.Stdout, string(res))
		} else {
			fmt.Fprintf(a.Stderr, "Error: %v\n", err)
		}
		return 1
	}

	if err := gw.RawWithHold(cleanHex, holdDuration); err != nil {
		if jsonOutput {
			res, _ := json.Marshal(map[string]interface{}{"status": "error", "error": err.Error()})
			fmt.Fprintln(a.Stdout, string(res))
		} else {
			fmt.Fprintf(a.Stderr, "Raw report failed: %v\n", err)
		}
		return 1
	}

	if jsonOutput {
		outMap := map[string]interface{}{
			"status":    "success",
			"raw":       cleanHex,
			"report_id": reportID,
			"bytes":     len(rawBytes),
		}
		if holdDuration > 0 {
			outMap["hold_ms"] = holdDuration.Milliseconds()
		}
		res, _ := json.Marshal(outMap)
		fmt.Fprintln(a.Stdout, string(res))
	} else {
		if holdDuration > 0 {
			fmt.Fprintf(a.Stdout, "✔ Dispatched raw HID report %s (Report ID %d, %d bytes, held for %s)\n", cleanHex, reportID, len(rawBytes), holdDuration)
		} else {
			fmt.Fprintf(a.Stdout, "✔ Dispatched raw HID report %s (Report ID %d, %d bytes)\n", cleanHex, reportID, len(rawBytes))
		}
	}
	return 0
}

func (a *App) handleQuick(serial string, args []string, jsonOutput bool) int {
	if len(args) == 0 {
		fmt.Fprintf(a.Stderr, "Usage: ghid quick <text...> or ghid quick --raw <16-hex-chars>\n")
		return 1
	}

	isRaw := false
	var filtered []string
	for _, arg := range args {
		if arg == "--raw" || arg == "-r" {
			isRaw = true
		} else {
			filtered = append(filtered, arg)
		}
	}

	if len(filtered) == 0 {
		fmt.Fprintf(a.Stderr, "Usage: ghid quick <text...> or ghid quick --raw <16-hex-chars>\n")
		return 1
	}

	payload := strings.Join(filtered, " ")

	gw, err := a.GatewayFactory(serial)
	if err != nil {
		if jsonOutput {
			res, _ := json.Marshal(map[string]interface{}{"status": "error", "error": err.Error()})
			fmt.Fprintln(a.Stdout, string(res))
		} else {
			fmt.Fprintf(a.Stderr, "Error: %v\n", err)
		}
		return 1
	}

	if isRaw {
		cleanHex := strings.ReplaceAll(payload, " ", "")
		rawBytes, valErr := hid.ValidateRaw(cleanHex)
		if valErr != nil {
			if jsonOutput {
				res, _ := json.Marshal(map[string]interface{}{"status": "error", "error": valErr.Error()})
				fmt.Fprintln(a.Stdout, string(res))
			} else {
				fmt.Fprintf(a.Stderr, "Error: %v\n", valErr)
			}
			return 1
		}
		reportID, _ := hid.ReportIDForLength(len(rawBytes))
		err = gw.Raw(cleanHex)
		if err != nil {
			if jsonOutput {
				res, _ := json.Marshal(map[string]interface{}{"status": "error", "error": err.Error()})
				fmt.Fprintln(a.Stdout, string(res))
			} else {
				fmt.Fprintf(a.Stderr, "Raw report failed: %v\n", err)
			}
			return 1
		}
		if jsonOutput {
			res, _ := json.Marshal(map[string]interface{}{
				"status":    "success",
				"mode":      "raw",
				"raw":       cleanHex,
				"report_id": reportID,
				"bytes":     len(rawBytes),
			})
			fmt.Fprintln(a.Stdout, string(res))
		} else {
			fmt.Fprintf(a.Stdout, "✔ Dispatched raw HID report: %s (Report ID %d, %d bytes)\n", cleanHex, reportID, len(rawBytes))
		}
		return 0
	}

	// Default: Type text
	err = gw.Type(payload)
	if err != nil {
		if jsonOutput {
			res, _ := json.Marshal(map[string]interface{}{"status": "error", "error": err.Error()})
			fmt.Fprintln(a.Stdout, string(res))
		} else {
			fmt.Fprintf(a.Stderr, "Type failed: %v\n", err)
		}
		return 1
	}
	if jsonOutput {
		res, _ := json.Marshal(map[string]interface{}{"status": "success", "mode": "type", "typed": payload})
		fmt.Fprintln(a.Stdout, string(res))
	} else {
		fmt.Fprintf(a.Stdout, "✔ Typed text (%d characters)\n", len(payload))
	}
	return 0
}

func (a *App) handleCombo(cfg config.Config, serial string, args []string, jsonOutput bool) int {
	store := a.comboStore()

	// Bare `ghid combo` opens the studio so creating and executing combos is a
	// single command. Without a terminal, fall back to listing the library.
	if len(args) == 0 {
		if !stdinIsTerminal() {
			return a.listCombos(store, jsonOutput, "")
		}
		return a.openComboStudio(store, serial)
	}

	switch args[0] {
	case "studio", "new", "ui", "interactive":
		if !stdinIsTerminal() {
			return a.listCombos(store, jsonOutput, "")
		}
		return a.openComboStudio(store, serial)

	case "list", "ls":
		osFilter, _, err := parseComboOSArgs(args[1:])
		if err != nil {
			return a.comboFail(jsonOutput, "%v", err)
		}
		return a.listCombos(store, jsonOutput, osFilter)

	case "fav", "favorite":
		if len(args) < 2 {
			return a.comboFail(jsonOutput, "usage: ghid combo fav <name>")
		}
		name := strings.Join(args[1:], " ")
		list, ok, err := store.ToggleFavorite(name)
		if err != nil {
			return a.comboFail(jsonOutput, "%v", err)
		}
		if !ok {
			return a.comboFail(jsonOutput, "no saved combo named %q", name)
		}
		var c combos.Combo
		for _, item := range list {
			if strings.EqualFold(item.Name, strings.TrimSpace(name)) {
				c = item
				break
			}
		}
		pushed, report, syncErr := a.autoPush(serial)
		if jsonOutput {
			out := map[string]interface{}{
				"status":   "success",
				"name":     c.Name,
				"favorite": c.Favorite,
				"synced":   pushed && syncErr == nil,
			}
			if report != nil {
				out["sync"] = report
			}
			if syncErr != nil {
				out["sync_error"] = syncErr.Error()
			}
			writeJSONIndent(a.Stdout, out)
		} else {
			status := "unpinned"
			if c.Favorite {
				status = "pinned to favorites (❤️)"
			}
			fmt.Fprintf(a.Stdout, "✔ Combo %q is now %s%s\n", c.Name, status, syncSuffix(pushed, syncErr))
		}
		return 0

	case "schema", "tokens", "capabilities":
		return a.handleComboSchema(store, jsonOutput)

	case "keys", "key-tokens":
		// `combo keys` with no arguments dumps the token table. With a sequence
		// it stays the historical dispatch alias.
		if len(args) == 1 {
			return a.handleComboKeys(jsonOutput)
		}
		return a.handleComboDispatchArgs(cfg, serial, store, args[1:], jsonOutput)

	case "show", "get", "cat":
		if len(args) < 2 {
			return a.comboFail(jsonOutput, "usage: ghid combo show <name>")
		}
		return a.handleComboShow(store, strings.Join(args[1:], " "), jsonOutput)

	case "validate", "check":
		if len(args) < 2 {
			return a.comboFail(jsonOutput, "usage: ghid combo validate <sequence...> [--os windows|macos|linux] [--allow-unresolved]")
		}
		osName := ""
		allowUnresolved := false
		var seqParts []string
		for i := 1; i < len(args); i++ {
			arg := args[i]
			switch {
			case arg == "--os" || arg == "-o":
				if i+1 >= len(args) {
					return a.comboFail(jsonOutput, "--os requires windows, macos or linux")
				}
				i++
				osName = args[i]
			case strings.HasPrefix(arg, "--os="):
				osName = strings.TrimPrefix(arg, "--os=")
			case strings.HasPrefix(arg, "-o="):
				osName = strings.TrimPrefix(arg, "-o=")
			case arg == "--allow-unresolved":
				allowUnresolved = true
			default:
				seqParts = append(seqParts, arg)
			}
		}
		if len(seqParts) == 0 {
			return a.comboFail(jsonOutput, "usage: ghid combo validate <sequence...> [--os windows|macos|linux] [--allow-unresolved]")
		}
		return a.handleComboValidate(cfg, store, strings.Join(seqParts, " "), osName, allowUnresolved, jsonOutput)

	case "save", "add", "set", "edit":
		var positional []string
		isFav := false
		osName := ""
		description := ""
		emoji := ""
		allowUnresolved := false

		for i := 1; i < len(args); i++ {
			arg := args[i]
			switch {
			case arg == "--fav" || arg == "--favorite" || arg == "-f":
				isFav = true
			case arg == "--allow-unresolved":
				allowUnresolved = true
			case arg == "--os" || arg == "-o":
				if i+1 < len(args) {
					osName = args[i+1]
					i++
				}
			case strings.HasPrefix(arg, "--os="):
				osName = strings.TrimPrefix(arg, "--os=")
			case strings.HasPrefix(arg, "-o="):
				osName = strings.TrimPrefix(arg, "-o=")
			case arg == "--desc" || arg == "--description" || arg == "-d":
				if i+1 < len(args) {
					description = args[i+1]
					i++
				}
			case arg == "--emoji" || arg == "-e":
				if i+1 < len(args) {
					emoji = args[i+1]
					i++
				}
			case strings.HasPrefix(arg, "--emoji="):
				emoji = strings.TrimPrefix(arg, "--emoji=")
			case strings.HasPrefix(arg, "--desc="):
				description = strings.TrimPrefix(arg, "--desc=")
			case strings.HasPrefix(arg, "--description="):
				description = strings.TrimPrefix(arg, "--description=")
			case strings.HasPrefix(arg, "-d="):
				description = strings.TrimPrefix(arg, "-d=")
			default:
				positional = append(positional, arg)
			}
		}

		if len(positional) < 2 {
			return a.comboFail(jsonOutput, "usage: ghid combo save <name> <sequence...> [--os windows|macos|linux] [--emoji <e>] [--desc <d>] [--fav] [--allow-unresolved]")
		}
		name := positional[0]
		seq := strings.Join(positional[1:], " ")
		normalizedOS, err := combos.NormalizeOS(osName)
		if err != nil {
			return a.comboFail(jsonOutput, "%v", err)
		}
		_, existed := store.Find(name)
		list, err := store.Add(combos.Combo{
			Name:        name,
			Sequence:    seq,
			OS:          normalizedOS,
			Emoji:       emoji,
			Description: description,
			Favorite:    isFav,
		}, combos.AddOptions{AllowUnresolvedPayloads: allowUnresolved})
		if err != nil {
			return a.comboFail(jsonOutput, "%v", err)
		}
		pushed, report, syncErr := a.autoPush(serial)
		if jsonOutput {
			saved := combos.Combo{Name: name, Sequence: seq, OS: normalizedOS, Emoji: emoji, Description: description, Favorite: isFav}
			for _, c := range list {
				if strings.EqualFold(c.Name, name) {
					saved = c
					break
				}
			}
			out := map[string]interface{}{
				"status":  "saved",
				"updated": existed,
				"synced":  pushed && syncErr == nil,
				"combo":   saved,
			}
			if report != nil {
				out["sync"] = report
			}
			if syncErr != nil {
				out["sync_error"] = syncErr.Error()
			}
			writeJSONIndent(a.Stdout, out)
		} else {
			favMsg := ""
			if isFav {
				favMsg = " [pinned ❤️]"
			}
			osMsg := "[" + combos.OSLabel(normalizedOS) + "] "
			if emoji != "" {
				osMsg = emoji + " "
			}
			fmt.Fprintf(a.Stdout, "✔ Saved combo %s%q (%s)%s%s\n", osMsg, name, seq, favMsg, syncSuffix(pushed, syncErr))
		}
		return 0

	case "send", "run":
		dryRun := false
		var nameParts []string
		for _, arg := range args[1:] {
			if arg == "--dry-run" || arg == "--dry" {
				dryRun = true
				continue
			}
			nameParts = append(nameParts, arg)
		}
		if len(nameParts) == 0 {
			return a.comboFail(jsonOutput, "usage: ghid combo send <name> [--dry-run]")
		}
		name := strings.Join(nameParts, " ")
		combo, ok := store.Find(name)
		if !ok {
			return a.comboFail(jsonOutput, "no saved combo named %q (run 'ghid combo list --json')", name)
		}
		if dryRun {
			return a.handleComboDryRun(combo.Name, combo.Sequence, combo.OS, jsonOutput)
		}
		return a.dispatchCombo(serial, combo.Name, combo.Sequence, jsonOutput)

	case "delete", "rm", "remove":
		if len(args) < 2 {
			return a.comboFail(jsonOutput, "usage: ghid combo delete <name> [-f]")
		}
		name := args[1]
		// --json implies non-interactive: machine callers cannot answer prompts.
		force := jsonOutput
		for _, arg := range args[2:] {
			if arg == "-f" || arg == "--force" || arg == "--yes" {
				force = true
			}
		}
		if _, ok := store.Find(name); !ok {
			return a.comboFail(jsonOutput, "no saved combo named %q", name)
		}
		if !force {
			fmt.Fprintf(a.Stdout, "Delete combo %q? [y/N]: ", name)
			var resp string
			_, _ = fmt.Scanln(&resp)
			resp = strings.ToLower(strings.TrimSpace(resp))
			if resp != "y" && resp != "yes" {
				fmt.Fprintln(a.Stdout, "Aborted.")
				return 0
			}
		}
		if _, deleted, err := store.Delete(name); err != nil {
			return a.comboFail(jsonOutput, "%v", err)
		} else if !deleted {
			return a.comboFail(jsonOutput, "no saved combo named %q", name)
		}
		pushed, report, syncErr := a.autoPush(serial)
		if jsonOutput {
			out := map[string]interface{}{
				"status": "deleted",
				"name":   name,
				"synced": pushed && syncErr == nil,
			}
			if report != nil {
				out["sync"] = report
			}
			if syncErr != nil {
				out["sync_error"] = syncErr.Error()
			}
			writeJSONIndent(a.Stdout, out)
		} else {
			fmt.Fprintf(a.Stdout, "✔ Deleted combo %q%s\n", name, syncSuffix(pushed, syncErr))
		}
		return 0

	case "mouse":
		if len(args) < 2 {
			return a.comboFail(jsonOutput, "usage: ghid combo mouse <click-left|click-right|click-middle|double-click|scroll-up|scroll-down>")
		}
		return a.dispatchMouse(serial, args[1], jsonOutput)
	}

	// Default one-shot sequence dispatch
	return a.handleComboDispatchArgs(cfg, serial, store, args, jsonOutput)
}

// comboFail reports a combo error as JSON in machine mode so agents never have
// to scrape stderr.
func (a *App) comboFail(jsonOutput bool, format string, args ...interface{}) int {
	message := fmt.Sprintf(format, args...)
	if jsonOutput {
		writeJSONIndent(a.Stdout, map[string]interface{}{
			"status": "error",
			"error":  message,
		})
	} else {
		fmt.Fprintf(a.Stderr, "Error: %s\n", message)
	}
	return 1
}

// handleComboSchema exposes the grammar and every accepted token so agents can
// discover how to build combos without reading source or docs.
func (a *App) handleComboSchema(store *combos.Store, jsonOutput bool) int {
	catalog := hid.Catalog()
	schema := map[string]interface{}{
		"windows_only": false,
		"storage":      store.Path,
		"grammar":      "Plain text outside braces is typed. {...} evaluates delays ({500ms}), chords ({mod+s}), keys ({Tab}), consumer keys ({VolumeUp}), or payloads ({NAME}). Literal { is escaped as {{.",
		"step_types": []string{
			"delays: {100ms}, {500ms}, {1s}",
			"keyboard chords: {mod+s}, {ctrl+alt+del}, {mod+shift+p}",
			"standard keys: {Tab}, {Enter}, {Escape}, {F13}",
			"bare modifiers: {shift}, {ctrl}",
			"consumer keys: {VolumeUp}, {VolumeDown}, {Mute}, {PlayPause}",
			"mouse: {click-left}, {scroll-up}",
			"raw HID reports: {02}, {cd00}, {08000f0000000000}",
			"payload references: {EMAIL}, {PASSWORD}, {WIN_SLEEP}",
			"literal typing: plain text outside braces, {{ for literal {",
		},
		"keys":               catalog.Keys,
		"modifiers":          catalog.Modifiers,
		"consumer":           catalog.Consumer,
		"mac_aliases":        catalog.MacAliases,
		"mouse_actions":      catalog.MouseActions,
		"fkey_range":         catalog.FKeyRange,
		"aliases":            catalog.Aliases,
		"payload_convention": catalog.PayloadConvention,
		"notes":              catalog.Notes,
		"os_tags":            combos.OSTokens(),
		"default_os":         combos.OSWindows,
		"emoji":              "optional single emoji shown as the combo icon on Android and instead of the OS badge in the CLI",
		"mod_note":           "The 'mod' modifier maps dynamically to Cmd on macOS, and Ctrl on Windows/Linux.",
		"payload_note":       "Secure payload values live in the macOS Keychain and never cross to Android; pushed entries carry a scrubbed value (ADR-009). Create phone-only secrets on the phone.",
		"example":            "user{Tab}{PASSWORD}{Enter}{500ms}{mod+s}",
		"mouse_note":         "No pointer coordinates or cursor movement (move x, y / relative dx, dy) are supported. GhostHID clicks wherever the cursor currently rests on the target screen.",
		"commands": map[string]string{
			"click":    "ghid click [left|right|middle|double] [--double] [--count <n>] --json (clicks at resting cursor position; no coordinates)",
			"list":     "ghid combo list --json",
			"show":     "ghid combo show \"Name\" --json",
			"fav":      "ghid combo fav <name> --json",
			"save":     "ghid combo save \"Name\" \"sequence\" [--os windows|macos|linux] [--emoji <e>] [--desc <d>] [--fav] [--allow-unresolved] --json",
			"send":     "ghid combo send \"Name\" [--dry-run] --json",
			"delete":   "ghid combo delete \"Name\" --json",
			"validate": "ghid combo validate \"sequence\" [--os windows|macos|linux] [--allow-unresolved] --json",
			"keys":     "ghid combo keys --json",
		},
	}

	if jsonOutput {
		writeJSONIndent(a.Stdout, schema)
		return 0
	}

	fmt.Fprintln(a.Stdout, "\n🎹 UNIFIED BRACKET SEQUENCE DSL · Cross-Platform HID Sequences")
	fmt.Fprintf(a.Stdout, "  Storage:  %s\n", store.Path)
	fmt.Fprintf(a.Stdout, "  Grammar:  %s\n", schema["grammar"])
	fmt.Fprintf(a.Stdout, "  Example:  %s\n", schema["example"])
	fmt.Fprintf(a.Stdout, "  OS tags:  %s\n", strings.Join(combos.OSTokens(), " "))
	fmt.Fprintf(a.Stdout, "  F-keys:   %s\n\n", catalog.FKeyRange)
	printTokenSection(a.Stdout, "Modifiers", catalog.Modifiers)
	printTokenSection(a.Stdout, "Keys", catalog.Keys)
	printTokenSection(a.Stdout, "Consumer", catalog.Consumer)
	printTokenSection(a.Stdout, "Mouse", catalog.MouseActions)
	printTokenSection(a.Stdout, "macOS aliases (rejected on Windows/Linux targets)", catalog.MacAliases)
	fmt.Fprintf(a.Stdout, "  Payloads: %s\n\n", catalog.PayloadConvention)
	return 0
}

// handleComboKeys dumps the complete token table in human or JSON form.
func (a *App) handleComboKeys(jsonOutput bool) int {
	catalog := hid.Catalog()
	if jsonOutput {
		writeJSONIndent(a.Stdout, catalog)
		return 0
	}

	fmt.Fprintln(a.Stdout, "\n🎹 COMBO TOKEN TABLE")
	fmt.Fprintf(a.Stdout, "  F-keys: %s\n\n", catalog.FKeyRange)
	printTokenSection(a.Stdout, "Modifiers", catalog.Modifiers)
	printTokenSection(a.Stdout, "Keys", catalog.Keys)
	printTokenSection(a.Stdout, "Consumer", catalog.Consumer)
	printTokenSection(a.Stdout, "Mouse", catalog.MouseActions)
	printTokenSection(a.Stdout, "macOS aliases", catalog.MacAliases)

	aliasKeys := make([]string, 0, len(catalog.Aliases))
	for alias := range catalog.Aliases {
		aliasKeys = append(aliasKeys, alias)
	}
	sort.Strings(aliasKeys)
	pairs := make([]string, 0, len(aliasKeys))
	for _, alias := range aliasKeys {
		pairs = append(pairs, alias+"="+catalog.Aliases[alias])
	}
	printTokenSection(a.Stdout, "Aliases", pairs)

	fmt.Fprintf(a.Stdout, "  Payloads: %s\n", catalog.PayloadConvention)
	for _, note := range catalog.Notes {
		fmt.Fprintf(a.Stdout, "    - %s\n", note)
	}
	fmt.Fprintln(a.Stdout)
	return 0
}

func printTokenSection(w io.Writer, label string, tokens []string) {
	if len(tokens) == 0 {
		return
	}
	const perLine = 10
	fmt.Fprintf(w, "  %s:\n", label)
	for i := 0; i < len(tokens); i += perLine {
		end := i + perLine
		if end > len(tokens) {
			end = len(tokens)
		}
		fmt.Fprintf(w, "    %s\n", strings.Join(tokens[i:end], "  "))
	}
	fmt.Fprintln(w)
}

// parseComboOSArgs extracts an optional --os filter from combo list arguments.
func parseComboOSArgs(args []string) (string, []string, error) {
	osFilter := ""
	var rest []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--os" || arg == "-o":
			if i+1 >= len(args) {
				return "", rest, errors.New("--os requires windows, macos, linux, or all")
			}
			osFilter = args[i+1]
			i++
		case strings.HasPrefix(arg, "--os="):
			osFilter = strings.TrimPrefix(arg, "--os=")
		case strings.HasPrefix(arg, "-o="):
			osFilter = strings.TrimPrefix(arg, "-o=")
		default:
			rest = append(rest, arg)
		}
	}
	if osFilter == "" {
		return "", rest, nil
	}
	normalized, err := combos.NormalizeOS(osFilter)
	if err != nil {
		return "", rest, err
	}
	return normalized, rest, nil
}

func (a *App) handleComboValidate(cfg config.Config, store *combos.Store, sequence string, osName string, allowUnresolved bool, jsonOutput bool) int {
	// A saved combo name validates that combo's real sequence, and its saved OS
	// becomes the grammar default. Raw sequences keep the caller's target OS.
	comboOS := ""
	if saved, ok := store.Find(strings.TrimSpace(sequence)); ok && saved.Sequence != "" {
		sequence = saved.Sequence
		comboOS = saved.OS
	}

	osSource := "flag"
	switch {
	case osName != "":
		normalized, err := combos.NormalizeOS(osName)
		if err != nil {
			return a.comboFail(jsonOutput, "%v", err)
		}
		osName = normalized
	case comboOS != "":
		osName = combos.NormalizeOSDefault(comboOS)
		osSource = "combo"
	default:
		osName, osSource = defaultValidateOS(cfg, store, sequence)
	}

	steps, err := hid.DescribeSequence(sequence)
	if err == nil {
		err = combos.ValidateForOS(sequence, osName)
	}
	if err != nil {
		if jsonOutput {
			writeJSONIndent(a.Stdout, map[string]interface{}{
				"valid":     false,
				"sequence":  sequence,
				"os":        osName,
				"os_source": osSource,
				"error":     err.Error(),
			})
		} else {
			fmt.Fprintf(a.Stderr, "✖ Invalid sequence: %v\n", err)
		}
		return 1
	}

	report := store.PayloadRefReport(sequence)
	refErr := combos.PayloadRefError(report)
	valid := refErr == nil

	warning := ""
	if !valid {
		if allowUnresolved {
			warning = refErr.Error()
			valid = true
		} else {
			if jsonOutput {
				writeJSONIndent(a.Stdout, map[string]interface{}{
					"valid":               false,
					"sequence":            sequence,
					"os":                  osName,
					"os_source":           osSource,
					"steps":               steps,
					"payloads":            report.Referenced,
					"resolved_payloads":   report.Resolved,
					"unresolved_payloads": report.Unresolved,
					"empty_payloads":      report.Empty,
					"error":               refErr.Error(),
				})
			} else {
				fmt.Fprintf(a.Stderr, "✖ Invalid sequence: %v\n", refErr)
			}
			return 1
		}
	}

	if jsonOutput {
		out := map[string]interface{}{
			"valid":               true,
			"sequence":            sequence,
			"os":                  osName,
			"os_source":           osSource,
			"steps":               steps,
			"payloads":            report.Referenced,
			"resolved_payloads":   report.Resolved,
			"unresolved_payloads": report.Unresolved,
			"empty_payloads":      report.Empty,
		}
		if warning != "" {
			out["warning"] = warning
		}
		writeJSONIndent(a.Stdout, out)
		return 0
	}

	fmt.Fprintf(a.Stdout, "✔ Valid sequence (%s, %s referenced) [%s]: %s\n",
		countLabel(len(steps), "step"), countLabel(len(report.Referenced), "payload"), osName, sequence)
	if warning != "" {
		fmt.Fprintf(a.Stderr, "⚠ %s\n", warning)
	}
	return 0
}

// defaultValidateOS picks the grammar to validate against when --os is absent:
// the saved combo's OS (handled by the caller), then the selected target device,
// then Windows for legacy compatibility.
func defaultValidateOS(cfg config.Config, store *combos.Store, sequence string) (string, string) {
	if saved, ok := store.Find(strings.TrimSpace(sequence)); ok && saved.OS != "" {
		return combos.NormalizeOSDefault(saved.OS), "combo"
	}
	if dev := cfg.GetSelectedDevice(); dev != nil && strings.TrimSpace(dev.Platform) != "" {
		if normalized, err := combos.NormalizeOS(dev.Platform); err == nil {
			return normalized, "selected_device"
		}
	}
	return combos.OSWindows, "default"
}

func countLabel(n int, singular string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, singular)
	}
	return fmt.Sprintf("%d %ss", n, singular)
}

func syncSuffix(pushed bool, syncErr error) string {
	if !pushed {
		return " (local only; no Android device attached, not synced)"
	}
	if syncErr != nil {
		return fmt.Sprintf(" (local only; sync failed: %v)", syncErr)
	}
	return " (synced to Android)"
}

// autoPush pushes the local library only when a phone is actually attached.
func (a *App) autoPush(serial string) (bool, *syncer.SyncReport, error) {
	mgr, err := adb.NewManager(serial)
	if err != nil {
		return false, nil, err
	}
	report, pushed, err := syncer.PushIfConnected(mgr, a.comboStore(), a.payloadStore())
	return pushed, report, err
}

// handleComboShow reads a saved combo back, including its payload resolution.
func (a *App) handleComboShow(store *combos.Store, name string, jsonOutput bool) int {
	combo, ok := store.Find(name)
	if !ok {
		return a.comboFail(jsonOutput, "no saved combo named %q (run 'ghid combo list --json')", name)
	}
	steps, _ := hid.DescribeSequence(combo.Sequence)
	report := store.PayloadRefReport(combo.Sequence)

	if jsonOutput {
		writeJSONIndent(a.Stdout, map[string]interface{}{
			"status":              "success",
			"combo":               combo,
			"steps":               steps,
			"payloads":            report.Referenced,
			"resolved_payloads":   report.Resolved,
			"unresolved_payloads": report.Unresolved,
			"empty_payloads":      report.Empty,
		})
		return 0
	}

	fmt.Fprintf(a.Stdout, "\n🎹 %s %s\n", combo.Badge(), combo.Name)
	fmt.Fprintf(a.Stdout, "  OS:          %s\n", combos.OSLabel(combo.OS))
	fmt.Fprintf(a.Stdout, "  Sequence:    %s\n", combo.Sequence)
	if combo.Description != "" {
		fmt.Fprintf(a.Stdout, "  Description: %s\n", combo.Description)
	}
	fmt.Fprintf(a.Stdout, "  Favorite:    %v\n", combo.Favorite)
	fmt.Fprintf(a.Stdout, "  Created:     %s\n", combo.CreatedAt.Local().Format("2006-01-02 15:04:05"))
	if len(report.Referenced) > 0 {
		fmt.Fprintf(a.Stdout, "  Payloads:    %s\n", strings.Join(report.Referenced, ", "))
	}
	if len(report.Unresolved) > 0 {
		fmt.Fprintf(a.Stdout, "  Missing:     %s\n", strings.Join(report.Unresolved, ", "))
	}
	if len(report.Empty) > 0 {
		fmt.Fprintf(a.Stdout, "  Empty:       %s\n", strings.Join(report.Empty, ", "))
	}
	fmt.Fprintln(a.Stdout, "  Steps:")
	for i, step := range steps {
		fmt.Fprintf(a.Stdout, "    %2d. %-9s %s\n", i+1, step.Type, step.Value)
	}
	fmt.Fprintln(a.Stdout)
	return 0
}

// handleComboDispatchArgs executes a one-shot sequence, or resolves it without
// dispatching when --dry-run is present.
func (a *App) handleComboDispatchArgs(cfg config.Config, serial string, store *combos.Store, args []string, jsonOutput bool) int {
	dryRun := false
	var seqParts []string
	for _, arg := range args {
		if arg == "--dry-run" || arg == "--dry" {
			dryRun = true
			continue
		}
		seqParts = append(seqParts, arg)
	}
	if len(seqParts) == 0 {
		return a.comboFail(jsonOutput, "usage: ghid combo <sequence...> [--dry-run]")
	}
	sequence := strings.Join(seqParts, " ")
	if dryRun {
		osName, _ := defaultValidateOS(cfg, store, sequence)
		return a.handleComboDryRun("", sequence, osName, jsonOutput)
	}
	return a.dispatchCombo(serial, "", sequence, jsonOutput)
}

// resolvedComboStep is one step of a dry run: grammar, HID bytes, and payload
// resolution all visible without touching the target.
type resolvedComboStep struct {
	Type           string `json:"type"`
	Token          string `json:"token,omitempty"`
	Modifier       byte   `json:"modifier,omitempty"`
	Keycode        byte   `json:"keycode,omitempty"`
	ReportHex      string `json:"report_hex,omitempty"`
	RawHex         string `json:"raw_hex,omitempty"`
	ReportID       int    `json:"report_id,omitempty"`
	Text           string `json:"text,omitempty"`
	Mouse          string `json:"mouse,omitempty"`
	Payload        string `json:"payload,omitempty"`
	PayloadType    string `json:"payload_type,omitempty"`
	PayloadSecure  bool   `json:"payload_secure,omitempty"`
	PayloadPresent bool   `json:"payload_present,omitempty"`
	ValuePreview   string `json:"value_preview,omitempty"`
	DelayMS        int64  `json:"delay_ms,omitempty"`
	HoldMS         int64  `json:"hold_ms,omitempty"`
}

// handleComboDryRun resolves a sequence into concrete HID actions without
// dispatching anything. Secure payload values stay masked.
func (a *App) handleComboDryRun(name, sequence, osName string, jsonOutput bool) int {
	steps, err := hid.ParseSequence(sequence)
	if err != nil {
		if jsonOutput {
			writeJSONIndent(a.Stdout, map[string]interface{}{
				"dry_run":  true,
				"valid":    false,
				"name":     name,
				"sequence": sequence,
				"os":       osName,
				"error":    err.Error(),
			})
		} else {
			fmt.Fprintf(a.Stderr, "✖ Invalid sequence: %v\n", err)
		}
		return 1
	}
	if osName == "" {
		osName = combos.OSWindows
	} else {
		osName = combos.NormalizeOSDefault(osName)
	}

	pStore := a.payloadStore()
	report := pStore.ResolveRefs(hid.ExtractPayloads(sequence))
	resolved := make([]resolvedComboStep, 0, len(steps))
	for _, step := range steps {
		out := resolvedComboStep{DelayMS: step.Delay.Milliseconds(), HoldMS: step.Hold.Milliseconds()}
		switch {
		case step.Payload != "":
			out.Type = "payload"
			out.Payload = step.Payload
			payload, ok, _ := pStore.Get(step.Payload)
			value, isRaw, resolveErr := pStore.Resolver()(step.Payload)
			if !ok || resolveErr != nil {
				out.ValuePreview = "(unresolved)"
				break
			}
			out.PayloadPresent = true
			out.PayloadType = payload.EffectiveType()
			out.PayloadSecure = payload.Secure
			cleanVal := strings.TrimSpace(value)
			isExplicitRaw := strings.HasPrefix(strings.ToLower(cleanVal), "raw:")
			if isRaw || isExplicitRaw {
				rawHex := cleanVal
				if isExplicitRaw {
					rawHex = strings.TrimSpace(cleanVal[4:])
				}
				rawHex = strings.ReplaceAll(rawHex, " ", "")
				if rawBytes, rawErr := hid.ValidateRaw(rawHex); rawErr == nil {
					out.Type = "raw"
					out.ReportID, _ = hid.ReportIDForLength(len(rawBytes))
					if payload.Secure {
						out.ValuePreview = "•••••••• (secure raw report, hidden)"
					} else {
						out.RawHex = rawHex
						out.ValuePreview = rawHex
					}
				} else {
					out.Type = "raw"
					out.ValuePreview = fmt.Sprintf("(invalid raw hex: %v)", rawErr)
				}
			} else {
				out.Type = "text"
				if payload.Secure {
					out.ValuePreview = "•••••••• (secure, hidden)"
				} else {
					out.Text = value
					out.ValuePreview = value
				}
			}
		case step.Text != "":
			out.Type = "text"
			out.Text = step.Text
			out.ValuePreview = step.Text
		case step.Raw != "":
			out.Type = "raw"
			out.RawHex = step.Raw
			if rawBytes, rawErr := hid.ValidateRaw(step.Raw); rawErr == nil {
				out.ReportID, _ = hid.ReportIDForLength(len(rawBytes))
			}
		case step.Mouse != nil:
			out.Type = "mouse"
			out.Mouse = step.Mouse.Name
			out.ReportID = hid.ReportIDMouse
		case step.Delay > 0 && step.Combo != "" && step.Modifier == 0 && step.KeyCode == 0:
			out.Type = "delay"
		default:
			out.Type = "keyboard"
			out.Token = hid.NormalizeComboForAndroid(step.Combo)
			out.Modifier = step.Modifier
			out.Keycode = step.KeyCode
			out.ReportHex = hid.NewKeyReport(step.Modifier, step.KeyCode).HexString()
		}
		resolved = append(resolved, out)
	}

	valid := len(report.Unresolved) == 0 && len(report.Empty) == 0
	if jsonOutput {
		out := map[string]interface{}{
			"dry_run":             true,
			"valid":               valid,
			"name":                name,
			"sequence":            sequence,
			"os":                  osName,
			"steps":               resolved,
			"payloads":            report.Referenced,
			"resolved_payloads":   report.Resolved,
			"unresolved_payloads": report.Unresolved,
			"empty_payloads":      report.Empty,
			"dispatched":          false,
		}
		if refErr := combos.PayloadRefError(report); refErr != nil {
			out["error"] = refErr.Error()
		}
		writeJSONIndent(a.Stdout, out)
		if !valid {
			return 1
		}
		return 0
	}

	fmt.Fprintln(a.Stdout, "\n🧪 DRY RUN · nothing dispatched")
	if name != "" {
		fmt.Fprintf(a.Stdout, "  Combo:     %s\n", name)
	}
	fmt.Fprintf(a.Stdout, "  Sequence:  %s\n", sequence)
	fmt.Fprintf(a.Stdout, "  Target OS: %s\n", osName)
	fmt.Fprintln(a.Stdout, "  Steps:")
	for i, step := range resolved {
		fmt.Fprintf(a.Stdout, "    %2d. %-9s %s\n", i+1, step.Type, describeResolvedStep(step))
	}
	if !valid {
		fmt.Fprintf(a.Stderr, "✖ Invalid sequence: %v\n", combos.PayloadRefError(report))
		return 1
	}
	fmt.Fprintln(a.Stdout)
	return 0
}

func describeResolvedStep(step resolvedComboStep) string {
	var b strings.Builder
	prefix := ""
	if step.Payload != "" {
		prefix = "{" + step.Payload + "} → "
	}
	switch step.Type {
	case "keyboard":
		fmt.Fprintf(&b, "%s{%s} modifier=0x%02x keycode=0x%02x report=%s", prefix, step.Token, step.Modifier, step.Keycode, step.ReportHex)
	case "raw":
		if step.RawHex != "" {
			fmt.Fprintf(&b, "%sreport_id=%d raw=%s", prefix, step.ReportID, step.RawHex)
		} else {
			fmt.Fprintf(&b, "%sreport_id=%d raw=hidden", prefix, step.ReportID)
		}
	case "text":
		if step.Payload != "" {
			if step.PayloadSecure {
				fmt.Fprintf(&b, "%stext (secure, hidden)", prefix)
			} else {
				fmt.Fprintf(&b, "%stext %q (%d chars)", prefix, step.Text, len([]rune(step.Text)))
			}
		} else {
			fmt.Fprintf(&b, "text %q (%d chars)", step.Text, len([]rune(step.Text)))
		}
	case "mouse":
		fmt.Fprintf(&b, "%s%s (report_id=%d)", prefix, step.Mouse, step.ReportID)
	case "delay":
		fmt.Fprintf(&b, "%dms", step.DelayMS)
	default:
		b.WriteString(step.Type)
	}
	if step.Type != "delay" && step.DelayMS > 0 {
		fmt.Fprintf(&b, " +delay %dms", step.DelayMS)
	}
	if step.HoldMS > 0 {
		fmt.Fprintf(&b, " hold %dms", step.HoldMS)
	}
	return b.String()
}

// comboStore returns the configured store, or one next to the config file.
func (a *App) comboStore() *combos.Store {
	if a.ComboStore != nil {
		return a.ComboStore
	}
	dir := filepath.Dir(config.GetConfigPath())
	if a.ConfigStore != nil && a.ConfigStore.Path != "" {
		dir = filepath.Dir(a.ConfigStore.Path)
	}
	return combos.NewStore(filepath.Join(dir, "combos.json"), a.payloadStore())
}

// payloadStore returns the configured payload store or defaults one.
func (a *App) payloadStore() *payloads.Store {
	if a.PayloadStore != nil {
		return a.PayloadStore
	}
	dir := filepath.Dir(config.GetConfigPath())
	if a.ConfigStore != nil && a.ConfigStore.Path != "" {
		dir = filepath.Dir(a.ConfigStore.Path)
	}
	return payloads.NewStore(filepath.Join(dir, "payloads.json"), a.Vault)
}

func (a *App) payloadResolver() hid.PayloadResolver {
	return a.payloadStore().Resolver()
}

func stdinIsTerminal() bool {
	info, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

func (a *App) openComboStudio(store *combos.Store, serial string) int {
	if a.ComboStudioFn == nil {
		return a.listCombos(store, false, "")
	}
	send := func(combo combos.Combo) error {
		gw, err := a.GatewayFactory(serial)
		if err != nil {
			return err
		}
		return gw.ComboWithResolver(combo.Sequence, a.payloadResolver())
	}
	if err := a.ComboStudioFn(store, send); err != nil {
		fmt.Fprintf(a.Stderr, "Combo studio error: %v\n", err)
		return 1
	}
	return 0
}

func (a *App) listCombos(store *combos.Store, jsonOutput bool, osFilter string) int {
	list, err := store.Load()
	if err != nil {
		fmt.Fprintf(a.Stderr, "Error: %v\n", err)
		return 1
	}
	if osFilter != "" {
		filtered := make([]combos.Combo, 0, len(list))
		for _, c := range list {
			if combos.NormalizeOSDefault(c.OS) == osFilter {
				filtered = append(filtered, c)
			}
		}
		list = filtered
	}
	if jsonOutput {
		writeJSONIndent(a.Stdout, list)
		return 0
	}

	title := "🎹 COMBO STUDIO · Universal Sequences"
	if osFilter != "" {
		title += " · " + combos.OSLabel(osFilter)
	}
	fmt.Fprintln(a.Stdout, "\n"+title)
	if len(list) == 0 {
		if osFilter != "" {
			fmt.Fprintf(a.Stdout, "  No combos tagged for %s.\n", combos.OSLabel(osFilter))
			fmt.Fprintln(a.Stdout)
			return 0
		}
		fmt.Fprintln(a.Stdout, "  No combos saved yet.")
		fmt.Fprintln(a.Stdout, "  Create one with 'ghid combo' (interactive) or:")
		fmt.Fprintln(a.Stdout, "    ghid combo save \"Format\" \"{mod+shift+p}Format{Enter}\"")
		fmt.Fprintln(a.Stdout)
		return 0
	}
	for i, c := range list {
		favIcon := "  "
		if c.Favorite {
			favIcon = "❤️ "
		}
		osTag := c.Badge()
		displayName := osTag + " " + c.Name
		nameWidth := runewidth.StringWidth(displayName)
		namePad := ""
		if nameWidth < 26 {
			namePad = strings.Repeat(" ", 26-nameWidth)
		}
		descStr := ""
		if c.Description != "" {
			descStr = fmt.Sprintf(" (%s)", c.Description)
		}
		fmt.Fprintf(a.Stdout, "  %2d. %s%s%s %-32s%s\n", i+1, favIcon, displayName, namePad, c.Sequence, descStr)
	}
	fmt.Fprintf(a.Stdout, "\n  Send one with 'ghid combo send <name>' or filter with 'ghid combo list --os macos'\n\n")
	return 0
}

func (a *App) handlePayloads(serial string, args []string, jsonOutput bool) int {
	store := a.payloadStore()
	_, _ = store.EnsureDefaults()
	if len(args) == 0 {
		args = []string{"list"}
	}

	subcmd := args[0]
	switch subcmd {
	case "list", "ls":
		list, err := store.List()
		if err != nil {
			return a.payloadFail(jsonOutput, "%v", err)
		}
		views := make([]payloads.View, 0, len(list))
		for _, p := range list {
			views = append(views, p.View())
		}
		if jsonOutput {
			writeJSONIndent(a.Stdout, views)
			return 0
		}
		fmt.Fprintln(a.Stdout, "\n📋 PAYLOADS LIBRARY · Universal DSL Tokens")
		if len(list) == 0 {
			fmt.Fprintln(a.Stdout, "  No payloads found.")
			return 0
		}
		fmt.Fprintf(a.Stdout, "  %-3s %-20s %-8s %-26s %-14s %s\n", "#", "TOKEN", "TYPE", "VALUE", "SECURITY", "SOURCE")
		for i, p := range list {
			view := p.View()
			tokenName := "{" + strings.ToUpper(p.Name) + "}"
			displayVal := p.MaskedValue()
			sec := "[PLAIN]"
			if p.Secure {
				sec = "[KEYCHAIN 🔒]"
			}
			fmt.Fprintf(a.Stdout, "  %2d. %-20s %-8s %-26s %-14s %s\n", i+1, tokenName, p.EffectiveType(), displayVal, sec, view.Source)
		}
		fmt.Fprintln(a.Stdout)
		return 0

	case "get":
		if len(args) < 2 {
			return a.payloadFail(jsonOutput, "usage: ghid payload get <name>")
		}
		name := args[1]
		p, ok, err := store.Get(name)
		if err != nil {
			return a.payloadFail(jsonOutput, "%v", err)
		}
		if !ok {
			return a.payloadFail(jsonOutput, "payload %q not found", name)
		}
		view := p.View()
		if jsonOutput {
			writeJSONIndent(a.Stdout, view)
			if !view.Present {
				return 1
			}
			return 0
		}
		if !view.Present {
			fmt.Fprintf(a.Stderr, "✖ Payload {%s} is secure but its Keychain item is missing.\n", strings.ToUpper(p.Name))
			fmt.Fprintf(a.Stderr, "  Service: %s\n  Account: %s\n", view.KeychainService, view.KeychainAccount)
			fmt.Fprintf(a.Stderr, "  Re-set it with: ghid payload set %s <value> --secure\n", p.Name)
			return 1
		}
		fmt.Fprintf(a.Stdout, "%s\n", p.Value)
		return 0

	case "set":
		parsed, err := parsePayloadSetArgs(args[1:])
		if err != nil {
			return a.payloadFail(jsonOutput, "%v", err)
		}
		p, err := store.Set(parsed.Name, parsed.Value, parsed.Secure, parsed.Type)
		if err != nil {
			return a.payloadFail(jsonOutput, "%v", err)
		}
		pushed, _, syncErr := a.autoPush(serial)
		if jsonOutput {
			writeJSONIndent(a.Stdout, p.View())
			return 0
		}
		status := "plain text"
		if parsed.Secure {
			status = "secure, stored in Keychain"
		}
		suffix := ""
		if parsed.Secure {
			suffix = " (device-only; secure values never sync)"
		} else {
			suffix = syncSuffix(pushed, syncErr)
		}
		fmt.Fprintf(a.Stdout, "✔ Payload {%s} saved (%s)%s\n", strings.ToUpper(p.Name), status, suffix)
		return 0

	case "delete", "rm":
		if len(args) < 2 {
			return a.payloadFail(jsonOutput, "usage: ghid payload delete <name>")
		}
		name := args[1]
		if strings.EqualFold(name, "username") || strings.EqualFold(name, "password") || strings.EqualFold(name, "email") || strings.EqualFold(name, "win_sleep") || strings.EqualFold(name, "play_pause") {
			return a.payloadFail(jsonOutput, "default payload %q cannot be deleted (clear it with 'ghid payload set %s \"\"')", name, name)
		}
		_, deleted, err := store.Delete(name)
		if err != nil {
			return a.payloadFail(jsonOutput, "%v", err)
		}
		if !deleted {
			return a.payloadFail(jsonOutput, "payload %q not found", name)
		}
		pushed, _, syncErr := a.autoPush(serial)
		if jsonOutput {
			writeJSONIndent(a.Stdout, map[string]interface{}{
				"status": "deleted",
				"name":   name,
				"synced": pushed && syncErr == nil,
			})
		} else {
			fmt.Fprintf(a.Stdout, "✔ Deleted payload %q%s\n", name, syncSuffix(pushed, syncErr))
		}
		return 0

	default:
		return a.payloadFail(jsonOutput, "unknown payload subcommand %q (usage: ghid payload [list|get|set|delete])", subcmd)
	}
}

// payloadSetArgs is the parsed form of `payload set` arguments.
type payloadSetArgs struct {
	Name   string
	Value  string
	Secure bool
	Type   string
}

// parsePayloadSetArgs accepts flags in any position, including the space form
// (--type raw). Unknown flags are errors, never silently folded into the value.
func parsePayloadSetArgs(args []string) (payloadSetArgs, error) {
	parsed := payloadSetArgs{Type: "type"}
	var positional []string
	literalValue := false

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if literalValue {
			positional = append(positional, arg)
			continue
		}
		switch {
		case arg == "--":
			literalValue = true
		case arg == "--secure" || arg == "-s":
			parsed.Secure = true
		case arg == "--raw" || arg == "-r":
			parsed.Type = "raw"
		case arg == "--type" || arg == "-t":
			if i+1 >= len(args) {
				return parsed, errors.New("--type requires type or raw")
			}
			i++
			parsed.Type = args[i]
		case strings.HasPrefix(arg, "--type="):
			parsed.Type = strings.TrimPrefix(arg, "--type=")
		case strings.HasPrefix(arg, "-t="):
			parsed.Type = strings.TrimPrefix(arg, "-t=")
		case strings.HasPrefix(arg, "-") && arg != "-":
			return parsed, fmt.Errorf("unknown flag %q (use --secure, --raw, --type type|raw, or -- before a value starting with -)", arg)
		default:
			positional = append(positional, arg)
		}
	}

	if len(positional) < 1 {
		return parsed, errors.New("usage: ghid payload set <name> <value> [--secure] [--type type|raw]")
	}
	if len(positional) < 2 {
		return parsed, errors.New("payload value is required (pass \"\" to clear it)")
	}
	parsed.Name = positional[0]
	parsed.Value = strings.Join(positional[1:], " ")
	parsed.Type = strings.ToLower(strings.TrimSpace(parsed.Type))
	if parsed.Type != "type" && parsed.Type != "raw" {
		return parsed, fmt.Errorf("unknown payload type %q (use type or raw)", parsed.Type)
	}
	return parsed, nil
}

func (a *App) payloadFail(jsonOutput bool, format string, args ...interface{}) int {
	message := fmt.Sprintf(format, args...)
	if jsonOutput {
		writeJSONIndent(a.Stdout, map[string]interface{}{
			"status": "error",
			"error":  message,
		})
	} else {
		fmt.Fprintf(a.Stderr, "Error: %s\n", message)
	}
	return 1
}

func writeJSONIndent(w io.Writer, value interface{}) {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	_ = enc.Encode(value)
}

func (a *App) dispatchMouse(serial, action string, jsonOutput bool) int {
	gw, err := a.GatewayFactory(serial)
	if err != nil {
		return a.comboFail(jsonOutput, "%v", err)
	}
	if err := gw.Mouse(action); err != nil {
		return a.comboFail(jsonOutput, "mouse gesture failed: %v", err)
	}
	if jsonOutput {
		res, _ := json.Marshal(map[string]interface{}{"status": "success", "mouse": action})
		fmt.Fprintln(a.Stdout, string(res))
	} else {
		fmt.Fprintf(a.Stdout, "✔ Dispatched mouse gesture: %s\n", action)
	}
	return 0
}

func (a *App) clickError(jsonOutput bool, format string, args ...interface{}) int {
	message := fmt.Sprintf(format, args...)
	if jsonOutput {
		writeJSONIndent(a.Stdout, map[string]interface{}{
			"status": "error",
			"error":  message,
		})
	} else {
		fmt.Fprintf(a.Stderr, "Error: %s\n", message)
	}
	return 1
}

func (a *App) printClickHelp() {
	fmt.Fprintln(a.Stdout, `GhostHID Click - Mouse Button Dispatcher

Usage:
  ghid click [left|right|middle|double] [flags]

Buttons:
  left              Primary click (default)
  right             Secondary / context menu click
  middle            Middle mouse button / wheel click
  double            Double left-click (native 2-cycle press/release)

Flags:
  -b, --button <b>  Select button: left, right, middle
  -d, --double      Double-click (equivalent to count=2 on left button)
  -c, --count <n>   Number of clicks to dispatch (default: 1)
  --json            Emit machine-readable JSON output (for AI agents & scripts)
  -h, --help        Show this help message

Important Architectural Notice for AI Agents:
  No pointer coordinates or cursor movement (move x, y / relative dx, dy) are supported.
  GhostHID operates strictly at the USB HID hardware layer as a Bluetooth peripheral.
  It has no awareness of host screen resolution, window bounds, or cursor coordinates.
  Clicks are dispatched wherever the cursor currently happens to be resting on screen.
  Use keyboard navigation (Tab, arrow keys, shortcuts, search) to reach target elements.`)
}

func (a *App) handleClick(serial string, args []string, jsonOutput bool) int {
	// 1. Check for coordinate or movement attempts
	if isCoord, detail := hid.IsCoordinateOrMoveAttempt(args); isCoord {
		if jsonOutput {
			writeJSONIndent(a.Stdout, map[string]interface{}{
				"status":                    "error",
				"error":                     fmt.Sprintf("%s (%s)", hid.NoCoordinatesExplanation, detail),
				"code":                      "COORDINATES_NOT_SUPPORTED",
				"coordinates_supported":     false,
				"cursor_movement_supported": false,
				"usage":                     "ghid click [left|right|middle|double] [--double] [--count <n>] [--json]",
			})
		} else {
			fmt.Fprintf(a.Stderr, "✖ Error: %s\n", hid.NoCoordinatesExplanation)
			fmt.Fprintf(a.Stderr, "  Detail: %s\n", detail)
			fmt.Fprintf(a.Stderr, "  Usage: ghid click [left|right|middle|double] [--double] [--count <n>] [--json]\n")
		}
		return 1
	}

	button := "left"
	count := 1
	doubleClick := false

	// Parse flags and positional arguments
	var positional []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--help" || arg == "-h":
			a.printClickHelp()
			return 0
		case arg == "--double" || arg == "-d":
			doubleClick = true
		case arg == "--button" || arg == "-b":
			if i+1 < len(args) {
				button = args[i+1]
				i++
			}
		case strings.HasPrefix(arg, "--button="):
			button = strings.TrimPrefix(arg, "--button=")
		case arg == "--count" || arg == "-c":
			if i+1 < len(args) {
				n, err := strconv.Atoi(args[i+1])
				if err != nil || n < 1 {
					return a.clickError(jsonOutput, "invalid click count %q: must be a positive integer", args[i+1])
				}
				count = n
				i++
			}
		case strings.HasPrefix(arg, "--count="):
			n, err := strconv.Atoi(strings.TrimPrefix(arg, "--count="))
			if err != nil || n < 1 {
				return a.clickError(jsonOutput, "invalid click count %q: must be a positive integer", arg)
			}
			count = n
		default:
			if strings.HasPrefix(arg, "-") {
				return a.clickError(jsonOutput, "unknown flag %q; run 'ghid click --help'", arg)
			}
			positional = append(positional, arg)
		}
	}

	if len(positional) > 1 {
		return a.clickError(jsonOutput, "too many arguments %v; usage: ghid click [left|right|middle|double]", positional)
	}
	if len(positional) == 1 {
		button = positional[0]
	}

	normButton, isDouble, err := hid.NormalizeClickButton(button)
	if err != nil {
		return a.clickError(jsonOutput, "%s", err.Error())
	}
	if isDouble {
		doubleClick = true
	}
	if doubleClick && count == 1 {
		count = 2
	}

	gw, err := a.GatewayFactory(serial)
	if err != nil {
		if jsonOutput {
			writeJSONIndent(a.Stdout, map[string]interface{}{
				"status": "error",
				"error":  err.Error(),
			})
		} else {
			fmt.Fprintf(a.Stderr, "Error: %v\n", err)
		}
		return 1
	}

	if err := gw.Click(normButton, count); err != nil {
		if jsonOutput {
			writeJSONIndent(a.Stdout, map[string]interface{}{
				"status": "error",
				"error":  fmt.Sprintf("click failed: %v", err),
			})
		} else {
			fmt.Fprintf(a.Stderr, "Error: click failed: %v\n", err)
		}
		return 1
	}

	actionName := "click-" + normButton
	if count == 2 && normButton == "left" {
		actionName = "double-click"
	}
	reportHex := hid.MouseReportHex(normButton)

	if jsonOutput {
		out := map[string]interface{}{
			"status":                "success",
			"command":               "click",
			"button":                normButton,
			"clicks":                count,
			"action":                actionName,
			"report_id":             hid.ReportIDMouse,
			"report_hex":            reportHex,
			"coordinates_supported": false,
			"cursor_context":        "resting_position",
			"note":                  "Clicked at resting cursor position. No pointer coordinates or cursor movement (move x, y / relative dx, dy) are supported by GhostHID.",
		}
		writeJSONIndent(a.Stdout, out)
	} else {
		if count == 2 && normButton == "left" {
			fmt.Fprintln(a.Stdout, "✔ Dispatched double-click at resting cursor position (no coordinates)")
		} else if count > 1 {
			fmt.Fprintf(a.Stdout, "✔ Dispatched %s click x%d at resting cursor position (no coordinates)\n", normButton, count)
		} else {
			fmt.Fprintf(a.Stdout, "✔ Dispatched %s click at resting cursor position (no coordinates)\n", normButton)
		}
	}
	return 0
}

func (a *App) handleMouse(serial string, args []string, jsonOutput bool) int {
	if len(args) == 0 {
		return a.handleClick(serial, nil, jsonOutput)
	}
	// Check for coordinates attempt
	if isCoord, _ := hid.IsCoordinateOrMoveAttempt(args); isCoord {
		return a.handleClick(serial, args, jsonOutput)
	}
	first := strings.ToLower(args[0])
	switch first {
	case "--help", "-h":
		a.printClickHelp()
		return 0
	case "click":
		return a.handleClick(serial, args[1:], jsonOutput)
	case "click-left", "left-click", "click-right", "right-click", "click-middle", "middle-click", "double-click", "doubleclick":
		return a.handleClick(serial, args, jsonOutput)
	case "scroll-up", "scrollup", "wheel-up", "scroll-down", "scrolldown", "wheel-down":
		return a.dispatchMouse(serial, first, jsonOutput)
	default:
		// Attempt click or mouse action
		if _, _, err := hid.NormalizeClickButton(first); err == nil {
			return a.handleClick(serial, args, jsonOutput)
		}
		return a.dispatchMouse(serial, first, jsonOutput)
	}
}

func (a *App) dispatchCombo(serial, name, sequence string, jsonOutput bool) int {
	gw, err := a.GatewayFactory(serial)
	if err != nil {
		return a.comboFail(jsonOutput, "%v", err)
	}

	err = gw.ComboWithResolver(sequence, a.payloadResolver())
	if err != nil {
		return a.comboFail(jsonOutput, "sequence failed: %v", err)
	}
	if jsonOutput {
		payload := map[string]interface{}{"status": "success", "sequence": sequence}
		if name != "" {
			payload["name"] = name
			if strings.EqualFold(name, "Lock") {
				payload["action"] = "lock"
			} else if strings.EqualFold(name, "Sleep") {
				payload["action"] = "sleep"
			}
		}
		res, _ := json.Marshal(payload)
		fmt.Fprintln(a.Stdout, string(res))
	} else if strings.EqualFold(name, "Lock") {
		fmt.Fprintln(a.Stdout, "✔ Windows lock command dispatched (Win + L)")
	} else if strings.EqualFold(name, "Sleep") {
		fmt.Fprintln(a.Stdout, "✔ Sleep sequence dispatched.")
	} else if name != "" {
		fmt.Fprintf(a.Stdout, "✔ Dispatched %q: %s\n", name, sequence)
	} else {
		fmt.Fprintf(a.Stdout, "✔ Dispatched sequence: %s\n", sequence)
	}
	return 0
}

func (a *App) handleDevices(cfg config.Config, serial string, args []string, jsonOutput bool) int {
	if len(args) == 0 {
		args = []string{"list"}
	}
	subcmd := args[0]
	switch subcmd {
	case "list":
		if jsonOutput {
			data, _ := json.MarshalIndent(cfg.Devices, "", "  ")
			fmt.Fprintln(a.Stdout, string(data))
		} else {
			fmt.Fprintln(a.Stdout, "\n=== Registered Target Devices ===")
			if len(cfg.Devices) == 0 {
				fmt.Fprintln(a.Stdout, "  No devices registered yet. Run 'ghid devices add' or 'ghid' to register one.")
			}
			for _, d := range cfg.Devices {
				sel := " "
				if d.Selected {
					sel = "[SELECTED]"
				}
				osIcon := "🪟"
				if d.Platform == "macos" {
					osIcon = "🍏"
				}
				fmt.Fprintf(a.Stdout, "  %s %-10s %-20s %-20s %s\n", osIcon, fmt.Sprintf("[%s]", d.Platform), d.Name, d.Address, sel)
			}
			fmt.Fprintln(a.Stdout)
		}
		return 0

	case "scan", "paired":
		gw, err := a.GatewayFactory(serial)
		if err != nil {
			if jsonOutput {
				res, _ := json.Marshal(map[string]interface{}{"error": err.Error()})
				fmt.Fprintln(a.Stdout, string(res))
			} else {
				fmt.Fprintf(a.Stderr, "Error: %v\n", err)
			}
			return 1
		}
		bonded, err := gw.GetBondedDevices()
		if err != nil {
			if jsonOutput {
				res, _ := json.Marshal(map[string]interface{}{"error": err.Error()})
				fmt.Fprintln(a.Stdout, string(res))
			} else {
				fmt.Fprintf(a.Stderr, "Error: %v\n", err)
			}
			return 1
		}
		if jsonOutput {
			data, _ := json.MarshalIndent(bonded, "", "  ")
			fmt.Fprintln(a.Stdout, string(data))
		} else {
			fmt.Fprintln(a.Stdout, "\n=== Paired Bluetooth Devices on Phone ===")
			if len(bonded) == 0 {
				fmt.Fprintln(a.Stdout, "  No bonded devices found on phone.")
			}
			for _, d := range bonded {
				osGuess := "windows"
				nameLower := strings.ToLower(d.Name)
				if strings.Contains(nameLower, "mac") || strings.Contains(nameLower, "apple") || strings.Contains(nameLower, "air") {
					osGuess = "macos"
				}
				osIcon := "🪟"
				if osGuess == "macos" {
					osIcon = "🍏"
				}
				fmt.Fprintf(a.Stdout, "  %s %-10s %-25s %-20s [%s]\n", osIcon, fmt.Sprintf("[%s]", osGuess), d.Name, d.Address, d.Type)
			}
			fmt.Fprintln(a.Stdout)
		}
		return 0

	case "add":
		fs := flag.NewFlagSet("devices add", flag.ContinueOnError)
		name := fs.String("name", "", "Device friendly name")
		addr := fs.String("addr", "", "Bluetooth MAC address")
		platform := fs.String("os", "windows", "Target OS: windows or macos")
		selectNow := fs.Bool("select", true, "Set as active target immediately")
		fs.SetOutput(a.Stderr)
		if err := fs.Parse(args[1:]); err != nil {
			return 1
		}

		if *name == "" || *addr == "" {
			fmt.Fprintf(a.Stderr, "Usage: ghid devices add --name <name> --addr <mac> [--os windows|macos] [--select]\n")
			return 1
		}

		newDev := cfg.AddDevice(*name, *addr, *platform, *selectNow)
		_ = a.ConfigStore.Save(cfg)
		if jsonOutput {
			data, _ := json.Marshal(newDev)
			fmt.Fprintln(a.Stdout, string(data))
		} else {
			fmt.Fprintf(a.Stdout, "✔ Registered device '%s' [%s] (%s).\n", newDev.Name, newDev.Address, newDev.Platform)
		}

		if *selectNow {
			gw, err := a.GatewayFactory(serial)
			if err == nil {
				_ = gw.Connect(newDev.Address)
			}
		}
		return 0

	case "select":
		if len(args) < 2 {
			fmt.Fprintf(a.Stderr, "Usage: ghid devices select <id_or_addr_or_name>\n")
			return 1
		}
		target := args[1]
		if cfg.SelectDevice(target) {
			_ = a.ConfigStore.Save(cfg)
			dev := cfg.GetSelectedDevice()
			if jsonOutput {
				data, _ := json.Marshal(dev)
				fmt.Fprintln(a.Stdout, string(data))
			} else {
				fmt.Fprintf(a.Stdout, "✔ Selected active target '%s' [%s].\n", dev.Name, dev.Address)
			}
			gw, err := a.GatewayFactory(serial)
			if err == nil {
				_ = gw.Connect(dev.Address)
			}
			return 0
		} else {
			fmt.Fprintf(a.Stderr, "Error: Device '%s' not found.\n", target)
			return 1
		}

	case "delete", "rm", "unregister":
		if len(args) < 2 {
			fmt.Fprintf(a.Stderr, "Usage: ghid devices delete <id_or_addr_or_name> [-f]\n")
			return 1
		}
		target := args[1]
		force := false
		for _, arg := range args[2:] {
			if arg == "-f" || arg == "--force" {
				force = true
			}
		}

		if !force {
			fmt.Fprintf(a.Stdout, "Are you sure you want to unregister '%s'? [y/N]: ", target)
			var resp string
			_, _ = fmt.Scanln(&resp)
			resp = strings.ToLower(strings.TrimSpace(resp))
			if resp != "y" && resp != "yes" {
				fmt.Fprintln(a.Stdout, "Aborted.")
				return 0
			}
		}

		deleted, ok := cfg.DeleteDevice(target)
		if ok && deleted != nil {
			_ = a.ConfigStore.Save(cfg)
			if jsonOutput {
				fmt.Fprintf(a.Stdout, `{"status": "deleted", "name": "%s", "address": "%s"}`+"\n", deleted.Name, deleted.Address)
			} else {
				fmt.Fprintf(a.Stdout, "✔ Unregistered device '%s' [%s].\n", deleted.Name, deleted.Address)
			}
			return 0
		} else {
			fmt.Fprintf(a.Stderr, "Error: Device '%s' not found.\n", target)
			return 1
		}

	default:
		fmt.Fprintf(a.Stderr, "Unknown devices subcommand '%s'. Usage: ghid devices [list|scan|add|select|delete]\n", subcmd)
		return 1
	}
}

func (a *App) handleService(action, serial string, jsonOutput bool) int {
	gw, err := a.GatewayFactory(serial)
	if err != nil {
		fmt.Fprintf(a.Stderr, "Error: %v\n", err)
		return 1
	}
	switch action {
	case "start":
		_ = gw.StartService()
		fmt.Fprintln(a.Stdout, "✔ GhostHID Foreground Service started.")
	case "stop":
		_ = gw.StopService()
		fmt.Fprintln(a.Stdout, "✔ GhostHID Foreground Service stopped.")
	case "restart":
		_ = gw.RestartService()
		fmt.Fprintln(a.Stdout, "✔ GhostHID Foreground Service restarted.")
	}
	return 0
}

func (a *App) handleConnect(cfg config.Config, serial string, args []string, jsonOutput bool) int {
	target := cfg.TargetAddress
	if len(args) > 0 {
		target = args[0]
	}
	gw, err := a.GatewayFactory(serial)
	if err != nil {
		fmt.Fprintf(a.Stderr, "Error: %v\n", err)
		return 1
	}
	_ = gw.Connect(target)
	fmt.Fprintf(a.Stdout, "✔ Bluetooth connection request sent for %s\n", target)
	return 0
}

func (a *App) handleDisconnect(serial string, jsonOutput bool) int {
	gw, err := a.GatewayFactory(serial)
	if err != nil {
		fmt.Fprintf(a.Stderr, "Error: %v\n", err)
		return 1
	}
	_ = gw.Disconnect()
	fmt.Fprintln(a.Stdout, "✔ Disconnected from target laptop.")
	return 0
}

func (a *App) handleUnpair(serial string, jsonOutput bool) int {
	gw, err := a.GatewayFactory(serial)
	if err != nil {
		fmt.Fprintf(a.Stderr, "Error: %v\n", err)
		return 1
	}
	_ = gw.Unpair("")
	fmt.Fprintln(a.Stdout, "✔ Unpaired target laptop.")
	return 0
}

func (a *App) handleLogs(serial string, args []string) int {
	gw, err := a.GatewayFactory(serial)
	if err != nil {
		fmt.Fprintf(a.Stderr, "Error: %v\n", err)
		return 1
	}
	logs, err := gw.GetLogs(40)
	if err != nil {
		fmt.Fprintf(a.Stderr, "Error: %v\n", err)
		return 1
	}
	fmt.Fprintln(a.Stdout, logs)
	return 0
}

func (a *App) handleFileLog(serial string) int {
	gw, err := a.GatewayFactory(serial)
	if err != nil {
		fmt.Fprintf(a.Stderr, "Error: %v\n", err)
		return 1
	}
	log, _ := gw.GetFileLog()
	fmt.Fprintln(a.Stdout, log)
	return 0
}

func (a *App) handleBuild() int {
	var cmd *exec.Cmd
	artifact := "build/ghost-hid.apk"
	if _, err := os.Stat("build.sh"); err == nil {
		fmt.Fprintln(a.Stdout, "Building GhostHID release APK (bash build.sh -> :app:assembleRelease)...")
		cmd = exec.Command("bash", "build.sh")
	} else if _, err := os.Stat(filepath.Join("android", "gradlew")); err == nil {
		fmt.Fprintln(a.Stdout, "Building GhostHID release APK (android/gradlew :app:assembleRelease)...")
		cmd = exec.Command("./gradlew", ":app:assembleRelease")
		cmd.Dir = "android"
		artifact = "android/app/build/outputs/apk/release/app-release.apk"
	} else {
		fmt.Fprintln(a.Stderr, "Build failed: build.sh not found (run ghid build from the GhostHID repo root)")
		return 1
	}
	cmd.Stdout = a.Stdout
	cmd.Stderr = a.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(a.Stderr, "Build failed: %v\n", err)
		return 1
	}
	fmt.Fprintf(a.Stdout, "✔ Build successful: %s (release)\n", artifact)
	return 0
}

func (a *App) handleInstallApk(serial string, args []string) int {
	apkPath := ""
	if len(args) > 0 {
		apkPath = args[0]
	} else {
		candidates := []string{
			"build/ghost-hid.apk",
			"android/app/build/outputs/apk/release/app-release.apk",
			"ghost-hid.apk",
		}
		for _, cand := range candidates {
			if _, err := os.Stat(cand); err == nil {
				apkPath = cand
				break
			}
		}
		if apkPath == "" {
			apkPath = "build/ghost-hid.apk"
		}
	}
	mgr, err := adb.NewManager(serial)
	if err != nil {
		fmt.Fprintf(a.Stderr, "Error: %v\n", err)
		return 1
	}
	fmt.Fprintf(a.Stdout, "Installing %s onto Android device...\n", apkPath)
	err = mgr.InstallApk(apkPath)
	if err != nil {
		fmt.Fprintf(a.Stderr, "Install failed: %v\n", err)
		return 1
	}
	fmt.Fprintln(a.Stdout, "✔ APK successfully installed!")
	return 0
}

func (a *App) handleInstallBin() int {
	exePath, err := os.Executable()
	if err != nil {
		fmt.Fprintf(a.Stderr, "Failed to resolve current binary path: %v\n", err)
		return 1
	}

	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(a.Stderr, "Failed to resolve home dir: %v\n", err)
		return 1
	}

	binDir := filepath.Join(home, ".local", "bin")
	err = os.MkdirAll(binDir, 0755)
	if err != nil {
		fmt.Fprintf(a.Stderr, "Failed to create ~/.local/bin: %v\n", err)
		return 1
	}

	symlinkPath := filepath.Join(binDir, "ghid")
	_ = os.Remove(symlinkPath)
	err = os.Symlink(exePath, symlinkPath)
	if err != nil {
		fmt.Fprintf(a.Stderr, "Failed to create symlink: %v\n", err)
		return 1
	}
	fmt.Fprintf(a.Stdout, "✔ Created symlink: %s -> %s\n", symlinkPath, exePath)

	manDir := filepath.Join(home, ".local", "share", "man", "man1")
	_ = os.MkdirAll(manDir, 0755)
	cwd, _ := os.Getwd()
	manSrc := filepath.Join(cwd, "man", "ghid.1")
	manDest := filepath.Join(manDir, "ghid.1")
	if _, err := os.Stat(manSrc); err == nil {
		_ = os.Remove(manDest)
		_ = os.Symlink(manSrc, manDest)
		fmt.Fprintf(a.Stdout, "✔ Installed man page: %s -> %s\n", manDest, manSrc)
	}

	fmt.Fprintln(a.Stdout, "\nGhostHID Go CLI is now installed as 'ghid'!")
	return 0
}

func (a *App) handleUninstallBin() int {
	home, _ := os.UserHomeDir()
	symlinkPath := filepath.Join(home, ".local", "bin", "ghid")
	manDest := filepath.Join(home, ".local", "share", "man", "man1", "ghid.1")
	_ = os.Remove(symlinkPath)
	_ = os.Remove(manDest)
	fmt.Fprintln(a.Stdout, "✔ Uninstalled ghid binary symlink and man page.")
	return 0
}

func (a *App) handleMan() int {
	cmd := exec.Command("man", "ghid")
	cmd.Stdout = a.Stdout
	cmd.Stderr = a.Stderr
	cmd.Stdin = os.Stdin
	_ = cmd.Run()
	return 0
}

func (a *App) syncFail(jsonOutput bool, format string, args ...interface{}) int {
	message := fmt.Sprintf(format, args...)
	if jsonOutput {
		writeJSONIndent(a.Stdout, map[string]interface{}{
			"status": "error",
			"error":  message,
		})
	} else {
		fmt.Fprintf(a.Stderr, "Error: %s\n", message)
	}
	return 1
}

func (a *App) handleSync(serial string, args []string, jsonOutput bool) int {
	mgr, err := adb.NewManager(serial)
	if err != nil {
		return a.syncFail(jsonOutput, "%v", err)
	}
	dev, err := mgr.SelectDevice()
	if err != nil {
		return a.syncFail(jsonOutput, "no device connected for sync: %v", err)
	}

	mode := "full"
	for _, arg := range args {
		switch arg {
		case "--pull":
			mode = "pull"
		case "--push":
			mode = "push"
		}
	}

	var report *syncer.SyncReport
	switch mode {
	case "pull":
		report, err = syncer.Pull(mgr, a.ComboStore, a.PayloadStore)
	case "push":
		report, err = syncer.Push(mgr, a.ComboStore, a.PayloadStore)
	default:
		report, err = syncer.Sync(mgr, a.ComboStore, a.PayloadStore)
	}

	if err != nil {
		return a.syncFail(jsonOutput, "sync error: %v", err)
	}

	if jsonOutput {
		writeJSONIndent(a.Stdout, map[string]interface{}{
			"status": "success",
			"mode":   mode,
			"device": dev.Serial,
			"report": report,
		})
		return 0
	}

	fmt.Fprintf(a.Stdout, "\n🔄 GHOST-HID SYNC COMPLETE (%s)\n", dev.Serial)
	fmt.Fprintf(a.Stdout, "  Mode:             %s\n", mode)
	if report.CombosPulled > 0 || report.PayloadsPulled > 0 {
		fmt.Fprintf(a.Stdout, "  Pulled (Android): %d combos, %d payloads\n", report.CombosPulled, report.PayloadsPulled)
	}
	if report.CombosPushed > 0 || report.PayloadsPushed > 0 {
		fmt.Fprintf(a.Stdout, "  Pushed (Android): %d combos, %d payloads\n", report.CombosPushed, report.PayloadsPushed)
	}
	if report.PayloadsSkippedSecure > 0 {
		fmt.Fprintf(a.Stdout, "  Secure skipped:   %d payloads (values scrubbed; secrets stay device-only)\n", report.PayloadsSkippedSecure)
	}
	if report.AppRestarted {
		fmt.Fprintf(a.Stdout, "  App Restarted:    ✔ com.sj010.ghid\n")
	}
	if report.Reconnected {
		fmt.Fprintf(a.Stdout, "  Bluetooth Link:   ✔ Reconnect broadcast dispatched\n")
	}
	fmt.Fprintln(a.Stdout)
	return 0
}

func (a *App) printHelp() {
	fmt.Fprintln(a.Stdout, `GhostHID - Android Bluetooth HID Keyboard Controller (Go/Charm TUI)

Usage:
  ghid [command] [options]

Commands:
  interactive (i)   Launch animated 24-bit Lip Gloss interactive dashboard (default)
  status            Query gateway & Bluetooth status; exit 1 unless the HID link is connected
                    (--json reports gateway, hid_link, can_send, os_source, secure_payloads)
  doctor            Comprehensive hardware & network diagnostic health check
  devices           Manage registered target devices (list, add, select, delete)
  payload           Manage text & secret payloads ({NAME}, custom)
  payload list      List all payloads with source/security (--json supported)
  payload get <name> Get payload value; secure misses report present:false + exit 1
  payload set <name> <value> [--secure] [--type type|raw] Save or update payload
                    (flags accepted in any position; --type raw and --type=raw both work)
  payload delete <name> Delete a payload
  login             Automated Windows unlock & credential injection
  lock              Instant Windows lock screen (Win + L)
  sleep             Hardware ACPI sleep sequence
  combo [seq]       Interactive Windows combo studio (create, send, delete)
  combo list [--os <os>]  List saved combos, optionally filtered by target OS (--json supported)
  combo show <name> Print a saved combo and resolve its payload references
  combo fav <name>  Toggle favorite (pin with ❤️ to top of dashboard action list)
  combo schema      Grammar + every accepted token (JSON for AI agents)
  combo keys        Dump every key, modifier, consumer, alias and mouse token (--json);
                    with a sequence argument it sends that sequence like 'combo <seq>'
  combo validate <sequence> [--os windows|macos|linux] [--allow-unresolved]
                    Validate grammar and resolve payload references without sending
  combo save <name> <sequence...> [--os windows|macos|linux] [--emoji <e>] [--desc <d>] [--fav] [--allow-unresolved]
                    Save a named combo (refuses missing payload references by default)
  combo send <name> [--dry-run] Send a saved combo, or print resolved HID steps only
  combo delete <name> [-f]      Delete a combo (--json skips confirmation)
  combo <sequence>  Send a one-shot sequence (e.g. '{mod+d}', '{mod+s}'); add --dry-run
  combo mouse       Send a mouse gesture (click-left, scroll-up, double-click)
  click [button]    Click mouse at resting cursor position (left, right, middle, double)
                    (--double, --count <n>, --json; no pointer coordinates or movement)
  mouse <action>    Send mouse gesture or click (click-left, scroll-up, double-click)
  raw <hex>         Send raw HID report with auto-inferred Report ID (1B, 2B, 4B, 8B)
  sync [--pull|--push] Bi-directional sync with Android device (pull, push, restart app, reconnect)
  quick <text...>   Quick send keystrokes (default: type; use --raw for hex HID report)
  connect [addr]    Connect Bluetooth to target laptop MAC
  disconnect        Disconnect Bluetooth HID
  unpair            Unpair phone Bluetooth bond
  start             Start Android background service
  stop              Stop Android service
  restart           Restart Android service
  logs              Live stream Android Logcat
  filelog           Dump persisted file log`)
}
