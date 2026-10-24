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
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ghost-hid/ghid/internal/adb"
	"github.com/ghost-hid/ghid/internal/combos"
	"github.com/ghost-hid/ghid/internal/config"
	"github.com/ghost-hid/ghid/internal/gateway"
	"github.com/ghost-hid/ghid/internal/hid"
	"github.com/ghost-hid/ghid/internal/snippets"
	"github.com/ghost-hid/ghid/internal/tui"
	"github.com/ghost-hid/ghid/internal/vault"
	"github.com/mattn/go-runewidth"
)

// App manages command line execution and dependency lifecycle.
type App struct {
	Version        string
	ConfigStore    *config.Store
	ComboStore     *combos.Store
	SnippetStore   *snippets.Store
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
	cStore := combos.NewStore("")
	_, _ = cStore.EnsureDefaults()
	sStore := snippets.NewStore("", vlt)
	_, _ = sStore.EnsureDefaults()

	return &App{
		Version:      version,
		ConfigStore:  store,
		ComboStore:   cStore,
		SnippetStore: sStore,
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
			model := tui.NewModelWithStores(mgr, cfg, cStore, sStore)
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

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--json" {
			jsonOutput = true
		} else if arg == "-s" || arg == "--serial" {
			if i+1 < len(args) {
				serial = args[i+1]
				i++
			}
		} else if strings.HasPrefix(arg, "--serial=") {
			serial = strings.TrimPrefix(arg, "--serial=")
		} else {
			filteredArgs = append(filteredArgs, arg)
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
		return a.handleCombo(serial, cmdArgs, jsonOutput)

	case "quick", "q":
		return a.handleQuick(serial, cmdArgs, jsonOutput)

	case "snippets", "snippet":
		return a.handleSnippets(cmdArgs, jsonOutput)

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

	default:
		fmt.Fprintf(a.Stderr, "Unknown command '%s'. Run 'ghid --help' for usage.\n", cmd)
		return 1
	}
}

func (a *App) runInteractive(cfg config.Config, serial string) int {
	mgr, err := adb.NewManager(serial)
	if err != nil {
		fmt.Fprintf(a.Stderr, "Error: %v\n", err)
		return 2
	}

	if err := a.InteractiveFn(mgr, cfg); err != nil {
		fmt.Fprintf(a.Stderr, "TUI error: %v\n", err)
		return 1
	}
	return 0
}

func (a *App) handleStatus(cfg config.Config, serial string, jsonOutput bool) int {
	gw, err := a.GatewayFactory(serial)
	if err != nil {
		if jsonOutput {
			res, _ := json.Marshal(map[string]interface{}{"error": err.Error(), "connected": false})
			fmt.Fprintln(a.Stdout, string(res))
		} else {
			fmt.Fprintf(a.Stderr, "Error: %v\n", err)
		}
		return 1
	}

	status, err := gw.Status()
	if err != nil {
		if jsonOutput {
			res, _ := json.Marshal(map[string]interface{}{"error": err.Error(), "connected": false})
			fmt.Fprintln(a.Stdout, string(res))
		} else {
			fmt.Fprintf(a.Stderr, "Error: %v\n", err)
		}
		return 1
	}

	selDev := cfg.GetSelectedDevice()
	if jsonOutput {
		data, _ := json.MarshalIndent(status, "", "  ")
		fmt.Fprintln(a.Stdout, string(data))
	} else {
		fmt.Fprintln(a.Stdout, "GhostHID Gateway Status:")
		fmt.Fprintf(a.Stdout, "  SDP Registered: %v\n", status.Registered)
		fmt.Fprintf(a.Stdout, "  Connection:     %s\n", status.State)
		if selDev != nil {
			fmt.Fprintf(a.Stdout, "  Active Target:  %s [%s] (%s)\n", selDev.Name, selDev.Address, selDev.Platform)
		} else {
			fmt.Fprintf(a.Stdout, "  Target:         %s [%s]\n", status.Target, status.Address)
		}
		fmt.Fprintf(a.Stdout, "  Target OS:      %s\n", describeStatusOS(status))
		fmt.Fprintf(a.Stdout, "  Updated:        %s\n", status.Updated)
	}

	if !status.Connected {
		return 1
	}
	return 0
}

// describeStatusOS renders the fingerprint result for humans.
func describeStatusOS(status adb.GatewayStatus) string {
	osName := strings.ToLower(strings.TrimSpace(status.OS))
	if osName == "" || osName == "unknown" {
		return "Unknown (waiting for a connected host)"
	}
	description := combos.OSLabel(osName)
	if status.OSConfidence != "" {
		description += " (" + status.OSConfidence + " confidence"
		if status.DetectionMethod != "" {
			description += " via " + status.DetectionMethod
		}
		description += ")"
	}
	return description
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

	snipCount := 0
	if sStore := a.snippetStore(); sStore != nil {
		if list, _ := sStore.List(); len(list) > 0 {
			snipCount = len(list)
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
			"snippets_count":      snipCount,
		}
		data, _ := json.MarshalIndent(reportMap, "", "  ")
		fmt.Fprintln(a.Stdout, string(data))
		return 0
	}

	fmt.Fprintln(a.Stdout, "\n=== GhostHID Doctor Health Check ===")
	for _, chk := range report.Checks {
		a.printCheck(chk.Name, chk.Pass, chk.Detail)
	}
	a.printCheck("Snippets Subsystem", snipCount > 0, fmt.Sprintf("%d snippets loaded", snipCount))

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

	// Resolve from SnippetStore first
	sStore := a.snippetStore()
	var userFromSnip, passFromSnip string
	if sStore != nil {
		if u, ok, _ := sStore.Get("username"); ok {
			userFromSnip = u.Value
		}
		if p, ok, _ := sStore.Get("password"); ok {
			passFromSnip = p.Value
		}
	}

	if username == "" {
		username = userFromSnip
	}
	if username == "" {
		username = cfg.DefaultUser
	}
	if password == "" {
		password = passFromSnip
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
				"message": "Username and password snippets are empty. Please set them using: ghid snippets set username <val> and ghid snippets set password <val> --secure",
			})
			fmt.Fprintln(a.Stdout, string(res))
		} else {
			fmt.Fprintln(a.Stderr, "✖ Error: Username and password snippets are empty.")
			fmt.Fprintln(a.Stderr, "  Please set your snippet values:")
			fmt.Fprintln(a.Stderr, "    ghid snippets set username <your-username>")
			fmt.Fprintln(a.Stderr, "    ghid snippets set password <your-password> --secure")
			fmt.Fprintln(a.Stderr, "  Or pass them directly: ghid login -u <user> -p <pass>")
		}
		return 1
	}

	if *saveFlag {
		if sStore != nil {
			_, _ = sStore.Set("username", username, false)
			_, _ = sStore.Set("password", password, true)
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
				"message": "Target laptop is not connected via Bluetooth",
				"target":  status.Target,
				"state":   status.State,
			})
			fmt.Fprintln(a.Stdout, string(res))
		} else {
			fmt.Fprintf(a.Stderr, "✖ Error: Target laptop is not connected via Bluetooth (State: %s, Target: %s).\n", status.State, status.Target)
			fmt.Fprintf(a.Stderr, "  Please ensure laptop is awake and Bluetooth is paired/connected.\n")
			fmt.Fprintf(a.Stderr, "  Run 'ghid doctor' to verify hardware links.\n")
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
	lockKeys := combos.DefaultLockKeys
	if c, ok := a.comboStore().Find("Lock"); ok && c.Keys != "" {
		lockKeys = c.Keys
	}
	return a.dispatchCombo(serial, "Lock", lockKeys, jsonOutput)
}

func (a *App) handleSleep(serial string, args []string, jsonOutput bool) int {
	sleepKeys := combos.DefaultSleepKeys
	if c, ok := a.comboStore().Find("Sleep"); ok && c.Keys != "" {
		sleepKeys = c.Keys
	}
	return a.dispatchCombo(serial, "Sleep", sleepKeys, jsonOutput)
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
	if !isRaw && strings.HasPrefix(strings.ToLower(payload), "raw:") {
		isRaw = true
		payload = strings.TrimSpace(payload[4:])
	} else if strings.HasPrefix(strings.ToLower(payload), "type:") {
		isRaw = false
		payload = strings.TrimSpace(payload[5:])
	}

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
		if len(cleanHex) != 16 {
			if jsonOutput {
				res, _ := json.Marshal(map[string]interface{}{"status": "error", "error": "raw HID report must be exactly 16 hexadecimal characters (8 bytes)"})
				fmt.Fprintln(a.Stdout, string(res))
			} else {
				fmt.Fprintf(a.Stderr, "Error: Raw HID report must be exactly 16 hexadecimal characters (8 bytes), got %d: %q\n", len(cleanHex), cleanHex)
			}
			return 1
		}
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
			res, _ := json.Marshal(map[string]interface{}{"status": "success", "mode": "raw", "raw": cleanHex})
			fmt.Fprintln(a.Stdout, string(res))
		} else {
			fmt.Fprintf(a.Stdout, "✔ Dispatched raw HID report: %s\n", cleanHex)
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

func (a *App) handleCombo(serial string, args []string, jsonOutput bool) int {
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
		if jsonOutput {
			writeJSONIndent(a.Stdout, map[string]interface{}{
				"status":   "success",
				"name":     c.Name,
				"favorite": c.Favorite,
			})
		} else {
			status := "unpinned"
			if c.Favorite {
				status = "pinned to favorites (❤️)"
			}
			fmt.Fprintf(a.Stdout, "✔ Combo %q is now %s\n", c.Name, status)
		}
		return 0

	case "schema", "tokens", "capabilities":
		return a.handleComboSchema(store, jsonOutput)

	case "validate", "check":
		if len(args) < 2 {
			return a.comboFail(jsonOutput, "usage: ghid combo validate <keys...>")
		}
		return a.handleComboValidate(strings.Join(args[1:], " "), jsonOutput)

	case "save", "add", "set", "edit":
		var positional []string
		isFav := false
		osName := ""
		description := ""

		for i := 1; i < len(args); i++ {
			arg := args[i]
			switch {
			case arg == "--fav" || arg == "--favorite" || arg == "-f":
				isFav = true
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
			return a.comboFail(jsonOutput, "usage: ghid combo save <name> <keys...> [--os windows|macos|linux] [--desc <d>] [--fav]")
		}
		name := positional[0]
		keys := strings.Join(positional[1:], " ")
		normalizedOS, err := combos.NormalizeOS(osName)
		if err != nil {
			return a.comboFail(jsonOutput, "%v", err)
		}
		_, existed := store.Find(name)
		list, err := store.Add(combos.Combo{
			Name:        name,
			Keys:        keys,
			OS:          normalizedOS,
			Description: description,
			Favorite:    isFav,
		})
		if err != nil {
			return a.comboFail(jsonOutput, "%v", err)
		}
		if jsonOutput {
			saved := combos.Combo{Name: name, Keys: keys, OS: normalizedOS, Description: description, Favorite: isFav}
			for _, c := range list {
				if strings.EqualFold(c.Name, name) {
					saved = c
					break
				}
			}
			writeJSONIndent(a.Stdout, map[string]interface{}{
				"status":  "saved",
				"updated": existed,
				"combo":   saved,
			})
		} else {
			favMsg := ""
			if isFav {
				favMsg = " [pinned ❤️]"
			}
			osMsg := "[" + combos.OSLabel(normalizedOS) + "] "
			fmt.Fprintf(a.Stdout, "✔ Saved combo %s%q (%s)%s\n", osMsg, name, keys, favMsg)
		}
		return 0

	case "send", "run":
		if len(args) < 2 {
			return a.comboFail(jsonOutput, "usage: ghid combo send <name>")
		}
		name := strings.Join(args[1:], " ")
		combo, ok := store.Find(name)
		if !ok {
			return a.comboFail(jsonOutput, "no saved combo named %q (run 'ghid combo list --json')", name)
		}
		return a.dispatchCombo(serial, combo.Name, combo.Keys, jsonOutput)

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
		if jsonOutput {
			res, _ := json.Marshal(map[string]interface{}{"status": "deleted", "name": name})
			fmt.Fprintln(a.Stdout, string(res))
		} else {
			fmt.Fprintf(a.Stdout, "✔ Deleted combo %q.\n", name)
		}
		return 0

	case "mouse":
		if len(args) < 2 {
			return a.comboFail(jsonOutput, "usage: ghid combo mouse <click-left|click-right|click-middle|double-click|scroll-up|scroll-down>")
		}
		return a.dispatchMouse(serial, args[1], jsonOutput)

	case "keys":
		if len(args) < 2 {
			return a.comboFail(jsonOutput, "usage: ghid combo keys <token...>")
		}
		return a.dispatchCombo(serial, "", strings.Join(args[1:], " "), jsonOutput)
	}

	// Backwards-compatible one-shot send: ghid combo "win+d".
	keys := strings.Join(args, " ")
	return a.dispatchCombo(serial, "", keys, jsonOutput)
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
	schema := map[string]interface{}{
		"windows_only": true,
		"storage":      store.Path,
		"grammar":      "step ->{seconds} step ->{seconds} step",
		"step_types": []string{
			"keyboard chord: ctrl+shift+esc",
			"mouse gesture: click-left, scroll-up",
			"variable injection: $USERNAME, $PASSWORD, ${PIN}",
			"typed text: type:rustdesk or type:\"rust desk\"",
			"event directive: !EXIT, !DISCONNECT, !CONNECT, !RECONNECT, !START, !STOP, !RESTART, !UNPAIR, !COMBO:SEND:lock",
		},
		"keyboard":      hid.WindowsKeyboardTokens(),
		"mouse":         hid.MouseActions(),
		"directives":    hid.Directives(),
		"text_prefixes": []string{"type:", "text:", "type ", "text "},
		"os_tags":       combos.OSTokens(),
		"default_os":    combos.OSWindows,
		"os_note":       "The win token is the HID GUI modifier: Windows/Linux Super, macOS Command.",
		"example":       "ctrl+alt+del ->{1.0} esc ->{0.8} $USERNAME ->{0.8} tab ->{0.8} $PASSWORD ->{0.8} enter",
		"commands": map[string]string{
			"list":     "ghid combo list --json",
			"fav":      "ghid combo fav <name> --json",
			"save":     "ghid combo save \"Name\" \"keys\" [--os windows|macos|linux] [--desc <d>] [--fav] --json",
			"send":     "ghid combo send \"Name\" --json",
			"delete":   "ghid combo delete \"Name\" --json",
			"validate": "ghid combo validate \"keys\" --json",
		},
	}

	if jsonOutput {
		writeJSONIndent(a.Stdout, schema)
		return 0
	}

	fmt.Fprintln(a.Stdout, "\n🎹 COMBO GRAMMAR · Windows keyboard + mouse + text + variables")
	fmt.Fprintf(a.Stdout, "  Storage: %s\n", store.Path)
	fmt.Fprintln(a.Stdout, "  Chain:   step ->{seconds} step")
	fmt.Fprintf(a.Stdout, "  Example: %s\n\n", schema["example"])
	fmt.Fprintln(a.Stdout, "  Variables: $USERNAME, $PASSWORD (or ${NAME})")
	fmt.Fprintln(a.Stdout, "  Keyboard: ", strings.Join(hid.WindowsKeyboardTokens(), " "))
	fmt.Fprintln(a.Stdout, "  Mouse:    ", strings.Join(hid.MouseActions(), " "))
	fmt.Fprintln(a.Stdout, "  Text:      type:rustdesk  or  type:\"rust desk\"")
	fmt.Fprintln(a.Stdout, "  OS tags:   ", strings.Join(combos.OSTokens(), " "))
	return 0
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
				return "", rest, errors.New("--os requires windows, macos or linux")
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

func (a *App) handleComboValidate(keys string, jsonOutput bool) int {
	steps, err := hid.DescribeSequence(keys)
	if err != nil {
		if jsonOutput {
			writeJSONIndent(a.Stdout, map[string]interface{}{
				"valid": false,
				"keys":  keys,
				"error": err.Error(),
			})
		} else {
			fmt.Fprintf(a.Stderr, "✖ Invalid combo: %v\n", err)
		}
		return 1
	}

	// Apply the same Windows-only and ASCII rules used when saving.
	if err := combos.Validate(keys); err != nil {
		if jsonOutput {
			writeJSONIndent(a.Stdout, map[string]interface{}{
				"valid": false,
				"keys":  keys,
				"error": err.Error(),
			})
		} else {
			fmt.Fprintf(a.Stderr, "✖ Invalid combo: %v\n", err)
		}
		return 1
	}

	if jsonOutput {
		writeJSONIndent(a.Stdout, map[string]interface{}{
			"valid": true,
			"keys":  keys,
			"steps": steps,
		})
	} else {
		fmt.Fprintf(a.Stdout, "✔ Valid Windows combo (%d steps): %s\n", len(steps), keys)
	}
	return 0
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
	return combos.NewStore(filepath.Join(dir, "combos.json"))
}

// snippetStore returns the configured snippet store or defaults one.
func (a *App) snippetStore() *snippets.Store {
	if a.SnippetStore != nil {
		return a.SnippetStore
	}
	dir := filepath.Dir(config.GetConfigPath())
	if a.ConfigStore != nil && a.ConfigStore.Path != "" {
		dir = filepath.Dir(a.ConfigStore.Path)
	}
	return snippets.NewStore(filepath.Join(dir, "snippets.json"), a.Vault)
}

func (a *App) snippetResolver() hid.VariableResolver {
	return a.snippetStore().Resolver()
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
		return gw.ComboWithResolver(combo.Keys, a.snippetResolver())
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

	title := "🎹 COMBO STUDIO · Windows keyboard + mouse + variables"
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
		fmt.Fprintln(a.Stdout, "    ghid combo save \"Open Run\" \"win+r\"")
		fmt.Fprintln(a.Stdout)
		return 0
	}
	for i, c := range list {
		favIcon := "  "
		if c.Favorite {
			favIcon = "❤️ "
		}
		osTag := "[" + combos.OSShortTag(c.OS) + "]"
		nameWithOS := osTag + " " + c.Name
		nameWidth := runewidth.StringWidth(nameWithOS)
		namePad := ""
		if nameWidth < 26 {
			namePad = strings.Repeat(" ", 26-nameWidth)
		}
		descStr := ""
		if c.Description != "" {
			descStr = fmt.Sprintf(" (%s)", c.Description)
		}
		fmt.Fprintf(a.Stdout, "  %2d. %s%s%s %-26s%s\n", i+1, favIcon, nameWithOS, namePad, c.Keys, descStr)
	}
	fmt.Fprintf(a.Stdout, "\n  Send one with 'ghid combo send <name>' or filter with 'ghid combo list --os macos'\n\n")
	return 0
}

func (a *App) handleSnippets(args []string, jsonOutput bool) int {
	store := a.snippetStore()
	if len(args) == 0 {
		args = []string{"list"}
	}

	subcmd := args[0]
	switch subcmd {
	case "list", "ls":
		list, err := store.List()
		if err != nil {
			fmt.Fprintf(a.Stderr, "Error: %v\n", err)
			return 1
		}
		if jsonOutput {
			writeJSONIndent(a.Stdout, list)
			return 0
		}
		fmt.Fprintln(a.Stdout, "\n📋 SNIPPETS LIBRARY · Variables & Texts")
		if len(list) == 0 {
			fmt.Fprintln(a.Stdout, "  No snippets found.")
			return 0
		}
		fmt.Fprintf(a.Stdout, "  %-3s %-20s %-26s %s\n", "#", "VARIABLE", "VALUE", "SECURITY")
		for i, s := range list {
			varName := "$" + strings.ToUpper(s.Name)
			displayVal := s.MaskedValue()
			sec := "[PLAIN]"
			if s.Secure {
				sec = "[KEYCHAIN 🔒]"
			}
			fmt.Fprintf(a.Stdout, "  %2d. %-20s %-26s %s\n", i+1, varName, displayVal, sec)
		}
		fmt.Fprintln(a.Stdout)
		return 0

	case "get":
		if len(args) < 2 {
			fmt.Fprintf(a.Stderr, "Usage: ghid snippets get <name>\n")
			return 1
		}
		name := args[1]
		sn, ok, err := store.Get(name)
		if err != nil {
			fmt.Fprintf(a.Stderr, "Error: %v\n", err)
			return 1
		}
		if !ok {
			fmt.Fprintf(a.Stderr, "Snippet %q not found.\n", name)
			return 1
		}
		if jsonOutput {
			writeJSONIndent(a.Stdout, sn)
		} else {
			fmt.Fprintf(a.Stdout, "%s\n", sn.Value)
		}
		return 0

	case "set":
		if len(args) < 3 {
			fmt.Fprintf(a.Stderr, "Usage: ghid snippets set <name> <value> [--secure]\n")
			return 1
		}
		name := args[1]
		secure := false
		var valParts []string
		for _, arg := range args[2:] {
			if arg == "--secure" || arg == "-s" {
				secure = true
			} else {
				valParts = append(valParts, arg)
			}
		}
		val := strings.Join(valParts, " ")
		sn, err := store.Set(name, val, secure)
		if err != nil {
			fmt.Fprintf(a.Stderr, "Error: %v\n", err)
			return 1
		}
		if jsonOutput {
			writeJSONIndent(a.Stdout, sn)
		} else {
			status := "plain text"
			if secure {
				status = "secure (stored in Keychain)"
			}
			fmt.Fprintf(a.Stdout, "✔ Snippet $%s saved (%s)\n", strings.ToUpper(sn.Name), status)
		}
		return 0

	case "delete", "rm":
		if len(args) < 2 {
			fmt.Fprintf(a.Stderr, "Usage: ghid snippets delete <name>\n")
			return 1
		}
		name := args[1]
		if strings.EqualFold(name, "username") || strings.EqualFold(name, "password") {
			fmt.Fprintf(a.Stderr, "Error: Default snippet %q cannot be deleted (clear value using 'ghid snippets set %s \"\"' instead)\n", name, name)
			return 1
		}
		_, deleted, err := store.Delete(name)
		if err != nil {
			fmt.Fprintf(a.Stderr, "Error: %v\n", err)
			return 1
		}
		if !deleted {
			fmt.Fprintf(a.Stderr, "Snippet %q not found.\n", name)
			return 1
		}
		if jsonOutput {
			writeJSONIndent(a.Stdout, map[string]interface{}{"status": "deleted", "name": name})
		} else {
			fmt.Fprintf(a.Stdout, "✔ Deleted snippet %q\n", name)
		}
		return 0

	default:
		fmt.Fprintf(a.Stderr, "Unknown snippets subcommand %q. Usage: ghid snippets [list|get|set|delete]\n", subcmd)
		return 1
	}
}

// writeComboJSON keeps "->" readable instead of escaping it to -\u003e.
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

func (a *App) dispatchCombo(serial, name, keys string, jsonOutput bool) int {
	gw, err := a.GatewayFactory(serial)
	if err != nil {
		return a.comboFail(jsonOutput, "%v", err)
	}

	depth := 0
	var handleDirective func(directive string) (bool, error)
	handleDirective = func(directive string) (bool, error) {
		upper := strings.ToUpper(strings.TrimSpace(directive))
		if strings.HasPrefix(upper, "!COMBO:SEND:") {
			targetName := strings.TrimSpace(directive[len("!COMBO:SEND:"):])
			if depth >= 5 {
				return true, fmt.Errorf("maximum combo nesting depth exceeded at %q", targetName)
			}
			targetCombo, found := a.comboStore().Find(targetName)
			if !found {
				return true, fmt.Errorf("nested combo %q not found", targetName)
			}
			depth++
			defer func() { depth-- }()
			return true, gw.ComboWithResolver(targetCombo.Keys, a.snippetResolver())
		}
		return false, nil
	}
	gw.SetDirectiveHandler(handleDirective)

	err = gw.ComboWithResolver(keys, a.snippetResolver())
	if err != nil {
		if errors.Is(err, hid.ErrExitRequested) {
			if jsonOutput {
				payload := map[string]interface{}{"status": "success", "exit": true, "combo": keys}
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
			} else if strings.EqualFold(name, "Sleep") {
				fmt.Fprintln(a.Stdout, "✔ Windows host put to sleep. Android gateway stopped.")
			} else {
				fmt.Fprintln(a.Stdout, "✔ Sequence completed and terminated via !EXIT")
			}
			return 0
		}
		return a.comboFail(jsonOutput, "combo failed: %v", err)
	}
	if jsonOutput {
		payload := map[string]interface{}{"status": "success", "combo": keys}
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
		fmt.Fprintln(a.Stdout, "✔ Windows host put to sleep. Android gateway stopped.")
	} else if name != "" {
		fmt.Fprintf(a.Stdout, "✔ Dispatched %q: %s\n", name, keys)
	} else {
		fmt.Fprintf(a.Stdout, "✔ Dispatched combo: %s\n", keys)
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
	fmt.Fprintln(a.Stdout, "Building GhostHID APK (./gradlew assembleDebug)...")
	cmd := exec.Command("./gradlew", "assembleDebug")
	cmd.Stdout = a.Stdout
	cmd.Stderr = a.Stderr
	err := cmd.Run()
	if err != nil {
		fmt.Fprintf(a.Stderr, "Build failed: %v\n", err)
		return 1
	}
	fmt.Fprintln(a.Stdout, "✔ Build successful: app/build/outputs/apk/debug/app-debug.apk")
	return 0
}

func (a *App) handleInstallApk(serial string, args []string) int {
	apkPath := ""
	if len(args) > 0 {
		apkPath = args[0]
	} else {
		candidates := []string{
			"build/ghost-hid.apk",
			"ghost-hid.apk",
			"app/build/outputs/apk/debug/app-debug.apk",
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

func (a *App) printHelp() {
	fmt.Fprintln(a.Stdout, `GhostHID - Headless Android Bluetooth HID Keyboard Controller (Go/Charm TUI)

Usage:
  ghid [command] [options]

Commands:
  interactive (i)   Launch animated 24-bit Lip Gloss interactive dashboard (default)
  status            Query gateway & Bluetooth connection status (--json supported)
  doctor            Comprehensive hardware & network diagnostic health check
  devices           Manage registered target devices (list, add, select, delete)
  snippets          Manage text & secret variables ($USERNAME, $PASSWORD, custom)
  snippets list     List all snippets with masked values (--json supported)
  snippets get <name> Get snippet value
  snippets set <name> <value> [--secure] Save or update snippet
  snippets delete <name> Delete a snippet
  login             Automated Windows unlock & credential injection
  lock              Instant Windows lock screen (Win + L)
  sleep             Windows sleep sequence + countdown exit
  combo [keys]      Interactive Windows combo studio (create, send, delete)
  combo list [--os <os>]  List saved combos, optionally filtered by target OS (--json supported)
  combo fav <name>  Toggle favorite (pin with ❤️ to top of dashboard action list)
  combo schema      Grammar + every accepted token (JSON for AI agents)
  combo validate <keys>  Validate without saving or sending
  combo save <name> <keys...> [--os windows|macos|linux] [--desc <d>] [--fav]   Save a named combo (keyboard + mouse + text + $VAR)
  combo send <name> Send a saved combo
  combo delete <name> [-f]      Delete a combo (--json skips confirmation)
  combo <keys>      Send a one-shot shortcut (e.g. 'win+d', 'ctrl+alt+del')
  combo mouse       Send a mouse gesture (click-left, scroll-up, double-click)
  quick <text...>   Quick send keystrokes (default: type; use --raw for 16-hex HID report)
  connect [addr]    Connect Bluetooth to target laptop MAC
  disconnect        Disconnect Bluetooth HID
  unpair            Unpair phone Bluetooth bond
  start             Start Android background service
  stop              Stop Android service
  restart           Restart Android service
  logs              Live stream Android Logcat
  filelog           Dump persisted file log
  build             Assemble Android debug APK
  install-apk       Install APK via adb
  install-bin       Install ghid binary into ~/.local/bin and manpage
  uninstall-bin     Uninstall ghid binary and manpage
  man               Open manual page
  version (-v)      Print version

Global Flags:
  -s, --serial <id> Explicit Android target ADB serial
  --json            Structured JSON output for scripts & AI agents
  -h, --help        Show this help text`)
}
