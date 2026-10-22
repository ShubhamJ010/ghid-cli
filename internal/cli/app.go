package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ghost-hid/ghid/internal/adb"
	"github.com/ghost-hid/ghid/internal/combos"
	"github.com/ghost-hid/ghid/internal/config"
	"github.com/ghost-hid/ghid/internal/gateway"
	"github.com/ghost-hid/ghid/internal/hid"
	"github.com/ghost-hid/ghid/internal/tui"
	"github.com/ghost-hid/ghid/internal/vault"
)

// App manages command line execution and dependency lifecycle.
type App struct {
	Version        string
	ConfigStore    *config.Store
	ComboStore     *combos.Store
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
	return &App{
		Version:     version,
		ConfigStore: store,
		ComboStore:  combos.NewStore(""),
		Vault:       vlt,
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
			model := tui.NewModel(mgr, cfg)
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

	case "type":
		return a.handleType(serial, cmdArgs, jsonOutput)

	case "combo":
		return a.handleCombo(serial, cmdArgs, jsonOutput)

	case "raw":
		return a.handleRaw(serial, cmdArgs, jsonOutput)

	case "creds":
		return a.handleCreds(cfg, cmdArgs, jsonOutput)

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
		fmt.Fprintf(a.Stdout, "  Updated:        %s\n", status.Updated)
	}

	if !status.Connected {
		return 1
	}
	return 0
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

	if jsonOutput {
		reportMap := map[string]interface{}{
			"version":             report.Version,
			"adb_path":            report.AdbPath,
			"adb_available":       report.AdbAvailable,
			"device_connected":    report.DeviceConnected,
			"device_model":        report.DeviceModel,
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
		}
		data, _ := json.MarshalIndent(reportMap, "", "  ")
		fmt.Fprintln(a.Stdout, string(data))
		return 0
	}

	fmt.Fprintln(a.Stdout, "\n=== GhostHID Doctor Health Check ===")
	for _, chk := range report.Checks {
		a.printCheck(chk.Name, chk.Pass, chk.Detail)
	}

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
	saveFlag := fs.Bool("save", false, "Save to Keychain")
	fs.SetOutput(a.Stderr)
	if err := fs.Parse(args); err != nil {
		return 1
	}

	username := *userFlag
	password := *passFlag

	if username == "" {
		username = cfg.DefaultUser
	}

	if password == "" && username != "" && a.Vault != nil && a.Vault.IsAvailable() {
		p, err := a.Vault.Get(username)
		if err == nil {
			password = p
		}
	}

	if username == "" || password == "" {
		fmt.Fprintf(a.Stderr, "Error: Username and Password required. Provide -u and -p or save them first with 'ghid creds set'.\n")
		return 1
	}

	if *saveFlag && a.Vault != nil && a.Vault.IsAvailable() {
		_ = a.Vault.Set(username, password)
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
	gw, err := a.GatewayFactory(serial)
	if err != nil {
		fmt.Fprintf(a.Stderr, "Error: %v\n", err)
		return 1
	}
	err = gw.Lock()
	if err != nil {
		fmt.Fprintf(a.Stderr, "Lock failed: %v\n", err)
		return 1
	}
	if jsonOutput {
		fmt.Fprintln(a.Stdout, `{"status": "success", "action": "lock"}`)
	} else {
		fmt.Fprintln(a.Stdout, "✔ Windows lock command dispatched (Win + L)")
	}
	return 0
}

func (a *App) handleSleep(serial string, args []string, jsonOutput bool) int {
	countdown := 5
	for i := 0; i < len(args); i++ {
		if args[i] == "--countdown" && i+1 < len(args) {
			_, _ = fmt.Sscanf(args[i+1], "%d", &countdown)
			i++
		}
	}

	gw, err := a.GatewayFactory(serial)
	if err != nil {
		fmt.Fprintf(a.Stderr, "Error: %v\n", err)
		return 1
	}

	if !jsonOutput {
		fmt.Fprintln(a.Stdout, "🌙 Sending Windows sleep sequence (Esc -> Esc -> Win+X -> u -> s)...")
	}
	_ = gw.SleepSequence()

	if !jsonOutput {
		fmt.Fprintf(a.Stdout, "Clean exit in %d seconds (disconnecting Bluetooth & stopping service)...\n", countdown)
		for c := countdown; c > 0; c-- {
			fmt.Fprintf(a.Stdout, "\r  Stopping in %d... ", c)
			time.Sleep(1 * time.Second)
		}
		fmt.Fprintln(a.Stdout, "\r✔ Gateway stopped cleanly. Zero background battery drain.")
	}

	_ = gw.Disconnect()
	_ = gw.StopService()

	if jsonOutput {
		fmt.Fprintln(a.Stdout, `{"status": "success", "action": "sleep"}`)
	}
	return 0
}

func (a *App) handleType(serial string, args []string, jsonOutput bool) int {
	if len(args) == 0 {
		fmt.Fprintf(a.Stderr, "Usage: ghid type <text...>\n")
		return 1
	}
	text := strings.Join(args, " ")
	gw, err := a.GatewayFactory(serial)
	if err != nil {
		fmt.Fprintf(a.Stderr, "Error: %v\n", err)
		return 1
	}
	err = gw.Type(text)
	if err != nil {
		fmt.Fprintf(a.Stderr, "Type failed: %v\n", err)
		return 1
	}
	if jsonOutput {
		res, _ := json.Marshal(map[string]interface{}{"status": "success", "typed": text})
		fmt.Fprintln(a.Stdout, string(res))
	} else {
		fmt.Fprintf(a.Stdout, "✔ Typed text (%d characters)\n", len(text))
	}
	return 0
}

func (a *App) handleCombo(serial string, args []string, jsonOutput bool) int {
	store := a.comboStore()

	// Bare `ghid combo` opens the studio so creating and executing combos is a
	// single command. Without a terminal, fall back to listing the library.
	if len(args) == 0 {
		if !stdinIsTerminal() {
			return a.listCombos(store, jsonOutput)
		}
		return a.openComboStudio(store, serial)
	}

	switch args[0] {
	case "studio", "new", "ui", "interactive":
		if !stdinIsTerminal() {
			return a.listCombos(store, jsonOutput)
		}
		return a.openComboStudio(store, serial)

	case "list", "ls":
		return a.listCombos(store, jsonOutput)

	case "schema", "tokens", "capabilities":
		return a.handleComboSchema(store, jsonOutput)

	case "validate", "check":
		if len(args) < 2 {
			return a.comboFail(jsonOutput, "usage: ghid combo validate <keys...>")
		}
		return a.handleComboValidate(strings.Join(args[1:], " "), jsonOutput)

	case "save", "add":
		if len(args) < 3 {
			return a.comboFail(jsonOutput, "usage: ghid combo save <name> <keys...> (example: ghid combo save \"Open Run\" \"win+r\")")
		}
		name := args[1]
		keys := strings.Join(args[2:], " ")
		_, existed := store.Find(name)
		list, err := store.Add(combos.Combo{Name: name, Keys: keys})
		if err != nil {
			return a.comboFail(jsonOutput, "%v", err)
		}
		if jsonOutput {
			saved := combos.Combo{Name: name, Keys: keys}
			for _, c := range list {
				if strings.EqualFold(c.Name, name) {
					saved = c
					break
				}
			}
			writeComboJSON(a.Stdout, map[string]interface{}{
				"status":  "saved",
				"updated": existed,
				"combo":   saved,
			})
		} else {
			fmt.Fprintf(a.Stdout, "✔ Saved combo %q (%s)\n", name, keys)
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
		writeComboJSON(a.Stdout, map[string]interface{}{
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
			"typed text: type:rustdesk or type:\"rust desk\"",
		},
		"keyboard":      hid.WindowsKeyboardTokens(),
		"mouse":         hid.MouseActions(),
		"text_prefixes": []string{"type:", "text:", "type ", "text "},
		"example":       "win+s ->{0.7} type:rustdesk ->{0.4} enter",
		"commands": map[string]string{
			"list":     "ghid combo list --json",
			"save":     "ghid combo save \"Name\" \"keys\" --json",
			"send":     "ghid combo send \"Name\" --json",
			"delete":   "ghid combo delete \"Name\" --json",
			"validate": "ghid combo validate \"keys\" --json",
		},
	}

	if jsonOutput {
		writeComboJSON(a.Stdout, schema)
		return 0
	}

	fmt.Fprintln(a.Stdout, "\n🎹 COMBO GRAMMAR · Windows keyboard + mouse + text")
	fmt.Fprintf(a.Stdout, "  Storage: %s\n", store.Path)
	fmt.Fprintln(a.Stdout, "  Chain:   step ->{seconds} step")
	fmt.Fprintf(a.Stdout, "  Example: %s\n\n", schema["example"])
	fmt.Fprintln(a.Stdout, "  Keyboard:", strings.Join(hid.WindowsKeyboardTokens(), " "))
	fmt.Fprintln(a.Stdout, "  Mouse:   ", strings.Join(hid.MouseActions(), " "))
	fmt.Fprintln(a.Stdout, "  Text:     type:rustdesk  or  type:\"rust desk\"")
	return 0
}

func (a *App) handleComboValidate(keys string, jsonOutput bool) int {
	steps, err := hid.DescribeSequence(keys)
	if err != nil {
		if jsonOutput {
			writeComboJSON(a.Stdout, map[string]interface{}{
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
			writeComboJSON(a.Stdout, map[string]interface{}{
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
		writeComboJSON(a.Stdout, map[string]interface{}{
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

func stdinIsTerminal() bool {
	info, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

func (a *App) openComboStudio(store *combos.Store, serial string) int {
	if a.ComboStudioFn == nil {
		return a.listCombos(store, false)
	}
	send := func(combo combos.Combo) error {
		gw, err := a.GatewayFactory(serial)
		if err != nil {
			return err
		}
		return gw.Combo(combo.Keys)
	}
	if err := a.ComboStudioFn(store, send); err != nil {
		fmt.Fprintf(a.Stderr, "Combo studio error: %v\n", err)
		return 1
	}
	return 0
}

func (a *App) listCombos(store *combos.Store, jsonOutput bool) int {
	list, err := store.Load()
	if err != nil {
		fmt.Fprintf(a.Stderr, "Error: %v\n", err)
		return 1
	}
	if jsonOutput {
		writeComboJSON(a.Stdout, list)
		return 0
	}

	fmt.Fprintln(a.Stdout, "\n🎹 COMBO STUDIO · Windows keyboard + mouse")
	if len(list) == 0 {
		fmt.Fprintln(a.Stdout, "  No combos saved yet.")
		fmt.Fprintln(a.Stdout, "  Create one with 'ghid combo' (interactive) or:")
		fmt.Fprintln(a.Stdout, "    ghid combo save \"Open Run\" \"win+r\"")
		fmt.Fprintln(a.Stdout)
		return 0
	}
	for i, c := range list {
		fmt.Fprintf(a.Stdout, "  %2d. %-28s %s\n", i+1, c.Name, c.Keys)
	}
	fmt.Fprintf(a.Stdout, "\n  Send one with 'ghid combo send <name>' or open the studio: ghid combo\n\n")
	return 0
}

// writeComboJSON keeps "->" readable instead of escaping it to -\u003e.
func writeComboJSON(w io.Writer, value interface{}) {
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
	err = gw.Combo(keys)
	if err != nil {
		return a.comboFail(jsonOutput, "combo failed: %v", err)
	}
	if jsonOutput {
		payload := map[string]interface{}{"status": "success", "combo": keys}
		if name != "" {
			payload["name"] = name
		}
		res, _ := json.Marshal(payload)
		fmt.Fprintln(a.Stdout, string(res))
	} else if name != "" {
		fmt.Fprintf(a.Stdout, "✔ Dispatched %q: %s\n", name, keys)
	} else {
		fmt.Fprintf(a.Stdout, "✔ Dispatched combo: %s\n", keys)
	}
	return 0
}

func (a *App) handleRaw(serial string, args []string, jsonOutput bool) int {
	if len(args) == 0 {
		fmt.Fprintf(a.Stderr, "Usage: ghid raw <16-hex-chars>\n")
		return 1
	}
	hexStr := args[0]
	gw, err := a.GatewayFactory(serial)
	if err != nil {
		fmt.Fprintf(a.Stderr, "Error: %v\n", err)
		return 1
	}
	err = gw.Raw(hexStr)
	if err != nil {
		fmt.Fprintf(a.Stderr, "Raw report failed: %v\n", err)
		return 1
	}
	if jsonOutput {
		res, _ := json.Marshal(map[string]interface{}{"status": "success", "raw": hexStr})
		fmt.Fprintln(a.Stdout, string(res))
	} else {
		fmt.Fprintf(a.Stdout, "✔ Dispatched raw HID report: %s\n", hexStr)
	}
	return 0
}

func (a *App) handleCreds(cfg config.Config, args []string, jsonOutput bool) int {
	if len(args) == 0 {
		args = []string{"status"}
	}
	action := args[0]

	switch action {
	case "status":
		hasPass := false
		if cfg.DefaultUser != "" && a.Vault != nil && a.Vault.IsAvailable() {
			if _, err := a.Vault.Get(cfg.DefaultUser); err == nil {
				hasPass = true
			}
		}
		if jsonOutput {
			res, _ := json.Marshal(map[string]interface{}{
				"default_user":          cfg.DefaultUser,
				"has_keychain_password": hasPass,
			})
			fmt.Fprintln(a.Stdout, string(res))
		} else {
			user := cfg.DefaultUser
			if user == "" {
				user = "(none)"
			}
			fmt.Fprintf(a.Stdout, "Stored User: %s\nKeychain Password: %v\n", user, hasPass)
		}
		return 0

	case "set":
		fs := flag.NewFlagSet("creds set", flag.ContinueOnError)
		user := fs.String("u", "", "Username")
		pass := fs.String("p", "", "Password")
		fs.SetOutput(a.Stderr)
		if err := fs.Parse(args[1:]); err != nil {
			return 1
		}

		if *user == "" || *pass == "" {
			fmt.Fprintf(a.Stderr, "Usage: ghid creds set -u <user> -p <pass>\n")
			return 1
		}
		if a.Vault != nil && a.Vault.IsAvailable() {
			_ = a.Vault.Set(*user, *pass)
		}
		cfg.DefaultUser = *user
		_ = a.ConfigStore.Save(cfg)
		fmt.Fprintf(a.Stdout, "✔ Credentials saved for '%s'.\n", *user)
		return 0

	case "clear":
		if cfg.DefaultUser != "" && a.Vault != nil && a.Vault.IsAvailable() {
			_ = a.Vault.Delete(cfg.DefaultUser)
		}
		cfg.DefaultUser = ""
		_ = a.ConfigStore.Save(cfg)
		fmt.Fprintln(a.Stdout, "✔ Cleared stored credentials.")
		return 0

	default:
		fmt.Fprintf(a.Stderr, "Unknown creds action: %s\n", action)
		return 1
	}
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
					sel = "● [ACTIVE]"
				}
				osIcon := "🪟"
				if d.Platform == "macos" {
					osIcon = "🍎"
				}
				fmt.Fprintf(a.Stdout, "  %s %-10s %-20s %-20s %s\n", osIcon, fmt.Sprintf("[%s]", d.Platform), d.Name, d.Address, sel)
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
		fmt.Fprintf(a.Stderr, "Unknown devices subcommand '%s'. Usage: ghid devices [list|add|select|delete]\n", subcmd)
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
	gw, err := a.GatewayFactory("")
	if err != nil {
		fmt.Fprintf(a.Stderr, "Error: %v\n", err)
		return 1
	}
	fmt.Fprintln(a.Stdout, "🔨 Compiling GhostHID APK from source...")
	err = gw.BuildApk()
	if err != nil {
		fmt.Fprintf(a.Stderr, "Build failed: %v\n", err)
		return 1
	}
	fmt.Fprintln(a.Stdout, "✔ Build complete! APK generated at build/ghost-hid.apk")
	return 0
}

func (a *App) handleInstallApk(serial string, args []string) int {
	var apkPath string
	if len(args) > 0 {
		apkPath = args[0]
	}
	gw, err := a.GatewayFactory(serial)
	if err != nil {
		fmt.Fprintf(a.Stderr, "Error: %v\n", err)
		return 1
	}
	fmt.Fprintln(a.Stdout, "📲 Installing GhostHID APK onto device...")
	err = gw.InstallApk(apkPath)
	if err != nil {
		fmt.Fprintf(a.Stderr, "Install failed: %v\n", err)
		return 1
	}
	fmt.Fprintln(a.Stdout, "✔ APK successfully installed!")
	return 0
}

func (a *App) handleInstallBin() int {
	home, _ := os.UserHomeDir()
	binDir := filepath.Join(home, ".local", "bin")
	_ = os.MkdirAll(binDir, 0755)

	exePath, err := os.Executable()
	if err != nil {
		exePath, _ = filepath.Abs(os.Args[0])
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
  login             Automated Windows unlock & credential injection
  lock              Instant Windows lock screen (Win + L)
  sleep             Windows sleep sequence + countdown exit
  type <text...>    Type text string through virtual keyboard
  combo [keys]      Interactive Windows combo studio (create, send, delete)
  combo list        List saved combos (--json supported)
  combo schema      Grammar + every accepted token (JSON for AI agents)
  combo validate <keys>  Validate without saving or sending
  combo save <name> <keys...>   Save a named combo (keyboard + mouse + text)
  combo send <name> Send a saved combo
  combo delete <name> [-f]      Delete a combo (--json skips confirmation)
  combo <keys>      Send a one-shot shortcut (e.g. 'win+d', 'ctrl+alt+del')
  combo mouse       Send a mouse gesture (click-left, scroll-up, double-click)
  raw <hex16>       Transmit raw 8-byte HID keyboard report
  creds             Manage stored credentials in macOS Keychain (status, set, clear)
  connect [addr]    Connect Bluetooth HID to target laptop
  disconnect        Disconnect Bluetooth HID
  unpair            Unpair Bluetooth bonding
  start             Start Android foreground service
  stop              Stop Android foreground service
  restart           Restart Android foreground service
  logs              View recent Logcat diagnostic logs
  filelog           Read persistent gateway.log from device
  build             Compile APK from source
  install-apk       Install APK onto connected phone
  install-bin       Symlink ghid into ~/.local/bin and install man page
  uninstall-bin     Remove ghid symlink and man page
  man               Display formatted manual page

Options:
  -s, --serial      Target specific Android device serial or IP:PORT
  --json            Output machine-readable JSON format
  -v, --version     Show version number
  -h, --help        Show this help message`)
}
