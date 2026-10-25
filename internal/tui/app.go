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
	"github.com/ghost-hid/ghid/internal/payloads"
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
	StatePayloadStudio
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
	payloadsStore  *payloads.Store
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
	payloadStudio  *PayloadStudioModel
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

func NewModelWithStores(mgr *adb.Manager, cfg config.Config, cStore *combos.Store, pStore *payloads.Store) Model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(ColorCyan)

	p := progress.New(
		progress.WithDefaultGradient(),
		progress.WithWidth(40),
	)

	ti := textinput.New()
	ti.Placeholder = "Enter text..."
	ti.CharLimit = 256
	ti.Width = 40

	ui := textinput.New()
	ui.Placeholder = "Username"
	ui.CharLimit = 64
	ui.Width = 32

	pi := textinput.New()
	pi.Placeholder = "Password"
	pi.EchoMode = textinput.EchoPassword
	pi.EchoCharacter = '•'
	pi.CharLimit = 64
	pi.Width = 32

	dni := textinput.New()
	dni.Placeholder = "e.g. Work MacBook Pro"
	dni.CharLimit = 40
	dni.Width = 32

	dai := textinput.New()
	dai.Placeholder = "e.g. 38:FC:98:E8:E3:87"
	dai.CharLimit = 17
	dai.Width = 32

	m := Model{
		manager:       mgr,
		cfg:           cfg,
		combosStore:   cStore,
		payloadsStore: pStore,
		state:         StateMenu,
		spinner:       s,
		progress:      p,
		textInput:     ti,
		userInput:     ui,
		passInput:     pi,
		devNameInput:  dni,
		devAddrInput:  dai,
	}

	m.refreshMenuItems()
	return m
}

func NewModel(mgr *adb.Manager, cfg config.Config) Model {
	cStore := combos.NewStore("", nil)
	_, _ = cStore.EnsureDefaults()
	pStore := payloads.NewStore("", nil)
	_, _ = pStore.EnsureDefaults()
	cStore.PayloadStore = pStore
	return NewModelWithStores(mgr, cfg, cStore, pStore)
}

func (m *Model) refreshMenuItems() {
	var items []MenuItem
	idx := 1

	// Dynamic favorite combos pinned to top
	if m.combosStore != nil {
		favs, _ := m.combosStore.Favorites()
		for _, f := range favs {
			icon := f.Badge()
			desc := f.Description
			if desc == "" {
				desc = f.Sequence
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

	// Standard action items
	items = append(items,
		MenuItem{Key: fmt.Sprintf("%d", idx), Title: "combo", Description: "Create, send & edit sequences across OSs", Icon: "🎹"},
	)
	idx++
	items = append(items,
		MenuItem{Key: fmt.Sprintf("%d", idx), Title: "quick", Description: "Quick send keystrokes or raw HID report (default: Type)", Icon: "⚡"},
	)
	idx++
	items = append(items,
		MenuItem{Key: fmt.Sprintf("%d", idx), Title: "payloads", Description: "Manage payloads & secrets ({NAME})", Icon: "📋"},
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

func (m Model) phoneDisplayName() string {
	if m.cfg.PhoneName != "" {
		return m.cfg.PhoneName
	}
	if m.deviceModel != "" {
		return m.deviceModel
	}
	return "Android Device"
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.spinner.Tick,
		m.statusTick(),
		func() tea.Msg {
			if m.manager != nil {
				devs, err := m.manager.GetDevices()
				if err == nil && len(devs) > 0 {
					return &devs[0]
				}
			}
			return nil
		},
	)
}

func (m *Model) statusTick() tea.Cmd {
	return tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
		return statusTickMsg(t)
	})
}

func (m *Model) sleepTick() tea.Cmd {
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

	// Full screen Payload Studio
	if m.state == StatePayloadStudio && m.payloadStudio != nil {
		if key, ok := msg.(tea.KeyMsg); ok && key.String() == "ctrl+c" {
			m.quitting = true
			return m, tea.Quit
		}
		switch msg := msg.(type) {
		case tea.WindowSizeMsg:
			m.width = msg.Width
			m.height = msg.Height
			if m.payloadStudio != nil {
				m.payloadStudio.width = msg.Width
				m.payloadStudio.height = msg.Height
			}
		case statusTickMsg:
			m.lastPing = time.Time(msg)
			m.status = m.manager.GetStatus()
			return m, m.statusTick()
		}

		updated, cmd := m.payloadStudio.Update(msg)
		studio := updated.(PayloadStudioModel)
		m.payloadStudio = &studio
		if studio.quitting {
			m.payloadStudio = nil
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

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		cmds = append(cmds, cmd)

	case statusTickMsg:
		m.lastPing = time.Time(msg)
		if m.manager != nil {
			m.status = m.manager.GetStatus()
			if m.deviceModel == "" {
				m.deviceModel = m.manager.GetDeviceModel()
			}
		}
		cmds = append(cmds, m.statusTick())

	case *adb.Device:
		m.device = msg
		if m.device != nil && m.manager != nil && m.deviceModel == "" {
			m.deviceModel = m.manager.GetDeviceModel()
		}

	case loginStepMsg:
		m.loginStep = msg.step
		m.loginStepText = msg.text

	case loginDoneMsg:
		m.state = StateMenu
		if msg != nil {
			m.notice = fmt.Sprintf("Login sequence failed: %v", msg)
			m.isErrorNotice = true
		} else {
			m.notice = "✔ Login sequence dispatched successfully!"
			m.isErrorNotice = false
		}

	case actionDoneMsg:
		m.state = StateMenu
		m.notice = msg.notice
		m.isErrorNotice = msg.isError

	case fetchBondedDevicesMsg:
		m.pickerLoading = false
		m.pickerDevices = msg.devices
		if msg.err != nil {
			m.notice = fmt.Sprintf("Error fetching devices: %v", msg.err)
			m.isErrorNotice = true
		} else {
			m.notice = ""
			m.isErrorNotice = false
		}
		if len(m.pickerDevices) > 0 {
			m.pickerIndex = 0
		}
		return m, nil

	case comboResultMsg:
		if m.state == StateComboStudio && m.comboStudio != nil {
			m.comboStudio.busy = false
			if msg.err != nil {
				m.comboStudio.notice = fmt.Sprintf("✖ Send failed: %v", msg.err)
				m.comboStudio.isError = true
			} else {
				m.comboStudio.notice = fmt.Sprintf("✔ Sent %q  →  %s", msg.name, msg.sequence)
				m.comboStudio.isError = false
			}
			return m, nil
		}
		m.quitting = true
		return m, tea.Quit

	case comboStepExecMsg:
		if m.state == StateComboExecuting {
			if msg.err != nil {
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
				m.notice = fmt.Sprintf("✔ Executed %q (%s)", m.execCombo.Name, m.execCombo.Sequence)
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
					m.notice = fmt.Sprintf("✔ Executed %q (%s)", m.execCombo.Name, m.execCombo.Sequence)
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

	case "c":
		return m.triggerAction("combo")

	case "d":
		return m.triggerAction("devices")

	default:
		key := msg.String()
		for _, item := range m.menuItems {
			if item.Key == key {
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
		return m.executeCombo(combos.Combo{Name: "Lock", Sequence: combos.DefaultLockSequence, OS: combos.OSWindows, Description: "Instant Windows lock screen (Win + L)"})

	case "sleep":
		if m.combosStore != nil {
			if combo, ok := m.combosStore.Find("Sleep"); ok {
				return m.executeCombo(combo)
			}
		}
		return m.executeCombo(combos.Combo{Name: "Sleep", Sequence: combos.DefaultSleepSequence, OS: combos.OSWindows, Description: "Hardware ACPI sleep sequence"})

	case "combo":
		studio := NewComboStudioModel(m.combosStore, func(combo combos.Combo) error {
			if m.manager == nil {
				return fmt.Errorf("adb manager not initialized")
			}
			return m.manager.SendComboWithResolver(combo.Sequence, m.payloadsStore.Resolver())
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

	case "payloads", "payload":
		studio := NewPayloadStudioModel(m.payloadsStore)
		studio.width = m.width
		studio.height = m.height
		m.payloadStudio = &studio
		m.state = StatePayloadStudio
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
	case step.Payload != "":
		return "{" + step.Payload + "}"
	case step.Text != "":
		if len(step.Text) > 20 {
			return fmt.Sprintf("%q...", step.Text[:17])
		}
		return fmt.Sprintf("%q", step.Text)
	case step.Mouse != nil:
		return step.Mouse.Name
	case step.Raw != "":
		return "raw:" + step.Raw
	case step.Delay > 0:
		return step.Delay.String()
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
		case step.Payload != "":
			if m.payloadsStore != nil {
				val, vErr := m.payloadsStore.Resolver()(step.Payload)
				if vErr != nil {
					err = vErr
				} else {
					cleanVal := strings.TrimSpace(val)
					cleanHex := strings.ReplaceAll(cleanVal, " ", "")
					if _, rawErr := hid.ValidateRaw(cleanHex); rawErr == nil {
						err = m.manager.SendRaw(cleanHex)
					} else {
						err = m.manager.TypeText(val)
					}
				}
			}

		case step.Raw != "":
			err = m.manager.SendRaw(step.Raw)

		case step.Text != "":
			err = m.manager.TypeText(step.Text)

		case step.Mouse != nil:
			err = m.manager.SendMouse(step.Mouse.Name)

		default:
			if step.Combo != "" {
				err = m.manager.SendCombo(step.Combo)
			}
		}

		return comboStepExecMsg{stepIdx: idx, err: err}
	}
}

func (m Model) executeCombo(combo combos.Combo) (tea.Model, tea.Cmd) {
	payloadsNeeded := hid.ExtractPayloads(combo.Sequence)
	missing := false
	if m.payloadsStore != nil {
		for _, p := range payloadsNeeded {
			item, ok, _ := m.payloadsStore.Get(p)
			if !ok || item.Value == "" {
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
		if m.payloadsStore != nil {
			if u, ok, _ := m.payloadsStore.Get("username"); ok {
				uVal = u.Value
			}
			if p, ok, _ := m.payloadsStore.Get("password"); ok {
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
		m.notice = fmt.Sprintf("Combo %q references empty payload(s). Please set required values:", combo.Name)
		m.isErrorNotice = false
		return m, textinput.Blink
	}

	steps, err := hid.ParseSequence(combo.Sequence)
	if err != nil {
		m.notice = fmt.Sprintf("Invalid combo sequence: %v", err)
		m.isErrorNotice = true
		return m, nil
	}

	totalDelay := time.Duration(0)
	for _, s := range steps {
		totalDelay += s.Delay
	}

	// Zero delay combos execute immediately without countdown UI
	if totalDelay == 0 {
		m.notice = fmt.Sprintf("Dispatching %q...", combo.Name)
		m.isErrorNotice = false
		return m, func() tea.Msg {
			err := m.manager.SendComboWithResolver(combo.Sequence, m.payloadsStore.Resolver())
			if err != nil {
				return actionDoneMsg{notice: fmt.Sprintf("Combo failed: %v", err), isError: true}
			}
			return actionDoneMsg{notice: fmt.Sprintf("✔ Executed %q (%s)", combo.Name, combo.Sequence), isError: false}
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
	payloadCount := 0
	if m.payloadsStore != nil {
		list, _ := m.payloadsStore.List()
		payloadCount = len(list)
	}

	m.doctorChecks = []DoctorCheck{
		{Title: "ADB Executable", Pass: m.manager.AdbPath != "", Info: m.manager.AdbPath},
		{Title: "Android Device", Pass: m.device != nil, Info: fmt.Sprintf("%s [%s]", m.phoneDisplayName(), m.manager.Serial)},
		{Title: "GhostHID APK", Pass: m.manager.IsPackageInstalled(), Info: adb.PackageName},
		{Title: "Phone Bluetooth Radio", Pass: m.manager.GetBluetoothState(), Info: "Enabled"},
		{Title: "Bluetooth HID (SDP)", Pass: m.status.Registered, Info: "Profile Active"},
		{Title: "Target Laptop Link", Pass: m.status.Connected, Info: fmt.Sprintf("%s [%s]", m.status.Target, m.status.Address)},
		{Title: "macOS Keychain Vault", Pass: keychain.IsAvailable(), Info: "Service 'ghost-hid'"},
		{Title: "Payloads Subsystem", Pass: payloadCount > 0, Info: fmt.Sprintf("%d payloads loaded", payloadCount)},
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
	return current + "{" + keyName + "}"
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
			err := m.manager.SendComboWithResolver(val, m.payloadsStore.Resolver())
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

		if m.payloadsStore != nil {
			_, _ = m.payloadsStore.Set("username", user, false, "type")
			_, _ = m.payloadsStore.Set("password", pass, true, "type")
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
		m.notice = "✔ Payloads updated."
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

func fetchBondedDevicesCmd(mgr *adb.Manager) tea.Cmd {
	return func() tea.Msg {
		if mgr == nil {
			return fetchBondedDevicesMsg{err: errors.New("adb manager not initialized")}
		}
		devs, err := mgr.GetBondedBluetoothDevices()
		return fetchBondedDevicesMsg{devices: devs, err: err}
	}
}

type fetchBondedDevicesMsg struct {
	devices []adb.BondedDevice
	err     error
}

func describeStatusOS(s adb.GatewayStatus) string {
	osName := strings.ToLower(strings.TrimSpace(s.OS))
	if osName == "" || osName == "unknown" {
		return "Unknown OS"
	}
	label := combos.OSLabel(osName)
	if s.OSConfidence != "" {
		label += fmt.Sprintf(" (%s confidence)", s.OSConfidence)
	}
	return label
}
