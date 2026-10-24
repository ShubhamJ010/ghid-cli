package tui

import (
	"encoding/hex"
	"errors"
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
	"github.com/ghost-hid/ghid/internal/hid"
	"github.com/ghost-hid/ghid/internal/keychain"
	"github.com/ghost-hid/ghid/internal/snippets"
	"github.com/ghost-hid/ghid/internal/vault"
)

type AppState int

const (
	StateInit AppState = iota
	StateMenu
	StateLoginPrompt
	StateLoginExecuting
	StateSleepCountdown
	StateQuickSend
	StateComboInput
	StateSnippetStudio
	StateDoctorView
	StateLogsView
	StateDeviceRecovery
	StateDevicesList
	StateDeviceRegisterPlatform
	StateDeviceRegisterGuide
	StateDeviceRegisterPicker
	StateDeviceRegisterInput
	StateDeviceDeleteConfirm
	StateComboStudio
	StateComboExecuting
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
	combosStore    *combos.Store
	snippetsStore  *snippets.Store
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
	pickerDevices  []adb.BondedDevice
	pickerIndex    int
	pickerLoading  bool
	comboStudio    *ComboStudioModel
	snippetStudio  *SnippetStudioModel
	pendingCombo   *combos.Combo
	quickSendRaw   bool

	// Combo execution / countdown stepper
	execCombo         *combos.Combo
	execSteps         []hid.ComboStep
	execStepIdx       int
	execTotalDuration time.Duration
	execRemaining     time.Duration
	execStepRemaining time.Duration
	execStepDesc      string
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
type comboStepExecMsg struct {
	stepIdx int
	err     error
}
type comboTickMsg time.Time

func NewModel(mgr *adb.Manager, cfg config.Config) Model {
	cStore := combos.NewStore("")
	_, _ = cStore.EnsureDefaults()
	sStore := snippets.NewStore("", vault.NewKeychainVault(""))
	_, _ = sStore.EnsureDefaults()
	return NewModelWithStores(mgr, cfg, cStore, sStore)
}

func NewModelWithStores(mgr *adb.Manager, cfg config.Config, cStore *combos.Store, sStore *snippets.Store) Model {
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
	ui.Placeholder = "Username / $USERNAME"
	ui.CharLimit = 64
	ui.Width = 32

	pi := textinput.New()
	pi.Placeholder = "Password / $PASSWORD"
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

	devModel := cfg.PhoneName

	m := Model{
		manager:       mgr,
		cfg:           cfg,
		combosStore:   cStore,
		snippetsStore: sStore,
		state:         StateInit,
		deviceModel:   devModel,
		spinner:       s,
		progress:      p,
		textInput:     ti,
		userInput:     ui,
		passInput:     pi,
		devNameInput:  dni,
		devAddrInput:  dai,
		sleepTotal:    5,
		sleepSeconds:  5,
	}

	m.refreshMenuItems()
	return m
}

func (m *Model) refreshMenuItems() {
	var items []MenuItem
	idx := 1

	// Pinned favorite combos at the top, badged with their target OS and description.
	if m.combosStore != nil {
		favs, _ := m.combosStore.Favorites()
		for _, f := range favs {
			icon := "[" + combos.OSShortTag(f.OS) + "]"
			desc := f.Description
			if desc == "" {
				desc = f.Keys
			}
			items = append(items, MenuItem{
				Key:         fmt.Sprintf("%d", idx),
				Title:       f.Name,
				Description: desc,
				Icon:        icon,
			})
			idx++
		}
	}

	// Standard action items (built-ins like lock & sleep are now native favorite combos)
	items = append(items,
		MenuItem{Key: fmt.Sprintf("%d", idx), Title: "combo", Description: "Create, send & edit Windows combos", Icon: "🎹"},
	)
	idx++
	items = append(items,
		MenuItem{Key: fmt.Sprintf("%d", idx), Title: "quick", Description: "Quick send keystrokes or raw HID report (default: Type)", Icon: "⚡"},
	)
	idx++
	items = append(items,
		MenuItem{Key: fmt.Sprintf("%d", idx), Title: "snippets", Description: "Manage text & secret snippets ($NAME)", Icon: "📋"},
	)
	idx++
	items = append(items,
		MenuItem{Key: "d", Title: "devices", Description: "Registered target devices & add new Windows/Mac", Icon: "💻"},
	)
	items = append(items,
		MenuItem{Key: fmt.Sprintf("%d", idx), Title: "reconnect", Description: "Re-trigger Bluetooth HID connection", Icon: "🔄"},
	)
	idx++
	items = append(items,
		MenuItem{Key: fmt.Sprintf("%d", idx), Title: "doctor", Description: "Run full hardware & gateway diagnostic check", Icon: "🩺"},
	)
	items = append(items,
		MenuItem{Key: "0", Title: "exit", Description: "Disconnect Bluetooth & terminate phone service", Icon: "🚪"},
	)

	m.menuItems = items
}

// phoneDisplayName returns the dynamic user-friendly device name for the connected phone.
// It prioritizes explicit config override, dynamic Bluetooth/device name, hardware model, or fallback.
func (m Model) phoneDisplayName() string {
	if m.cfg.PhoneName != "" {
		return m.cfg.PhoneName
	}
	if m.deviceModel != "" && m.deviceModel != "Android Device" {
		return m.deviceModel
	}
	if m.device != nil && m.device.Model != "" {
		return m.device.Model
	}
	if m.manager != nil && m.manager.Serial != "" {
		name := m.manager.GetDeviceName()
		if name != "" && name != "Android Device" {
			return name
		}
	}
	if m.deviceModel != "" {
		return m.deviceModel
	}
	return "GhostHID Phone"
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
		devName := ""
		if m.cfg.PhoneName != "" {
			devName = m.cfg.PhoneName
		} else if m.manager != nil {
			devName = m.manager.GetDeviceName()
		}
		return initSuccessMsg{dev: dev, status: st, devName: devName}
	}
}

type initSuccessMsg struct {
	dev     *adb.Device
	status  adb.GatewayStatus
	devName string
}

type bondedDevicesMsg struct {
	devices []adb.BondedDevice
	err     error
}

func fetchBondedDevicesCmd(mgr *adb.Manager) tea.Cmd {
	return func() tea.Msg {
		if mgr == nil {
			return bondedDevicesMsg{err: errors.New("adb manager not initialized")}
		}
		devices, err := mgr.GetBondedBluetoothDevices()
		return bondedDevicesMsg{devices: devices, err: err}
	}
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
	// Full screen Combo Studio
	if m.state == StateComboStudio && m.comboStudio != nil {
		if key, ok := msg.(tea.KeyMsg); ok && key.String() == "ctrl+c" {
			m.quitting = true
			return m, tea.Quit
		}
		switch msg := msg.(type) {
		case tea.WindowSizeMsg:
			m.width = msg.Width
			m.height = msg.Height
			if m.comboStudio != nil {
				m.comboStudio.width = msg.Width
				m.comboStudio.height = msg.Height
			}
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
			m.refreshMenuItems()
			return m, nil
		}
		return m, cmd
	}

	// Full screen Snippet Studio
	if m.state == StateSnippetStudio && m.snippetStudio != nil {
		if key, ok := msg.(tea.KeyMsg); ok && key.String() == "ctrl+c" {
			m.quitting = true
			return m, tea.Quit
		}
		switch msg := msg.(type) {
		case tea.WindowSizeMsg:
			m.width = msg.Width
			m.height = msg.Height
			if m.snippetStudio != nil {
				m.snippetStudio.width = msg.Width
				m.snippetStudio.height = msg.Height
			}
		case statusTickMsg:
			m.lastPing = time.Time(msg)
			m.status = m.manager.GetStatus()
			return m, m.statusTick()
		}

		updated, cmd := m.snippetStudio.Update(msg)
		studio := updated.(SnippetStudioModel)
		m.snippetStudio = &studio
		if studio.quitting {
			m.snippetStudio = nil
			m.state = StateMenu
			m.refreshMenuItems()
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
		if msg.devName != "" {
			m.deviceModel = msg.devName
		} else if m.cfg.PhoneName != "" {
			m.deviceModel = m.cfg.PhoneName
		} else {
			m.deviceModel = m.manager.GetDeviceName()
		}
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

	case bondedDevicesMsg:
		m.pickerLoading = false
		if msg.err == nil {
			m.pickerDevices = msg.devices
		} else {
			m.notice = "Failed to query Bluetooth devices: " + msg.err.Error()
			m.isErrorNotice = true
		}
		return m, nil

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

	case comboStepExecMsg:
		if m.state == StateComboExecuting {
			if msg.err != nil {
				if errors.Is(msg.err, hid.ErrExitRequested) {
					m.quitting = true
					return m, tea.Quit
				}
				m.state = StateMenu
				m.notice = fmt.Sprintf("Combo failed at step %d: %v", msg.stepIdx+1, msg.err)
				m.isErrorNotice = true
				return m, nil
			}

			step := m.execSteps[m.execStepIdx]
			if step.Delay > 0 {
				m.execStepRemaining = step.Delay
				return m, m.comboTick()
			}

			m.execStepIdx++
			if m.execStepIdx >= len(m.execSteps) {
				m.state = StateMenu
				m.notice = fmt.Sprintf("✔ Executed %q (%s)", m.execCombo.Name, m.execCombo.Keys)
				m.isErrorNotice = false
				return m, nil
			}
			m.execStepDesc = describeStep(m.execSteps[m.execStepIdx])
			return m, m.executeCurrentStepCmd()
		}

	case comboTickMsg:
		if m.state == StateComboExecuting {
			stepTick := 100 * time.Millisecond
			m.execStepRemaining -= stepTick
			m.execRemaining -= stepTick
			if m.execRemaining < 0 {
				m.execRemaining = 0
			}

			if m.execStepRemaining <= 0 {
				m.execStepIdx++
				if m.execStepIdx >= len(m.execSteps) {
					m.state = StateMenu
					m.notice = fmt.Sprintf("✔ Executed %q (%s)", m.execCombo.Name, m.execCombo.Keys)
					m.isErrorNotice = false
					return m, nil
				}
				m.execStepDesc = describeStep(m.execSteps[m.execStepIdx])
				return m, m.executeCurrentStepCmd()
			}
			return m, m.comboTick()
		}

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
		if m.state == StateComboExecuting {
			switch msg.String() {
			case "esc", "q", "ctrl+c":
				m.state = StateMenu
				name := "Combo"
				if m.execCombo != nil {
					name = m.execCombo.Name
				}
				m.notice = fmt.Sprintf("Cancelled %q", name)
				m.isErrorNotice = false
				return m, nil
			}
		}

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
			if m.state == StateDeviceRegisterPicker {
				m.state = StateDeviceRegisterGuide
				return m, nil
			}
			if m.state == StateDeviceRegisterInput {
				m.state = StateDeviceRegisterPicker
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
		case StateQuickSend:
			return m.updateQuickSend(msg)
		case StateComboInput:
			return m.updateComboInput(msg)
		case StateLoginPrompt:
			return m.updateLoginPrompt(msg)
		case StateDevicesList:
			return m.updateDevicesList(msg)
		case StateDeviceRegisterPlatform:
			return m.updateRegisterPlatform(msg)
		case StateDeviceRegisterGuide:
			return m.updateRegisterGuide(msg)
		case StateDeviceRegisterPicker:
			return m.updateRegisterPicker(msg)
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
		} else if len(m.menuItems) > 0 {
			m.menuIndex = len(m.menuItems) - 1
		}
		return m, nil

	case "down", "j":
		if m.menuIndex < len(m.menuItems)-1 {
			m.menuIndex++
		} else {
			m.menuIndex = 0
		}
		return m, nil

	case "enter":
		if len(m.menuItems) > 0 && m.menuIndex < len(m.menuItems) {
			selected := m.menuItems[m.menuIndex]
			return m.triggerAction(selected.Title)
		}
		return m, nil

	case "q":
		return m.triggerAction("exit")

	default:
		keyStr := msg.String()
		for i, item := range m.menuItems {
			if strings.EqualFold(item.Key, keyStr) {
				m.menuIndex = i
				return m.triggerAction(item.Title)
			}
		}
	}
	return m, nil
}

func (m Model) triggerAction(action string) (tea.Model, tea.Cmd) {
	m.notice = ""

	// Check if action matches a combo (such as favorite Login combo)
	if m.combosStore != nil {
		if combo, ok := m.combosStore.Find(action); ok {
			return m.executeCombo(combo)
		}
	}

	switch action {
	case "lock":
		if m.combosStore != nil {
			if combo, ok := m.combosStore.Find("Lock"); ok {
				return m.executeCombo(combo)
			}
		}
		return m.executeCombo(combos.Combo{Name: "Lock", Keys: combos.DefaultLockKeys, OS: combos.OSWindows, Description: "Instant Windows lock screen (Win + L)"})

	case "sleep":
		if m.combosStore != nil {
			if combo, ok := m.combosStore.Find("Sleep"); ok {
				return m.executeCombo(combo)
			}
		}
		return m.executeCombo(combos.Combo{Name: "Sleep", Keys: combos.DefaultSleepKeys, OS: combos.OSWindows, Description: "Windows sleep sequence + countdown exit"})

	case "combo":
		studio := NewComboStudioModel(m.combosStore, func(combo combos.Combo) error {
			if m.manager == nil {
				return fmt.Errorf("adb manager not initialized")
			}
			return m.manager.SendComboWithResolver(combo.Keys, m.snippetsStore.Resolver())
		})
		studio.width = m.width
		studio.height = m.height
		m.comboStudio = &studio
		m.state = StateComboStudio
		return m, studio.Init()

	case "quick":
		m.state = StateQuickSend
		m.quickSendRaw = false
		m.textInput.Placeholder = "Type text to send (or press Tab for Raw mode)"
		m.textInput.SetValue("")
		m.textInput.Focus()
		return m, textinput.Blink

	case "snippets":
		studio := NewSnippetStudioModel(m.snippetsStore)
		studio.width = m.width
		studio.height = m.height
		m.snippetStudio = &studio
		m.state = StateSnippetStudio
		return m, studio.Init()

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
			return actionDoneMsg{notice: "✔ Reconnect broadcast dispatched!", isError: false}
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

func describeStep(step hid.ComboStep) string {
	switch {
	case step.Directive != "":
		return step.Directive
	case step.Variable != "":
		return "$" + step.Variable
	case step.Text != "":
		if len(step.Text) > 20 {
			return fmt.Sprintf("type: %q...", step.Text[:17])
		}
		return fmt.Sprintf("type: %q", step.Text)
	case step.Mouse != nil:
		return step.Mouse.Name
	default:
		return step.Combo
	}
}

func (m *Model) comboTick() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg {
		return comboTickMsg(t)
	})
}

func (m *Model) executeCurrentStepCmd() tea.Cmd {
	if m.execStepIdx >= len(m.execSteps) {
		return nil
	}
	step := m.execSteps[m.execStepIdx]
	idx := m.execStepIdx
	return func() tea.Msg {
		var err error
		switch {
		case step.Directive != "":
			upper := strings.ToUpper(strings.TrimSpace(step.Directive))
			switch {
			case upper == hid.DirectiveExit:
				return comboStepExecMsg{stepIdx: idx, err: hid.ErrExitRequested}
			case upper == hid.DirectiveDisconnect:
				err = m.manager.DisconnectTarget()
			case upper == hid.DirectiveConnect:
				err = m.manager.ConnectTarget("")
			case strings.HasPrefix(upper, hid.DirectiveConnect+":"):
				target := strings.TrimPrefix(step.Directive, hid.DirectiveConnect+":")
				err = m.manager.ConnectTarget(strings.TrimSpace(target))
			case upper == hid.DirectiveReconnect:
				_ = m.manager.DisconnectTarget()
				time.Sleep(300 * time.Millisecond)
				err = m.manager.ConnectTarget("")
			case upper == hid.DirectiveStart:
				err = m.manager.StartGateway()
			case upper == hid.DirectiveStop:
				err = m.manager.StopGateway()
			case upper == hid.DirectiveRestart:
				_ = m.manager.StopGateway()
				time.Sleep(300 * time.Millisecond)
				err = m.manager.StartGateway()
			case upper == hid.DirectiveUnpair:
				err = m.manager.UnpairTarget()
			case strings.HasPrefix(upper, hid.DirectiveComboSend):
				targetName := strings.TrimSpace(step.Directive[len(hid.DirectiveComboSend):])
				if m.combosStore != nil {
					if targetCombo, ok := m.combosStore.Find(targetName); ok {
						err = m.manager.SendComboWithResolver(targetCombo.Keys, m.snippetsStore.Resolver())
					} else {
						err = fmt.Errorf("combo %q not found", targetName)
					}
				}
			default:
				err = fmt.Errorf("unknown directive %q", step.Directive)
			}

		case step.Variable != "":
			if m.snippetsStore != nil {
				val, vErr := m.snippetsStore.Resolver()(step.Variable)
				if vErr != nil {
					err = vErr
				} else {
					err = m.manager.TypeText(val)
				}
			}

		case step.Text != "":
			text := step.Text
			if m.snippetsStore != nil {
				text = hid.ExpandTextVariables(text, m.snippetsStore.Resolver())
			}
			err = m.manager.TypeText(text)

		case step.Mouse != nil:
			err = m.manager.SendMouse(step.Mouse.Name)

		default:
			err = m.manager.SendCombo(step.Combo)
		}

		return comboStepExecMsg{stepIdx: idx, err: err}
	}
}

func (m Model) executeCombo(combo combos.Combo) (tea.Model, tea.Cmd) {
	vars := hid.ExtractVariables(combo.Keys)
	missing := false
	if m.snippetsStore != nil {
		for _, v := range vars {
			snip, ok, _ := m.snippetsStore.Get(v)
			if !ok || snip.Value == "" {
				missing = true
				break
			}
		}
	}

	if missing {
		m.pendingCombo = &combo
		m.state = StateLoginPrompt
		m.loginActiveIn = 0
		var uVal, pVal string
		if m.snippetsStore != nil {
			if u, ok, _ := m.snippetsStore.Get("username"); ok {
				uVal = u.Value
			}
			if p, ok, _ := m.snippetsStore.Get("password"); ok {
				pVal = p.Value
			}
		}
		if uVal == "" {
			uVal = m.cfg.DefaultUser
		}
		m.userInput.SetValue(uVal)
		m.passInput.SetValue(pVal)
		m.userInput.Focus()
		m.passInput.Blur()
		m.notice = fmt.Sprintf("Combo %q references empty snippet(s). Please set required values:", combo.Name)
		m.isErrorNotice = false
		return m, textinput.Blink
	}

	steps, err := hid.ParseActionSequence(combo.Keys)
	if err != nil {
		m.notice = fmt.Sprintf("Invalid combo sequence: %v", err)
		m.isErrorNotice = true
		return m, nil
	}

	totalDelay := time.Duration(0)
	for _, s := range steps {
		totalDelay += s.Delay
	}

	// Zero delay combos (e.g. Lock with win+l) execute immediately without countdown UI
	if totalDelay == 0 {
		m.notice = fmt.Sprintf("Dispatching %q...", combo.Name)
		m.isErrorNotice = false
		return m, func() tea.Msg {
			err := m.manager.SendComboWithResolver(combo.Keys, m.snippetsStore.Resolver())
			if err != nil {
				return actionDoneMsg{notice: fmt.Sprintf("Combo failed: %v", err), isError: true}
			}
			return actionDoneMsg{notice: fmt.Sprintf("✔ Executed %q (%s)", combo.Name, combo.Keys), isError: false}
		}
	}

	// Combos with delay transition to unified countdown stepper UI
	m.state = StateComboExecuting
	m.execCombo = &combo
	m.execSteps = steps
	m.execStepIdx = 0
	m.execTotalDuration = totalDelay
	m.execRemaining = totalDelay
	m.execStepRemaining = 0
	if len(steps) > 0 {
		m.execStepDesc = describeStep(steps[0])
	}
	m.notice = ""
	m.isErrorNotice = false

	return m, m.executeCurrentStepCmd()
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
	case "1":
		m.regPlatform = "windows"
		m.state = StateDeviceRegisterGuide
	case "2":
		m.regPlatform = "macos"
		m.state = StateDeviceRegisterGuide
	case "esc":
		m.state = StateDevicesList
	}
	return m, nil
}

func (m Model) updateRegisterGuide(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter", " ":
		m.state = StateDeviceRegisterPicker
		m.pickerLoading = true
		m.pickerIndex = 0
		return m, fetchBondedDevicesCmd(m.manager)
	case "esc":
		m.state = StateDeviceRegisterPlatform
	}
	return m, nil
}

func (m Model) updateRegisterPicker(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	totalItems := len(m.pickerDevices) + 1 // +1 for manual option
	switch msg.String() {
	case "up", "k":
		if m.pickerIndex > 0 {
			m.pickerIndex--
		} else {
			m.pickerIndex = totalItems - 1
		}
	case "down", "j":
		if m.pickerIndex < totalItems-1 {
			m.pickerIndex++
		} else {
			m.pickerIndex = 0
		}
	case "r":
		m.pickerLoading = true
		return m, fetchBondedDevicesCmd(m.manager)
	case "m":
		m.state = StateDeviceRegisterInput
		m.regActiveIn = 0
		m.devNameInput.SetValue("")
		m.devAddrInput.SetValue("")
		m.devNameInput.Focus()
		m.devAddrInput.Blur()
		return m, textinput.Blink
	case "e":
		if m.pickerIndex < len(m.pickerDevices) {
			dev := m.pickerDevices[m.pickerIndex]
			m.state = StateDeviceRegisterInput
			m.regActiveIn = 0
			m.devNameInput.SetValue(dev.Name)
			m.devAddrInput.SetValue(dev.Address)
			m.devNameInput.Focus()
			m.devAddrInput.Blur()
			return m, textinput.Blink
		}
	case "enter", "s":
		if m.pickerIndex == len(m.pickerDevices) {
			// Manual entry option selected
			m.state = StateDeviceRegisterInput
			m.regActiveIn = 0
			m.devNameInput.SetValue("")
			m.devAddrInput.SetValue("")
			m.devNameInput.Focus()
			m.devAddrInput.Blur()
			return m, textinput.Blink
		}
		dev := m.pickerDevices[m.pickerIndex]
		platform := m.regPlatform
		nameLower := strings.ToLower(dev.Name)
		if strings.Contains(nameLower, "mac") || strings.Contains(nameLower, "apple") || strings.Contains(nameLower, "air") {
			platform = "macos"
		} else if strings.Contains(nameLower, "windows") || strings.Contains(nameLower, "thinkpad") || strings.Contains(nameLower, "lp-") || strings.Contains(nameLower, "pc") {
			platform = "windows"
		}

		newDev := m.cfg.AddDevice(dev.Name, dev.Address, platform, true)
		_ = config.SaveConfig(m.cfg)
		m.state = StateDevicesList
		m.devicesIndex = len(m.cfg.Devices) - 1

		return m, func() tea.Msg {
			if m.manager != nil {
				_ = m.manager.StartGateway()
				time.Sleep(300 * time.Millisecond)
				_ = m.manager.ConnectTarget(dev.Address)
			}
			return actionDoneMsg{
				notice:  fmt.Sprintf("✔ Registered & connecting to '%s' (%s) [%s]", newDev.Name, newDev.Platform, newDev.Address),
				isError: false,
			}
		}
	case "esc":
		m.state = StateDeviceRegisterGuide
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
		} else {
			m.regActiveIn = 0
			m.devAddrInput.Blur()
			m.devNameInput.Focus()
		}
		return m, textinput.Blink
	case "shift+tab", "up":
		if m.regActiveIn == 1 {
			m.regActiveIn = 0
			m.devAddrInput.Blur()
			m.devNameInput.Focus()
		} else {
			m.regActiveIn = 1
			m.devNameInput.Blur()
			m.devAddrInput.Focus()
		}
		return m, textinput.Blink
	case "enter":
		name := strings.TrimSpace(m.devNameInput.Value())
		addr := strings.ToUpper(strings.TrimSpace(m.devAddrInput.Value()))

		if name == "" {
			m.notice = "Device name cannot be empty."
			m.isErrorNotice = true
			return m, nil
		}
		if !isValidMAC(addr) {
			m.notice = "Invalid Bluetooth MAC address (e.g. 38:FC:98:E8:E3:87)."
			m.isErrorNotice = true
			return m, nil
		}

		dev := m.cfg.AddDevice(name, addr, m.regPlatform, true)
		_ = config.SaveConfig(m.cfg)

		m.state = StateDevicesList
		m.devicesIndex = len(m.cfg.Devices) - 1

		return m, func() tea.Msg {
			_ = m.manager.StartGateway()
			time.Sleep(300 * time.Millisecond)
			_ = m.manager.ConnectTarget(dev.Address)
			return actionDoneMsg{
				notice:  fmt.Sprintf("✔ Registered & connecting to '%s' (%s) [%s]", dev.Name, dev.Platform, dev.Address),
				isError: false,
			}
		}
	case "esc":
		m.state = StateDeviceRegisterPicker
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

func isValidMAC(mac string) bool {
	parts := strings.Split(mac, ":")
	if len(parts) != 6 {
		return false
	}
	for _, p := range parts {
		if len(p) != 2 {
			return false
		}
		for _, c := range p {
			if !((c >= '0' && c <= '9') || (c >= 'A' && c <= 'F') || (c >= 'a' && c <= 'f')) {
				return false
			}
		}
	}
	return true
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
	snipCount := 0
	if m.snippetsStore != nil {
		list, _ := m.snippetsStore.List()
		snipCount = len(list)
	}

	m.doctorChecks = []DoctorCheck{
		{Title: "ADB Executable", Pass: m.manager.AdbPath != "", Info: m.manager.AdbPath},
		{Title: "Android Device", Pass: m.device != nil, Info: fmt.Sprintf("%s [%s]", m.phoneDisplayName(), m.manager.Serial)},
		{Title: "GhostHID APK", Pass: m.manager.IsPackageInstalled(), Info: adb.PackageName},
		{Title: "Phone Bluetooth Radio", Pass: m.manager.GetBluetoothState(), Info: "Enabled"},
		{Title: "Bluetooth HID (SDP)", Pass: m.status.Registered, Info: "Profile Active"},
		{Title: "Target Laptop Link", Pass: m.status.Connected, Info: fmt.Sprintf("%s [%s]", m.status.Target, m.status.Address)},
		{Title: "macOS Keychain Vault", Pass: keychain.IsAvailable(), Info: "Service 'ghost-hid'"},
		{Title: "Snippets Subsystem", Pass: snipCount > 0, Info: fmt.Sprintf("%d snippets loaded", snipCount)},
	}
}

func (m Model) updateQuickSend(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.state = StateMenu
		return m, nil

	case "tab", "ctrl+r", "ctrl+t":
		m.quickSendRaw = !m.quickSendRaw
		if m.quickSendRaw {
			m.textInput.Placeholder = "16 hex chars e.g. 0000040000000000"
		} else {
			m.textInput.Placeholder = "Type text to send (or press Tab for Raw mode)"
		}
		return m, nil

	case "enter":
		val := strings.TrimSpace(m.textInput.Value())
		if val == "" {
			m.state = StateMenu
			return m, nil
		}

		isRaw := m.quickSendRaw
		if strings.HasPrefix(strings.ToLower(val), "raw:") {
			isRaw = true
			val = strings.TrimSpace(val[4:])
		} else if strings.HasPrefix(strings.ToLower(val), "type:") {
			isRaw = false
			val = strings.TrimSpace(val[5:])
		}

		m.state = StateMenu

		if isRaw {
			cleanHex := strings.ReplaceAll(val, " ", "")
			if _, err := hex.DecodeString(cleanHex); err != nil || len(cleanHex) != 16 {
				m.notice = "Raw HID report must be exactly 16 hexadecimal characters (8 bytes)."
				m.isErrorNotice = true
				return m, nil
			}
			return m, func() tea.Msg {
				err := m.manager.SendRaw(cleanHex)
				if err != nil {
					return actionDoneMsg{notice: fmt.Sprintf("Error sending raw report: %v", err), isError: true}
				}
				return actionDoneMsg{notice: fmt.Sprintf("✔ Dispatched raw HID report: %s", cleanHex), isError: false}
			}
		}

		return m, func() tea.Msg {
			err := m.manager.TypeText(val)
			if err != nil {
				return actionDoneMsg{notice: fmt.Sprintf("Error sending text: %v", err), isError: true}
			}
			return actionDoneMsg{notice: fmt.Sprintf("✔ Typed text (%d characters)", len(val)), isError: false}
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
			err := m.manager.SendComboWithResolver(val, m.snippetsStore.Resolver())
			if err != nil {
				return actionDoneMsg{notice: fmt.Sprintf("Error: %v", err), isError: true}
			}
			return actionDoneMsg{notice: fmt.Sprintf("✔ Key combo sequence dispatched: %s", val), isError: false}
		}

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

	var cmd tea.Cmd
	m.textInput, cmd = m.textInput.Update(msg)
	return m, cmd
}

func (m Model) updateLoginPrompt(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.pendingCombo = nil
		m.state = StateMenu
		return m, nil

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
			m.notice = "Username and password cannot be empty."
			m.isErrorNotice = true
			return m, nil
		}

		if m.snippetsStore != nil {
			_, _ = m.snippetsStore.Set("username", user, false)
			_, _ = m.snippetsStore.Set("password", pass, true)
		}
		if keychain.IsAvailable() {
			_ = keychain.SetPassword(user, pass)
		}
		m.cfg.DefaultUser = user
		_ = config.SaveConfig(m.cfg)

		m.state = StateMenu
		if m.pendingCombo != nil {
			combo := *m.pendingCombo
			m.pendingCombo = nil
			return m.executeCombo(combo)
		}
		m.notice = "✔ Snippets updated."
		m.isErrorNotice = false
		m.refreshMenuItems()
		return m, nil
	}

	var cmd tea.Cmd
	if m.loginActiveIn == 0 {
		m.userInput, cmd = m.userInput.Update(msg)
	} else {
		m.passInput, cmd = m.passInput.Update(msg)
	}
	return m, cmd
}
