package tui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ghost-hid/ghid/internal/adb"
	"github.com/ghost-hid/ghid/internal/combos"
	"github.com/ghost-hid/ghid/internal/config"
	"github.com/ghost-hid/ghid/internal/keychain"
)

type AppState int

const (
	StateInit AppState = iota
	StateMenu
	StateLoginPrompt
	StateLoginExecuting
	StateSleepCountdown
	StateTextInput
	StateComboInput
	StateRawInput
	StateCredsMenu
	StateDoctorView
	StateLogsView
	StateDeviceRecovery
	StateDevicesList
	StateDeviceRegisterPlatform
	StateDeviceRegisterGuide
	StateDeviceRegisterInput
	StateDeviceDeleteConfirm
	StateComboStudio
)

type MenuItem struct {
	Key         string
	Title       string
	Description string
	Icon        string
}

type DoctorCheck struct {
	Title string
	Pass  bool
	Info  string
}

type Model struct {
	manager        *adb.Manager
	cfg            config.Config
	state          AppState
	status         adb.GatewayStatus
	device         *adb.Device
	deviceModel    string
	menuIndex      int
	menuItems      []MenuItem
	spinner        spinner.Model
	progress       progress.Model
	sleepSeconds   int
	sleepTotal     int
	textInput      textinput.Model
	userInput      textinput.Model
	passInput      textinput.Model
	loginActiveIn  int // 0: user, 1: pass
	loginStep      int
	loginStepText  string
	notice         string
	isErrorNotice  bool
	doctorChecks   []DoctorCheck
	logContent     string
	width          int
	height         int
	quitting       bool
	lastPing       time.Time
	devicesIndex   int
	regPlatform    string // "windows" or "macos"
	devNameInput   textinput.Model
	devAddrInput   textinput.Model
	regActiveIn    int // 0: name, 1: addr
	deviceToDelete *config.TargetDevice
	comboStudio    *ComboStudioModel
}

type statusTickMsg time.Time
type sleepTickMsg time.Time
type loginStepMsg struct {
	step int
	text string
}
type loginDoneMsg error
type actionDoneMsg struct {
	notice  string
	isError bool
}

func NewModel(mgr *adb.Manager, cfg config.Config) Model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(ColorCyan)

	p := progress.New(
		progress.WithDefaultGradient(),
		progress.WithWidth(40),
	)

	ti := textinput.New()
	ti.Placeholder = "Enter text to type..."
	ti.Focus()
	ti.CharLimit = 256
	ti.Width = 40

	ui := textinput.New()
	ui.Placeholder = "Windows Username"
	ui.CharLimit = 64
	ui.Width = 32

	pi := textinput.New()
	pi.Placeholder = "Windows Password"
	pi.EchoMode = textinput.EchoPassword
	pi.EchoCharacter = '•'
	pi.CharLimit = 64
	pi.Width = 32

	dni := textinput.New()
	dni.Placeholder = "e.g. Work ThinkPad or MacBook Pro"
	dni.CharLimit = 64
	dni.Width = 36

	dai := textinput.New()
	dai.Placeholder = "XX:XX:XX:XX:XX:XX"
	dai.CharLimit = 17
	dai.Width = 36

	items := []MenuItem{
		{Key: "1", Title: "login", Description: "Automated Windows unlock & credential injection", Icon: "⚡"},
		{Key: "2", Title: "lock", Description: "Instant Windows lock screen (Win + L)", Icon: "🔒"},
		{Key: "3", Title: "sleep", Description: "Windows sleep sequence + countdown clean exit", Icon: "🌙"},
		{Key: "4", Title: "type", Description: "Type text string through virtual keyboard", Icon: "⌨️"},
		{Key: "5", Title: "combo", Description: "Create, send & delete Windows combos", Icon: "🎹"},
		{Key: "6", Title: "raw", Description: "Transmit raw 16-hex character HID report", Icon: "⚙️"},
		{Key: "7", Title: "creds", Description: "Inspect / update macOS Keychain credentials", Icon: "🔑"},
		{Key: "d", Title: "devices", Description: "Registered target devices & add new Windows/Mac", Icon: "💻"},
		{Key: "8", Title: "reconnect", Description: "Re-trigger Bluetooth HID connection", Icon: "🔄"},
		{Key: "9", Title: "doctor", Description: "Run full hardware & gateway diagnostic check", Icon: "🩺"},
		{Key: "0", Title: "exit", Description: "Disconnect Bluetooth & terminate phone service", Icon: "🚪"},
	}

	return Model{
		manager:      mgr,
		cfg:          cfg,
		state:        StateInit,
		menuItems:    items,
		spinner:      s,
		progress:     p,
		textInput:    ti,
		userInput:    ui,
		passInput:    pi,
		devNameInput: dni,
		devAddrInput: dai,
		sleepTotal:   5,
		sleepSeconds: 5,
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.spinner.Tick,
		m.initCheckCmd(),
	)
}

func (m Model) initCheckCmd() tea.Cmd {
	return func() tea.Msg {
		dev, err := m.manager.SelectDevice()
		if err != nil {
			return actionDoneMsg{notice: err.Error(), isError: true}
		}

		if !m.manager.GetBluetoothState() {
			_ = m.manager.EnableBluetooth()
		}

		_ = m.manager.StartGateway()

		target := m.cfg.TargetAddress
		if target != "" {
			_ = m.manager.ConnectTarget(target)
		}

		time.Sleep(800 * time.Millisecond)
		st := m.manager.GetStatus()
		return initSuccessMsg{dev: dev, status: st}
	}
}

type initSuccessMsg struct {
	dev    *adb.Device
	status adb.GatewayStatus
}

func (m Model) statusTick() tea.Cmd {
	return tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
		return statusTickMsg(t)
	})
}

func (m Model) sleepTick() tea.Cmd {
	return tea.Tick(1*time.Second, func(t time.Time) tea.Msg {
		return sleepTickMsg(t)
	})
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// The Combo Studio runs inside the main dashboard so menu key [5] feels
	// native: it renders full screen and returns here on esc.
	if m.state == StateComboStudio && m.comboStudio != nil {
		if key, ok := msg.(tea.KeyMsg); ok && key.String() == "ctrl+c" {
			m.quitting = true
			return m, tea.Quit
		}
		switch msg := msg.(type) {
		case tea.WindowSizeMsg:
			m.width = msg.Width
			m.height = msg.Height
		case statusTickMsg:
			m.lastPing = time.Time(msg)
			m.status = m.manager.GetStatus()
			return m, m.statusTick()
		}

		updated, cmd := m.comboStudio.Update(msg)
		studio := updated.(ComboStudioModel)
		m.comboStudio = &studio
		if studio.quitting {
			m.comboStudio = nil
			m.state = StateMenu
			return m, nil
		}
		return m, cmd
	}

	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.progress.Width = msg.Width - 20
		if m.progress.Width > 50 {
			m.progress.Width = 50
		}

	case spinner.TickMsg:
		var sCmd tea.Cmd
		m.spinner, sCmd = m.spinner.Update(msg)
		cmds = append(cmds, sCmd)

	case initSuccessMsg:
		m.device = msg.dev
		m.deviceModel = m.manager.GetDeviceModel()
		m.status = msg.status
		m.state = StateMenu
		cmds = append(cmds, m.statusTick())

	case statusTickMsg:
		m.lastPing = time.Time(msg)
		if m.state == StateMenu || m.state == StateDoctorView || m.state == StateDevicesList {
			m.status = m.manager.GetStatus()
		}
		if !m.quitting {
			cmds = append(cmds, m.statusTick())
		}

	case actionDoneMsg:
		m.notice = msg.notice
		m.isErrorNotice = msg.isError
		if m.state == StateInit && msg.isError {
			m.state = StateDeviceRecovery
		} else if m.state != StateDevicesList {
			m.state = StateMenu
		}

	case loginStepMsg:
		m.loginStep = msg.step
		m.loginStepText = msg.text

	case loginDoneMsg:
		if msg != nil {
			m.notice = fmt.Sprintf("Login failed: %v", msg)
			m.isErrorNotice = true
			m.state = StateMenu
			return m, nil
		}
		m.quitting = true
		return m, tea.Quit

	case sleepTickMsg:
		if m.state == StateSleepCountdown {
			m.sleepSeconds--
			if m.sleepSeconds <= 0 {
				_ = m.manager.DisconnectTarget()
				_ = m.manager.StopGateway()
				m.quitting = true
				return m, tea.Quit
			}
			cmds = append(cmds, m.sleepTick())
		}

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		case "esc":
			if m.state == StateDevicesList {
				m.state = StateMenu
				m.notice = ""
				return m, nil
			}
			if m.state == StateDeviceRegisterPlatform || m.state == StateDeviceDeleteConfirm {
				m.state = StateDevicesList
				return m, nil
			}
			if m.state == StateDeviceRegisterGuide {
				m.state = StateDeviceRegisterPlatform
				return m, nil
			}
			if m.state == StateDeviceRegisterInput {
				m.state = StateDeviceRegisterGuide
				return m, nil
			}
			if m.state == StateComboInput {
				if strings.TrimSpace(m.textInput.Value()) == "" {
					m.state = StateMenu
					m.notice = ""
					return m, nil
				}
				return m.updateComboInput(msg)
			}
			if m.state != StateMenu && m.state != StateInit && m.state != StateSleepCountdown {
				m.state = StateMenu
				m.notice = ""
				return m, nil
			}
		}

		switch m.state {
		case StateMenu:
			return m.updateMenu(msg)
		case StateTextInput, StateRawInput:
			return m.updateInput(msg)
		case StateComboInput:
			return m.updateComboInput(msg)
		case StateLoginPrompt:
			return m.updateLoginPrompt(msg)
		case StateCredsMenu:
			return m.updateCredsMenu(msg)
		case StateDevicesList:
			return m.updateDevicesList(msg)
		case StateDeviceRegisterPlatform:
			return m.updateRegisterPlatform(msg)
		case StateDeviceRegisterGuide:
			return m.updateRegisterGuide(msg)
		case StateDeviceRegisterInput:
			return m.updateRegisterInput(msg)
		case StateDeviceDeleteConfirm:
			return m.updateDeleteConfirm(msg)
		case StateDoctorView, StateLogsView, StateDeviceRecovery:
			if msg.String() == "enter" || msg.String() == "q" || msg.String() == "esc" {
				m.state = StateMenu
			}
		}
	}

	return m, tea.Batch(cmds...)
}

func (m Model) updateMenu(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if m.menuIndex > 0 {
			m.menuIndex--
		} else {
			m.menuIndex = len(m.menuItems) - 1
		}
	case "down", "j":
		if m.menuIndex < len(m.menuItems)-1 {
			m.menuIndex++
		} else {
			m.menuIndex = 0
		}
	case "1":
		return m.triggerAction("login")
	case "2":
		return m.triggerAction("lock")
	case "3":
		return m.triggerAction("sleep")
	case "4":
		return m.triggerAction("type")
	case "5":
		return m.triggerAction("combo")
	case "6":
		return m.triggerAction("raw")
	case "7":
		return m.triggerAction("creds")
	case "d", "D":
		return m.triggerAction("devices")
	case "8":
		return m.triggerAction("reconnect")
	case "9":
		return m.triggerAction("doctor")
	case "0", "q":
		return m.triggerAction("exit")
	case "enter":
		selected := m.menuItems[m.menuIndex]
		return m.triggerAction(selected.Title)
	}
	return m, nil
}

func (m Model) triggerAction(action string) (tea.Model, tea.Cmd) {
	m.notice = ""
	switch action {
	case "login":
		if !m.status.Connected {
			m.notice = "✖ Target laptop is not connected via Bluetooth. Run [8] reconnect or [9] doctor."
			m.isErrorNotice = true
			return m, nil
		}
		user := m.cfg.DefaultUser
		var pass string
		if user != "" && keychain.IsAvailable() {
			p, err := keychain.GetPassword(user)
			if err == nil {
				pass = p
			}
		}
		if user != "" && pass != "" {
			m.state = StateLoginExecuting
			m.loginStep = 0
			return m, m.startLoginCmd(user, pass)
		}
		m.state = StateLoginPrompt
		m.loginActiveIn = 0
		m.userInput.SetValue(user)
		m.passInput.SetValue("")
		m.userInput.Focus()
		return m, textinput.Blink

	case "lock":
		return m, func() tea.Msg {
			err := m.manager.SendLock()
			if err != nil {
				return actionDoneMsg{notice: fmt.Sprintf("Lock failed: %v", err), isError: true}
			}
			return actionDoneMsg{notice: "✔ Win + L lock command dispatched!", isError: false}
		}

	case "sleep":
		m.state = StateSleepCountdown
		m.sleepSeconds = 5
		m.sleepTotal = 5
		return m, tea.Batch(
			m.sleepTick(),
			func() tea.Msg {
				_ = m.manager.SendSleepSequence()
				return nil
			},
		)

	case "type":
		m.state = StateTextInput
		m.textInput.Placeholder = "Enter text to type..."
		m.textInput.SetValue("")
		m.textInput.Focus()
		return m, textinput.Blink

	case "combo":
		store := combos.NewStore("")
		studio := NewComboStudioModel(store, func(combo combos.Combo) error {
			if m.manager == nil {
				return fmt.Errorf("adb manager not initialized")
			}
			return m.manager.SendCombo(combo.Keys)
		})
		m.comboStudio = &studio
		m.state = StateComboStudio
		return m, studio.Init()

	case "raw":
		m.state = StateRawInput
		m.textInput.Placeholder = "16 hex chars e.g. 08000f0000000000"
		m.textInput.SetValue("")
		m.textInput.Focus()
		return m, textinput.Blink

	case "creds":
		m.state = StateCredsMenu
		return m, nil

	case "devices":
		m.state = StateDevicesList
		m.devicesIndex = 0
		return m, nil

	case "reconnect":
		return m, func() tea.Msg {
			_ = m.manager.StartGateway()
			time.Sleep(500 * time.Millisecond)
			err := m.manager.ConnectTarget(m.cfg.TargetAddress)
			if err != nil {
				return actionDoneMsg{notice: fmt.Sprintf("Reconnect failed: %v", err), isError: true}
			}
			time.Sleep(1 * time.Second)
			return actionDoneMsg{notice: "✔ Bluetooth connection signal sent.", isError: false}
		}

	case "doctor":
		m.state = StateDoctorView
		m.runDoctorChecks()
		return m, nil

	case "exit":
		_ = m.manager.DisconnectTarget()
		_ = m.manager.StopGateway()
		m.quitting = true
		return m, tea.Quit
	}
	return m, nil
}

func (m Model) updateDevicesList(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if m.devicesIndex > 0 {
			m.devicesIndex--
		} else if len(m.cfg.Devices) > 0 {
			m.devicesIndex = len(m.cfg.Devices) - 1
		}
	case "down", "j":
		if m.devicesIndex < len(m.cfg.Devices)-1 {
			m.devicesIndex++
		} else {
			m.devicesIndex = 0
		}
	case "enter", "s":
		if len(m.cfg.Devices) > 0 && m.devicesIndex < len(m.cfg.Devices) {
			dev := m.cfg.Devices[m.devicesIndex]
			m.cfg.SelectDevice(dev.ID)
			return m, func() tea.Msg {
				_ = m.manager.StartGateway()
				time.Sleep(400 * time.Millisecond)
				_ = m.manager.ConnectTarget(dev.Address)
				return actionDoneMsg{
					notice:  fmt.Sprintf("✔ Target switched to '%s' [%s]. Connecting...", dev.Name, dev.Address),
					isError: false,
				}
			}
		}
	case "a", "n":
		m.state = StateDeviceRegisterPlatform
		return m, nil
	case "d", "x":
		if len(m.cfg.Devices) > 0 && m.devicesIndex < len(m.cfg.Devices) {
			dev := m.cfg.Devices[m.devicesIndex]
			m.deviceToDelete = &dev
			m.state = StateDeviceDeleteConfirm
			return m, nil
		}
	case "u":
		return m, func() tea.Msg {
			_ = m.manager.UnpairTarget()
			return actionDoneMsg{notice: "✔ Dispatched unpair broadcast to phone.", isError: false}
		}
	case "esc", "b":
		m.state = StateMenu
		return m, nil
	}
	return m, nil
}

func (m Model) updateRegisterPlatform(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "1", "w", "W":
		m.regPlatform = "windows"
		m.state = StateDeviceRegisterGuide
		return m, nil
	case "2", "m", "M":
		m.regPlatform = "macos"
		m.state = StateDeviceRegisterGuide
		return m, nil
	case "esc", "b":
		m.state = StateDevicesList
		return m, nil
	}
	return m, nil
}

func (m Model) updateRegisterGuide(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter", "c", "C":
		m.state = StateDeviceRegisterInput
		m.regActiveIn = 0
		m.devNameInput.SetValue("")
		if m.regPlatform == "macos" {
			m.devNameInput.SetValue("MacBook")
		} else {
			m.devNameInput.SetValue("Windows PC")
		}

		// Prefill current detected target address if available
		st := m.manager.GetStatus()
		if st.Address != "" && st.Address != "none" {
			m.devAddrInput.SetValue(st.Address)
		} else {
			m.devAddrInput.SetValue("")
		}

		m.devNameInput.Focus()
		return m, textinput.Blink
	case "esc", "b":
		m.state = StateDeviceRegisterPlatform
		return m, nil
	}
	return m, nil
}

func (m Model) updateRegisterInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "tab", "down":
		if m.regActiveIn == 0 {
			m.regActiveIn = 1
			m.devNameInput.Blur()
			m.devAddrInput.Focus()
			return m, textinput.Blink
		} else {
			m.regActiveIn = 0
			m.devAddrInput.Blur()
			m.devNameInput.Focus()
			return m, textinput.Blink
		}
	case "up":
		if m.regActiveIn == 1 {
			m.regActiveIn = 0
			m.devAddrInput.Blur()
			m.devNameInput.Focus()
			return m, textinput.Blink
		}
	case "enter":
		name := strings.TrimSpace(m.devNameInput.Value())
		addr := strings.ToUpper(strings.TrimSpace(m.devAddrInput.Value()))
		if name == "" {
			name = "New Device"
		}
		if addr == "" {
			m.notice = "Bluetooth MAC address is required."
			m.isErrorNotice = true
			return m, nil
		}

		newDev := m.cfg.AddDevice(name, addr, m.regPlatform, true)
		m.state = StateDevicesList
		return m, func() tea.Msg {
			_ = m.manager.StartGateway()
			time.Sleep(500 * time.Millisecond)
			_ = m.manager.ConnectTarget(newDev.Address)
			return actionDoneMsg{
				notice:  fmt.Sprintf("✔ Registered and selected '%s' [%s]!", newDev.Name, newDev.Address),
				isError: false,
			}
		}
	case "esc", "b":
		m.state = StateDeviceRegisterGuide
		return m, nil
	}

	var cmd tea.Cmd
	if m.regActiveIn == 0 {
		m.devNameInput, cmd = m.devNameInput.Update(msg)
	} else {
		m.devAddrInput, cmd = m.devAddrInput.Update(msg)
	}
	return m, cmd
}

func (m Model) updateDeleteConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y":
		if m.deviceToDelete != nil {
			targetID := m.deviceToDelete.ID
			targetName := m.deviceToDelete.Name
			deleted, ok := m.cfg.DeleteDevice(targetID)
			m.deviceToDelete = nil
			m.state = StateDevicesList
			m.devicesIndex = 0
			if ok && deleted != nil {
				return m, func() tea.Msg {
					return actionDoneMsg{
						notice:  fmt.Sprintf("✔ Unregistered '%s' from configuration.", targetName),
						isError: false,
					}
				}
			}
		}
		m.state = StateDevicesList
		return m, nil
	case "n", "N", "esc":
		m.deviceToDelete = nil
		m.state = StateDevicesList
		return m, nil
	}
	return m, nil
}

func (m *Model) runDoctorChecks() {
	m.doctorChecks = []DoctorCheck{
		{Title: "ADB Executable", Pass: m.manager.AdbPath != "", Info: m.manager.AdbPath},
		{Title: "Android Device", Pass: m.device != nil, Info: fmt.Sprintf("%s [%s]", m.deviceModel, m.manager.Serial)},
		{Title: "GhostHID APK", Pass: m.manager.IsPackageInstalled(), Info: adb.PackageName},
		{Title: "Phone Bluetooth Radio", Pass: m.manager.GetBluetoothState(), Info: "Enabled"},
		{Title: "Bluetooth HID (SDP)", Pass: m.status.Registered, Info: "Profile Active"},
		{Title: "Target Laptop Link", Pass: m.status.Connected, Info: fmt.Sprintf("%s [%s]", m.status.Target, m.status.Address)},
		{Title: "macOS Keychain Vault", Pass: keychain.IsAvailable(), Info: "Service 'ghost-hid'"},
	}
}

func (m Model) startLoginCmd(user, pass string) tea.Cmd {
	return func() tea.Msg {
		delays := map[string]float64{
			"wake":    m.cfg.Delays.Wake,
			"overlay": m.cfg.Delays.Overlay,
			"user":    m.cfg.Delays.User,
			"tab":     m.cfg.Delays.Tab,
			"pass":    m.cfg.Delays.Pass,
			"submit":  m.cfg.Delays.Submit,
		}
		err := m.manager.ExecuteLogin(user, pass, delays, nil)
		return loginDoneMsg(err)
	}
}

func (m Model) updateInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		val := strings.TrimSpace(m.textInput.Value())
		if val == "" {
			m.state = StateMenu
			return m, nil
		}
		currentState := m.state
		m.state = StateMenu
		return m, func() tea.Msg {
			var err error
			var successMsg string
			switch currentState {
			case StateTextInput:
				err = m.manager.TypeText(val)
				successMsg = fmt.Sprintf("✔ Typed: %s", val)
			case StateComboInput:
				err = m.manager.SendCombo(val)
				successMsg = fmt.Sprintf("✔ Key combo dispatched: %s", val)
			case StateRawInput:
				err = m.manager.SendRaw(val)
				successMsg = fmt.Sprintf("✔ Raw report sent: %s", val)
			}
			if err != nil {
				return actionDoneMsg{notice: fmt.Sprintf("Error: %v", err), isError: true}
			}
			return actionDoneMsg{notice: successMsg, isError: false}
		}
	}
	var cmd tea.Cmd
	m.textInput, cmd = m.textInput.Update(msg)
	return m, cmd
}

func appendComboKey(current string, keyName string) string {
	current = strings.TrimSpace(current)
	if current == "" {
		return keyName
	}
	if strings.HasSuffix(current, "->") || strings.HasSuffix(current, "}") {
		return current + " " + keyName
	}
	if strings.HasSuffix(current, "+") {
		return current + keyName
	}
	return current + " -> " + keyName
}

func (m Model) updateComboInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	keyStr := msg.String()
	switch keyStr {
	case "enter":
		val := strings.TrimSpace(m.textInput.Value())
		if val == "" {
			m.state = StateMenu
			return m, nil
		}
		m.state = StateMenu
		return m, func() tea.Msg {
			err := m.manager.SendCombo(val)
			if err != nil {
				return actionDoneMsg{notice: fmt.Sprintf("Error: %v", err), isError: true}
			}
			return actionDoneMsg{notice: fmt.Sprintf("✔ Key combo sequence dispatched: %s", val), isError: false}
		}

	case "ctrl+d", "ctrl+q", "ctrl+g":
		m.state = StateMenu
		return m, nil

	case "esc":
		current := strings.TrimSpace(m.textInput.Value())
		if current == "" {
			m.state = StateMenu
			return m, nil
		}
		newVal := appendComboKey(current, "esc")
		m.textInput.SetValue(newVal)
		m.textInput.SetCursor(len(newVal))
		return m, nil

	case "backspace":
		var cmd tea.Cmd
		m.textInput, cmd = m.textInput.Update(msg)
		return m, cmd

	case "left", "right", "home", "end":
		var cmd tea.Cmd
		m.textInput, cmd = m.textInput.Update(msg)
		return m, cmd

	case "tab":
		newVal := appendComboKey(m.textInput.Value(), "tab")
		m.textInput.SetValue(newVal)
		m.textInput.SetCursor(len(newVal))
		return m, nil
	}

	// Capture modifier key combinations: ctrl+..., alt+...
	if strings.HasPrefix(keyStr, "ctrl+") || strings.HasPrefix(keyStr, "alt+") {
		newVal := appendComboKey(m.textInput.Value(), keyStr)
		m.textInput.SetValue(newVal)
		m.textInput.SetCursor(len(newVal))
		return m, nil
	}

	// Capture Function keys f1-f12
	if strings.HasPrefix(keyStr, "f") && len(keyStr) >= 2 && len(keyStr) <= 3 {
		if _, err := strconv.Atoi(keyStr[1:]); err == nil {
			newVal := appendComboKey(m.textInput.Value(), keyStr)
			m.textInput.SetValue(newVal)
			m.textInput.SetCursor(len(newVal))
			return m, nil
		}
	}

	// Standard text editing for typing characters: "->", "->{1.5}", "super+l", etc.
	var cmd tea.Cmd
	m.textInput, cmd = m.textInput.Update(msg)
	return m, cmd
}

func (m Model) updateLoginPrompt(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "tab", "down":
		if m.loginActiveIn == 0 {
			m.loginActiveIn = 1
			m.userInput.Blur()
			m.passInput.Focus()
			return m, textinput.Blink
		} else {
			m.loginActiveIn = 0
			m.passInput.Blur()
			m.userInput.Focus()
			return m, textinput.Blink
		}
	case "enter":
		user := strings.TrimSpace(m.userInput.Value())
		pass := strings.TrimSpace(m.passInput.Value())
		if user == "" || pass == "" {
			m.notice = "Username and password are required."
			m.isErrorNotice = true
			return m, nil
		}

		if keychain.IsAvailable() {
			_ = keychain.SetPassword(user, pass)
		}
		m.cfg.DefaultUser = user
		_ = config.SaveConfig(m.cfg)

		m.state = StateLoginExecuting
		m.loginStep = 0
		return m, m.startLoginCmd(user, pass)
	}

	var cmd tea.Cmd
	if m.loginActiveIn == 0 {
		m.userInput, cmd = m.userInput.Update(msg)
	} else {
		m.passInput, cmd = m.passInput.Update(msg)
	}
	return m, cmd
}

func (m Model) updateCredsMenu(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "c":
		if m.cfg.DefaultUser != "" {
			_ = keychain.DeletePassword(m.cfg.DefaultUser)
		}
		m.cfg.DefaultUser = ""
		_ = config.SaveConfig(m.cfg)
		m.notice = "✔ Cleared Keychain credentials."
		m.isErrorNotice = false
		m.state = StateMenu
	case "u":
		m.state = StateLoginPrompt
		m.userInput.SetValue(m.cfg.DefaultUser)
		m.passInput.SetValue("")
		m.userInput.Focus()
		return m, textinput.Blink
	}
	return m, nil
}
