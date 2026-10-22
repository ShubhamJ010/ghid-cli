package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ghost-hid/ghid/internal/adb"
	"github.com/ghost-hid/ghid/internal/config"
	"github.com/ghost-hid/ghid/internal/keychain"
	"github.com/ghost-hid/ghid/internal/tui"
)

const Version = "1.0.0"

func main() {
	cfg := config.LoadConfig()

	// Peek for global flags or help
	args := os.Args[1:]
	if len(args) == 0 {
		runInteractive(cfg, "")
		return
	}

	var jsonOutput bool
	var serial string

	// Filter common flags
	var filteredArgs []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--json" {
			jsonOutput = true
		} else if a == "-s" || a == "--serial" {
			if i+1 < len(args) {
				serial = args[i+1]
				i++
			}
		} else if strings.HasPrefix(a, "--serial=") {
			serial = strings.TrimPrefix(a, "--serial=")
		} else {
			filteredArgs = append(filteredArgs, a)
		}
	}

	if len(filteredArgs) == 0 {
		runInteractive(cfg, serial)
		return
	}

	cmd := filteredArgs[0]
	cmdArgs := filteredArgs[1:]

	switch cmd {
	case "interactive", "i":
		runInteractive(cfg, serial)

	case "version", "-v", "--version":
		fmt.Printf("GhostHID ghid %s (Go/Charm TUI)\n", Version)

	case "help", "-h", "--help":
		printHelp()

	case "status":
		handleStatus(cfg, serial, jsonOutput)

	case "doctor":
		handleDoctor(cfg, serial, jsonOutput)

	case "login":
		handleLogin(cfg, serial, cmdArgs, jsonOutput)

	case "lock":
		handleLock(serial, jsonOutput)

	case "sleep":
		handleSleep(serial, cmdArgs, jsonOutput)

	case "type":
		handleType(serial, cmdArgs, jsonOutput)

	case "combo":
		handleCombo(serial, cmdArgs, jsonOutput)

	case "raw":
		handleRaw(serial, cmdArgs, jsonOutput)

	case "creds":
		handleCreds(cfg, cmdArgs, jsonOutput)

	case "devices", "device":
		handleDevices(cfg, serial, cmdArgs, jsonOutput)

	case "start":
		handleService("start", serial, jsonOutput)

	case "stop":
		handleService("stop", serial, jsonOutput)

	case "restart":
		handleService("restart", serial, jsonOutput)

	case "connect":
		handleConnect(cfg, serial, cmdArgs, jsonOutput)

	case "disconnect":
		handleDisconnect(serial, jsonOutput)

	case "unpair":
		handleUnpair(serial, jsonOutput)

	case "logs":
		handleLogs(serial, cmdArgs)

	case "filelog":
		handleFileLog(serial)

	case "build":
		handleBuild()

	case "install-apk":
		handleInstallApk(serial, cmdArgs)

	case "install-bin":
		handleInstallBin()

	case "uninstall-bin":
		handleUninstallBin()

	case "man":
		handleMan()

	default:
		fmt.Fprintf(os.Stderr, "Unknown command '%s'. Run 'ghid --help' for usage.\n", cmd)
		os.Exit(1)
	}
}

func getManager(serial string) (*adb.Manager, error) {
	mgr, err := adb.NewManager(serial)
	if err != nil {
		return nil, err
	}
	_, err = mgr.SelectDevice()
	if err != nil {
		return nil, err
	}
	return mgr, nil
}

func runInteractive(cfg config.Config, serial string) {
	mgr, err := adb.NewManager(serial)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(2)
	}

	model := tui.NewModel(mgr, cfg)
	p := tea.NewProgram(model, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "TUI error: %v\n", err)
		os.Exit(1)
	}
}

func handleStatus(cfg config.Config, serial string, jsonOutput bool) {
	mgr, err := getManager(serial)
	if err != nil {
		if jsonOutput {
			res, _ := json.Marshal(map[string]interface{}{"error": err.Error(), "connected": false})
			fmt.Println(string(res))
		} else {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		}
		os.Exit(1)
	}

	status := mgr.GetStatus()
	selDev := cfg.GetSelectedDevice()
	if jsonOutput {
		data, _ := json.MarshalIndent(status, "", "  ")
		fmt.Println(string(data))
	} else {
		fmt.Println("GhostHID Gateway Status:")
		fmt.Printf("  SDP Registered: %v\n", status.Registered)
		fmt.Printf("  Connection:     %s\n", status.State)
		if selDev != nil {
			fmt.Printf("  Active Target:  %s [%s] (%s)\n", selDev.Name, selDev.Address, selDev.Platform)
		} else {
			fmt.Printf("  Target:         %s [%s]\n", status.Target, status.Address)
		}
		fmt.Printf("  Updated:        %s\n", status.Updated)
	}

	if !status.Connected {
		os.Exit(1)
	}
}

func handleDoctor(cfg config.Config, serial string, jsonOutput bool) {
	mgr, err := adb.NewManager(serial)
	if err != nil {
		if jsonOutput {
			res, _ := json.Marshal(map[string]interface{}{"error": err.Error()})
			fmt.Println(string(res))
		} else {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		}
		os.Exit(2)
	}

	dev, devErr := mgr.SelectDevice()
	pkgInstalled := mgr.IsPackageInstalled()
	btOn := mgr.GetBluetoothState()
	status := mgr.GetStatus()
	keychainAvail := keychain.IsAvailable()
	selDev := cfg.GetSelectedDevice()

	hasCachedPass := false
	if cfg.DefaultUser != "" && keychainAvail {
		if _, err := keychain.GetPassword(cfg.DefaultUser); err == nil {
			hasCachedPass = true
		}
	}

	if jsonOutput {
		report := map[string]interface{}{
			"version":             Version,
			"adb_path":            mgr.AdbPath,
			"adb_available":       true,
			"device_connected":    dev != nil && devErr == nil,
			"device_model":        mgr.GetDeviceModel(),
			"device_serial":       mgr.Serial,
			"apk_installed":       pkgInstalled,
			"bluetooth_on":        btOn,
			"gateway_registered":  status.Registered,
			"gateway_connected":   status.Connected,
			"target_laptop":       status.Target,
			"target_address":      status.Address,
			"selected_device":     selDev,
			"registered_devices":  len(cfg.Devices),
			"keychain_available":  keychainAvail,
			"cached_user":         cfg.DefaultUser,
			"has_cached_password": hasCachedPass,
		}
		data, _ := json.MarshalIndent(report, "", "  ")
		fmt.Println(string(data))
		return
	}

	fmt.Println("\n=== GhostHID Doctor Health Check ===")
	printCheck("ADB Executable", true, mgr.AdbPath)
	printCheck("Android Device", devErr == nil, fmt.Sprintf("%s [%s]", mgr.GetDeviceModel(), mgr.Serial))
	printCheck("GhostHID APK", pkgInstalled, adb.PackageName)
	printCheck("Phone Bluetooth State", btOn, "Enabled")
	printCheck("Bluetooth HID Profile (SDP)", status.Registered, "Registered")
	printCheck("Target Laptop Connection", status.Connected, fmt.Sprintf("%s [%s]", status.Target, status.Address))

	targetDesc := "None"
	if selDev != nil {
		targetDesc = fmt.Sprintf("%s [%s] (%s)", selDev.Name, selDev.Address, selDev.Platform)
	}
	printCheck("Configured Active Target", selDev != nil, targetDesc)
	printCheck("macOS Keychain Vault", keychainAvail, "Service 'ghost-hid'")

	userDesc := "None"
	if cfg.DefaultUser != "" {
		userDesc = cfg.DefaultUser
	}
	printCheck("Cached Windows Credentials", hasCachedPass, userDesc)

	if status.Connected {
		fmt.Println("\nAll systems operational! GhostHID is ready.")
	} else {
		fmt.Println("\nNote: Target laptop is not currently connected via Bluetooth.")
	}
}

func printCheck(name string, pass bool, detail string) {
	if pass {
		fmt.Printf("  \033[32m✔\033[0m %-28s %s\n", name, detail)
	} else {
		fmt.Printf("  \033[31m✖\033[0m %-28s %s\n", name, detail)
	}
}

func handleDevices(cfg config.Config, serial string, args []string, jsonOutput bool) {
	if len(args) == 0 {
		args = []string{"list"}
	}
	subcmd := args[0]
	switch subcmd {
	case "list":
		if jsonOutput {
			data, _ := json.MarshalIndent(cfg.Devices, "", "  ")
			fmt.Println(string(data))
		} else {
			fmt.Println("\n=== Registered Target Devices ===")
			if len(cfg.Devices) == 0 {
				fmt.Println("  No devices registered yet. Run 'ghid devices add' or 'ghid' to register one.")
			}
			for _, d := range cfg.Devices {
				sel := " "
				if d.Selected {
					sel = "● [ACTIVE]"
				}
				osIcon := "🪟"
				if d.Platform == "macos" {
					osIcon = "🍏"
				}
				fmt.Printf("  %s %-10s %-20s %-20s %s\n", osIcon, fmt.Sprintf("[%s]", d.Platform), d.Name, d.Address, sel)
			}
			fmt.Println()
		}

	case "add":
		fs := flag.NewFlagSet("devices add", flag.ExitOnError)
		name := fs.String("name", "", "Device friendly name")
		addr := fs.String("addr", "", "Bluetooth MAC address")
		platform := fs.String("os", "windows", "Target OS: windows or macos")
		selectNow := fs.Bool("select", true, "Set as active target immediately")
		_ = fs.Parse(args[1:])

		if *name == "" || *addr == "" {
			fmt.Fprintf(os.Stderr, "Usage: ghid devices add --name <name> --addr <mac> [--os windows|macos] [--select]\n")
			os.Exit(1)
		}

		newDev := cfg.AddDevice(*name, *addr, *platform, *selectNow)
		if jsonOutput {
			data, _ := json.Marshal(newDev)
			fmt.Println(string(data))
		} else {
			fmt.Printf("✔ Registered device '%s' [%s] (%s).\n", newDev.Name, newDev.Address, newDev.Platform)
		}

		if *selectNow {
			mgr, err := getManager(serial)
			if err == nil {
				_ = mgr.ConnectTarget(newDev.Address)
			}
		}

	case "select":
		if len(args) < 2 {
			fmt.Fprintf(os.Stderr, "Usage: ghid devices select <id_or_addr_or_name>\n")
			os.Exit(1)
		}
		target := args[1]
		if cfg.SelectDevice(target) {
			dev := cfg.GetSelectedDevice()
			if jsonOutput {
				data, _ := json.Marshal(dev)
				fmt.Println(string(data))
			} else {
				fmt.Printf("✔ Selected active target '%s' [%s].\n", dev.Name, dev.Address)
			}
			mgr, err := getManager(serial)
			if err == nil {
				_ = mgr.ConnectTarget(dev.Address)
			}
		} else {
			fmt.Fprintf(os.Stderr, "Error: Device '%s' not found.\n", target)
			os.Exit(1)
		}

	case "delete", "rm", "unregister":
		if len(args) < 2 {
			fmt.Fprintf(os.Stderr, "Usage: ghid devices delete <id_or_addr_or_name> [-f]\n")
			os.Exit(1)
		}
		target := args[1]
		force := false
		for _, a := range args[2:] {
			if a == "-f" || a == "--force" {
				force = true
			}
		}

		if !force {
			fmt.Printf("Are you sure you want to unregister '%s'? [y/N]: ", target)
			var resp string
			_, _ = fmt.Scanln(&resp)
			resp = strings.ToLower(strings.TrimSpace(resp))
			if resp != "y" && resp != "yes" {
				fmt.Println("Aborted.")
				return
			}
		}

		deleted, ok := cfg.DeleteDevice(target)
		if ok && deleted != nil {
			if jsonOutput {
				fmt.Printf(`{"status": "deleted", "name": "%s", "address": "%s"}`+"\n", deleted.Name, deleted.Address)
			} else {
				fmt.Printf("✔ Unregistered device '%s' [%s].\n", deleted.Name, deleted.Address)
			}
		} else {
			fmt.Fprintf(os.Stderr, "Error: Device '%s' not found.\n", target)
			os.Exit(1)
		}

	default:
		fmt.Fprintf(os.Stderr, "Unknown devices subcommand '%s'. Usage: ghid devices [list|add|select|delete]\n", subcmd)
		os.Exit(1)
	}
}

func handleLogin(cfg config.Config, serial string, args []string, jsonOutput bool) {
	fs := flag.NewFlagSet("login", flag.ExitOnError)
	userFlag := fs.String("u", "", "Username")
	passFlag := fs.String("p", "", "Password")
	saveFlag := fs.Bool("save", false, "Save to Keychain")
	_ = fs.Parse(args)

	username := *userFlag
	password := *passFlag

	if username == "" {
		username = cfg.DefaultUser
	}

	if password == "" && username != "" && keychain.IsAvailable() {
		p, err := keychain.GetPassword(username)
		if err == nil {
			password = p
		}
	}

	if username == "" || password == "" {
		fmt.Fprintf(os.Stderr, "Error: Username and Password required. Provide -u and -p or save them first with 'ghid creds set'.\n")
		os.Exit(1)
	}

	if *saveFlag && keychain.IsAvailable() {
		_ = keychain.SetPassword(username, password)
		cfg.DefaultUser = username
		_ = config.SaveConfig(cfg)
	}

	mgr, err := getManager(serial)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	status := mgr.GetStatus()
	if !status.Connected {
		if jsonOutput {
			res, _ := json.Marshal(map[string]interface{}{
				"status": "error",
				"message": "Target laptop is not connected via Bluetooth",
				"target": status.Target,
				"state": status.State,
			})
			fmt.Println(string(res))
		} else {
			fmt.Fprintf(os.Stderr, "✖ Error: Target laptop is not connected via Bluetooth (State: %s, Target: %s).\n", status.State, status.Target)
			fmt.Fprintf(os.Stderr, "  Please ensure laptop is awake and Bluetooth is paired/connected.\n")
			fmt.Fprintf(os.Stderr, "  Run 'ghid doctor' to verify hardware links.\n")
		}
		os.Exit(1)
	}

	if !jsonOutput {
		fmt.Printf("⚡ Initiating Windows wake & login sequence for user '%s'...\n", username)
	}

	delays := map[string]float64{
		"wake":    cfg.Delays.Wake,
		"overlay": cfg.Delays.Overlay,
		"user":    cfg.Delays.User,
		"tab":     cfg.Delays.Tab,
		"pass":    cfg.Delays.Pass,
		"submit":  cfg.Delays.Submit,
	}

	err = mgr.ExecuteLogin(username, password, delays, func(step int, desc string) {
		if !jsonOutput {
			fmt.Printf("  [%d/6] %s\n", step, desc)
		}
	})

	if err != nil {
		if jsonOutput {
			res, _ := json.Marshal(map[string]interface{}{"status": "error", "message": err.Error()})
			fmt.Println(string(res))
		} else {
			fmt.Fprintf(os.Stderr, "Login sequence failed: %v\n", err)
		}
		os.Exit(1)
	}

	if jsonOutput {
		res, _ := json.Marshal(map[string]interface{}{"status": "success", "user": username})
		fmt.Println(string(res))
	} else {
		fmt.Println("✔ Login sequence completed successfully!")
	}
}

func handleLock(serial string, jsonOutput bool) {
	mgr, err := getManager(serial)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	err = mgr.SendLock()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Lock failed: %v\n", err)
		os.Exit(1)
	}
	if jsonOutput {
		fmt.Println(`{"status": "success", "action": "lock"}`)
	} else {
		fmt.Println("✔ Windows lock command dispatched (Win + L)")
	}
}

func handleSleep(serial string, args []string, jsonOutput bool) {
	countdown := 5
	for i := 0; i < len(args); i++ {
		if args[i] == "--countdown" && i+1 < len(args) {
			_, _ = fmt.Sscanf(args[i+1], "%d", &countdown)
			i++
		}
	}

	mgr, err := getManager(serial)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if !jsonOutput {
		fmt.Println("🌙 Sending Windows sleep sequence (Win+X -> u -> s)...")
	}
	_ = mgr.SendSleepSequence()

	if !jsonOutput {
		fmt.Printf("Clean exit in %d seconds (disconnecting Bluetooth & stopping service)...\n", countdown)
		for c := countdown; c > 0; c-- {
			fmt.Printf("\r  Stopping in %d... ", c)
			time.Sleep(1 * time.Second)
		}
		fmt.Println("\r✔ Gateway stopped cleanly. Zero background battery drain.")
	}

	_ = mgr.DisconnectTarget()
	_ = mgr.StopGateway()

	if jsonOutput {
		fmt.Println(`{"status": "success", "action": "sleep"}`)
	}
}

func handleType(serial string, args []string, jsonOutput bool) {
	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "Usage: ghid type <text...>\n")
		os.Exit(1)
	}
	text := strings.Join(args, " ")
	mgr, err := getManager(serial)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	err = mgr.TypeText(text)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Type failed: %v\n", err)
		os.Exit(1)
	}
	if jsonOutput {
		res, _ := json.Marshal(map[string]interface{}{"status": "success", "typed": text})
		fmt.Println(string(res))
	} else {
		fmt.Printf("✔ Typed text (%d characters)\n", len(text))
	}
}

func handleCombo(serial string, args []string, jsonOutput bool) {
	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "Usage: ghid combo <key1+key2...>\n")
		os.Exit(1)
	}
	keys := args[0]
	mgr, err := getManager(serial)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	err = mgr.SendCombo(keys)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Combo failed: %v\n", err)
		os.Exit(1)
	}
	if jsonOutput {
		res, _ := json.Marshal(map[string]interface{}{"status": "success", "combo": keys})
		fmt.Println(string(res))
	} else {
		fmt.Printf("✔ Dispatched combo: %s\n", keys)
	}
}

func handleRaw(serial string, args []string, jsonOutput bool) {
	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "Usage: ghid raw <16-hex-chars>\n")
		os.Exit(1)
	}
	hexStr := args[0]
	mgr, err := getManager(serial)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	err = mgr.SendRaw(hexStr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Raw report failed: %v\n", err)
		os.Exit(1)
	}
	if jsonOutput {
		res, _ := json.Marshal(map[string]interface{}{"status": "success", "raw": hexStr})
		fmt.Println(string(res))
	} else {
		fmt.Printf("✔ Dispatched raw HID report: %s\n", hexStr)
	}
}

func handleCreds(cfg config.Config, args []string, jsonOutput bool) {
	if len(args) == 0 {
		args = []string{"status"}
	}
	action := args[0]

	switch action {
	case "status":
		hasPass := false
		if cfg.DefaultUser != "" && keychain.IsAvailable() {
			if _, err := keychain.GetPassword(cfg.DefaultUser); err == nil {
				hasPass = true
			}
		}
		if jsonOutput {
			res, _ := json.Marshal(map[string]interface{}{
				"default_user":          cfg.DefaultUser,
				"has_keychain_password": hasPass,
			})
			fmt.Println(string(res))
		} else {
			user := cfg.DefaultUser
			if user == "" {
				user = "(none)"
			}
			fmt.Printf("Stored User: %s\nKeychain Password: %v\n", user, hasPass)
		}

	case "set":
		fs := flag.NewFlagSet("creds set", flag.ExitOnError)
		user := fs.String("u", "", "Username")
		pass := fs.String("p", "", "Password")
		_ = fs.Parse(args[1:])

		if *user == "" || *pass == "" {
			fmt.Fprintf(os.Stderr, "Usage: ghid creds set -u <user> -p <pass>\n")
			os.Exit(1)
		}
		if keychain.IsAvailable() {
			_ = keychain.SetPassword(*user, *pass)
		}
		cfg.DefaultUser = *user
		_ = config.SaveConfig(cfg)
		fmt.Printf("✔ Credentials saved for '%s'.\n", *user)

	case "clear":
		if cfg.DefaultUser != "" && keychain.IsAvailable() {
			_ = keychain.DeletePassword(cfg.DefaultUser)
		}
		cfg.DefaultUser = ""
		_ = config.SaveConfig(cfg)
		fmt.Println("✔ Cleared stored credentials.")
	}
}

func handleService(action, serial string, jsonOutput bool) {
	mgr, err := adb.NewManager(serial)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	switch action {
	case "start":
		_ = mgr.StartGateway()
		fmt.Println("✔ GhostHID Foreground Service started.")
	case "stop":
		_ = mgr.StopGateway()
		fmt.Println("✔ GhostHID Foreground Service stopped.")
	case "restart":
		_ = mgr.StopGateway()
		time.Sleep(500 * time.Millisecond)
		_ = mgr.StartGateway()
		fmt.Println("✔ GhostHID Foreground Service restarted.")
	}
}

func handleConnect(cfg config.Config, serial string, args []string, jsonOutput bool) {
	target := cfg.TargetAddress
	if len(args) > 0 {
		target = args[0]
	}
	mgr, err := getManager(serial)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	_ = mgr.ConnectTarget(target)
	fmt.Printf("✔ Bluetooth connection request sent for %s\n", target)
}

func handleDisconnect(serial string, jsonOutput bool) {
	mgr, err := getManager(serial)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	_ = mgr.DisconnectTarget()
	fmt.Println("✔ Disconnected from target laptop.")
}

func handleUnpair(serial string, jsonOutput bool) {
	mgr, err := getManager(serial)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	_ = mgr.UnpairTarget()
	fmt.Println("✔ Unpaired target laptop.")
}

func handleLogs(serial string, args []string) {
	mgr, err := getManager(serial)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	logs, err := mgr.GetLogs(40)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(logs)
}

func handleFileLog(serial string) {
	mgr, err := getManager(serial)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	log, _ := mgr.GetFileLog()
	fmt.Println(log)
}

func handleBuild() {
	mgr, err := adb.NewManager("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("🔨 Compiling GhostHID APK from source...")
	err = mgr.BuildApk()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Build failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("✔ Build complete! APK generated at build/ghost-hid.apk")
}

func handleInstallApk(serial string, args []string) {
	var apkPath string
	if len(args) > 0 {
		apkPath = args[0]
	}
	mgr, err := getManager(serial)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("📲 Installing GhostHID APK onto device...")
	err = mgr.InstallApk(apkPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Install failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("✔ APK successfully installed!")
}

func handleInstallBin() {
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
		fmt.Fprintf(os.Stderr, "Failed to create symlink: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("✔ Created symlink: %s -> %s\n", symlinkPath, exePath)

	manDir := filepath.Join(home, ".local", "share", "man", "man1")
	_ = os.MkdirAll(manDir, 0755)
	cwd, _ := os.Getwd()
	manSrc := filepath.Join(cwd, "man", "ghid.1")
	manDest := filepath.Join(manDir, "ghid.1")
	if _, err := os.Stat(manSrc); err == nil {
		_ = os.Remove(manDest)
		_ = os.Symlink(manSrc, manDest)
		fmt.Printf("✔ Installed man page: %s -> %s\n", manDest, manSrc)
	}

	fmt.Println("\nGhostHID Go CLI is now installed as 'ghid'!")
}

func handleUninstallBin() {
	home, _ := os.UserHomeDir()
	symlinkPath := filepath.Join(home, ".local", "bin", "ghid")
	manDest := filepath.Join(home, ".local", "share", "man", "man1", "ghid.1")
	_ = os.Remove(symlinkPath)
	_ = os.Remove(manDest)
	fmt.Println("✔ Uninstalled ghid binary symlink and man page.")
}

func handleMan() {
	cmd := exec.Command("man", "ghid")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	_ = cmd.Run()
}

func printHelp() {
	fmt.Println(`GhostHID - Headless Android Bluetooth HID Keyboard Controller (Go/Charm TUI)

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
  combo <keys>      Send shortcut combination (e.g. 'win+d', 'ctrl+alt+del')
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
