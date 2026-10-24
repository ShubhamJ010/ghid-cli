package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ghost-hid/ghid/internal/adb"
	"github.com/ghost-hid/ghid/internal/config"
)

func TestAppendComboKey(t *testing.T) {
	tests := []struct {
		name     string
		current  string
		keyName  string
		expected string
	}{
		{
			name:     "first key on empty input",
			current:  "",
			keyName:  "ctrl+l",
			expected: "ctrl+l",
		},
		{
			name:     "chain without arrow automatically adds -> with default 1.0s delay",
			current:  "ctrl+l",
			keyName:  "alt+p",
			expected: "ctrl+l -> alt+p",
		},
		{
			name:     "chain after typing arrow ->",
			current:  "ctrl+l ->",
			keyName:  "alt+p",
			expected: "ctrl+l -> alt+p",
		},
		{
			name:     "chain after typing arrow with spaces",
			current:  "ctrl+l -> ",
			keyName:  "alt+p",
			expected: "ctrl+l -> alt+p",
		},
		{
			name:     "chain after typing explicit duration ->{1.5}",
			current:  "ctrl+l ->{1.5}",
			keyName:  "alt+p",
			expected: "ctrl+l ->{1.5} alt+p",
		},
		{
			name:     "chain after typing explicit duration with trailing space ->{1.5} ",
			current:  "ctrl+l ->{1.5} ",
			keyName:  "alt+p",
			expected: "ctrl+l ->{1.5} alt+p",
		},
		{
			name:     "first key esc",
			current:  "",
			keyName:  "esc",
			expected: "esc",
		},
		{
			name:     "double esc chaining",
			current:  "esc",
			keyName:  "esc",
			expected: "esc -> esc",
		},
		{
			name:     "super modifier prefix",
			current:  "super+",
			keyName:  "l",
			expected: "super+l",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := appendComboKey(tc.current, tc.keyName)
			if got != tc.expected {
				t.Errorf("appendComboKey(%q, %q) = %q; want %q", tc.current, tc.keyName, got, tc.expected)
			}
		})
	}
}

func TestUpdateComboInputKeyHandling(t *testing.T) {
	ti := textinput.New()
	ti.Focus()
	m := Model{
		state:     StateComboInput,
		textInput: ti,
	}

	// 1. Direct ctrl+l key message
	m1, _ := m.updateComboInput(tea.KeyMsg{Type: tea.KeyCtrlL})
	mod1 := m1.(Model)
	if mod1.textInput.Value() != "ctrl+l" {
		t.Errorf("expected value 'ctrl+l', got %q", mod1.textInput.Value())
	}

	// 2. Type ->{1.5}
	for _, r := range " ->{1.5}" {
		m1, _ = mod1.updateComboInput(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		mod1 = m1.(Model)
	}

	// 3. Press alt+p (Option+P)
	m2, _ := mod1.updateComboInput(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p"), Alt: true})
	mod2 := m2.(Model)
	if mod2.textInput.Value() != "ctrl+l ->{1.5} alt+p" {
		t.Errorf("expected 'ctrl+l ->{1.5} alt+p', got %q", mod2.textInput.Value())
	}

	// 4. Press Esc on non-empty: appends "esc"
	m3, _ := mod2.updateComboInput(tea.KeyMsg{Type: tea.KeyEsc})
	mod3 := m3.(Model)
	if mod3.textInput.Value() != "ctrl+l ->{1.5} alt+p -> esc" {
		t.Errorf("expected appended esc, got %q", mod3.textInput.Value())
	}

	// 5. Cancel on empty with Esc returns to StateMenu
	emptyTi := textinput.New()
	emptyTi.Focus()
	emptyModel := Model{state: StateComboInput, textInput: emptyTi}
	mEmpty, _ := emptyModel.updateComboInput(tea.KeyMsg{Type: tea.KeyEsc})
	if mEmpty.(Model).state != StateMenu {
		t.Errorf("expected state to return to StateMenu on empty esc")
	}
}

func TestMenuComboOpensStudio(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	m := Model{state: StateMenu, menuItems: []MenuItem{{Key: "5", Title: "combo"}}}
	next, _ := m.triggerAction("combo")
	got := next.(Model)
	if got.state != StateComboStudio || got.comboStudio == nil {
		t.Fatalf("menu combo should open the studio, state=%v studio=%v", got.state, got.comboStudio)
	}
	if !strings.Contains(got.View(), "COMBO STUDIO") {
		t.Fatalf("studio view not rendered:\n%s", got.View())
	}

	// esc returns to the dashboard instead of quitting the app.
	next, _ = got.Update(tea.KeyMsg{Type: tea.KeyEsc})
	got = next.(Model)
	if got.state != StateMenu || got.comboStudio != nil {
		t.Fatalf("esc should close the studio back to the menu, state=%v", got.state)
	}
}

func TestQuickSendToggle(t *testing.T) {
	ti := textinput.New()
	ti.Focus()
	m := Model{
		state:        StateQuickSend,
		textInput:    ti,
		quickSendRaw: false,
	}

	// Press Tab: toggles to Raw mode
	m1, _ := m.updateQuickSend(tea.KeyMsg{Type: tea.KeyTab})
	mod1 := m1.(Model)
	if !mod1.quickSendRaw {
		t.Fatalf("expected quickSendRaw to be true after Tab")
	}
	if !strings.Contains(mod1.textInput.Placeholder, "16 hex") {
		t.Fatalf("expected raw placeholder, got %q", mod1.textInput.Placeholder)
	}

	// Press Tab again: toggles back to Type mode
	m2, _ := mod1.updateQuickSend(tea.KeyMsg{Type: tea.KeyTab})
	mod2 := m2.(Model)
	if mod2.quickSendRaw {
		t.Fatalf("expected quickSendRaw to be false after second Tab")
	}

	// Press Esc: returns to StateMenu
	m3, _ := mod2.updateQuickSend(tea.KeyMsg{Type: tea.KeyEsc})
	mod3 := m3.(Model)
	if mod3.state != StateMenu {
		t.Fatalf("expected state to be StateMenu after Esc, got %v", mod3.state)
	}
}

func TestQuickSendHexValidation(t *testing.T) {
	ti := textinput.New()
	ti.SetValue("gggggggggggggggg") // 16 chars, but invalid hex
	m := Model{
		state:        StateQuickSend,
		textInput:    ti,
		quickSendRaw: true,
	}

	m1, cmd := m.updateQuickSend(tea.KeyMsg{Type: tea.KeyEnter})
	mod1 := m1.(Model)
	if cmd != nil {
		t.Errorf("expected no execution command on invalid hex")
	}
	if !mod1.isErrorNotice || !strings.Contains(mod1.notice, "16 hexadecimal characters") {
		t.Errorf("expected error notice about 16 hex chars, got %q (isErr=%v)", mod1.notice, mod1.isErrorNotice)
	}
}

func TestTelemetryCardDoesNotExposeSnippets(t *testing.T) {
	m := Model{
		state:       StateMenu,
		deviceModel: "Pixel 7",
		device:      &adb.Device{Serial: "192.168.1.50:5555"},
		status:      adb.GatewayStatus{Registered: true, Connected: false},
	}
	card := m.renderTelemetryCard()

	if strings.Contains(card, "Snippets") || strings.Contains(card, "Keychain") {
		t.Fatalf("telemetry card should not expose Snippets or Keychain:\n%s", card)
	}
	if !strings.Contains(card, "Phone:") || !strings.Contains(card, "Target:") {
		t.Fatalf("telemetry card missing Phone or Target rows:\n%s", card)
	}
	if !strings.Contains(card, "SDP:") || !strings.Contains(card, "Link:") {
		t.Fatalf("telemetry card missing SDP or Link labels:\n%s", card)
	}
	if !strings.Contains(card, "READY") || !strings.Contains(card, "DISCONNECTED") {
		t.Fatalf("telemetry card missing status badges:\n%s", card)
	}
}

func TestMenuItemsDoNotWrapMultiWordTitles(t *testing.T) {
	m := Model{
		state: StateMenu,
		menuItems: []MenuItem{
			{Key: "1", Title: "Open RustDesk", Description: "Open Rustdesk", Icon: "😎"},
			{Key: "2", Title: "Login", Description: "Windows unlock & credential injection", Icon: "🔑"},
			{Key: "3", Title: "Lock", Description: "Instant Windows lock screen (Win + L)", Icon: "🔒"},
		},
		menuIndex: 0,
	}
	rendered := m.renderMenu()

	lines := strings.Split(strings.TrimSpace(rendered), "\n")
	if len(lines) != 4 {
		t.Fatalf("expected 4 lines in menu rendering (header + 3 items), got %d:\n%s", len(lines), rendered)
	}
	if !strings.Contains(lines[1], "Open RustDesk") {
		t.Fatalf("item 1 should contain full title on the same line, got: %q", lines[1])
	}
}

func TestCardWidthResponsiveConsistency(t *testing.T) {
	m := Model{}
	// Default when width is 0
	if w := m.cardWidth(); w != 80 {
		t.Fatalf("expected default cardWidth 80, got %d", w)
	}

	// Narrow window (< 74)
	m.width = 60
	if w := m.cardWidth(); w != 74 {
		t.Fatalf("expected clamped min cardWidth 74, got %d", w)
	}

	// Standard window (84) -> 84 - 4 = 80
	m.width = 84
	if w := m.cardWidth(); w != 80 {
		t.Fatalf("expected cardWidth 80 for width 84, got %d", w)
	}

	// Large window (> 104) -> clamped to 100
	m.width = 120
	if w := m.cardWidth(); w != 100 {
		t.Fatalf("expected clamped max cardWidth 100, got %d", w)
	}
}

func TestAllViewsConsistentCardWidth(t *testing.T) {
	m := Model{width: 90}
	expectedWidth := m.cardWidth() + 2 // card width + rounded border

	vDoctor := m.renderDoctorView()
	vQuick := m.renderQuickSend()
	vCombo := m.renderComboInput()
	vLogin := m.renderLoginPrompt()
	vReg := m.renderRegisterPlatform()

	for name, view := range map[string]string{
		"doctor":   vDoctor,
		"quick":    vQuick,
		"combo":    vCombo,
		"login":    vLogin,
		"register": vReg,
	} {
		actualWidth := lipgloss.Width(view)
		if actualWidth != expectedWidth {
			t.Errorf("view %s has width %d, expected %d", name, actualWidth, expectedWidth)
		}
	}
}

func TestSnippetStudioAndComboStudioConsistentWidth(t *testing.T) {
	cStudio := NewComboStudioModel(nil, nil)
	cStudio.width = 90
	sStudio := NewSnippetStudioModel(nil)
	sStudio.width = 90

	cView := cStudio.View()
	sView := sStudio.View()

	cWidth := lipgloss.Width(cView)
	sWidth := lipgloss.Width(sView)

	if cWidth != sWidth {
		t.Fatalf("ComboStudio width (%d) and SnippetStudio width (%d) should be identical", cWidth, sWidth)
	}
}

func TestStudioWindowResizeNoPanic(t *testing.T) {
	m := Model{
		state:       StateComboStudio,
		comboStudio: &ComboStudioModel{},
	}
	// snippetStudio is nil; must not panic
	newM, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	model := newM.(Model)
	if model.comboStudio.width != 100 {
		t.Fatalf("expected comboStudio width 100, got %d", model.comboStudio.width)
	}

	m2 := Model{
		state:         StateSnippetStudio,
		snippetStudio: &SnippetStudioModel{},
	}
	// comboStudio is nil; must not panic
	newM2, _ := m2.Update(tea.WindowSizeMsg{Width: 110, Height: 50})
	model2 := newM2.(Model)
	if model2.snippetStudio.width != 110 {
		t.Fatalf("expected snippetStudio width 110, got %d", model2.snippetStudio.width)
	}
}

func TestPhoneDisplayName(t *testing.T) {
	// 1. cfg.PhoneName override
	m1 := Model{
		cfg:         config.Config{PhoneName: "Custom Phone Name"},
		deviceModel: "Pixel 7",
		device:      &adb.Device{Model: "KB2001"},
	}
	if got := m1.phoneDisplayName(); got != "Custom Phone Name" {
		t.Errorf("expected cfg.PhoneName override, got %q", got)
	}

	// 2. deviceModel takes precedence over raw hardware model
	m2 := Model{
		deviceModel: "SJ's OnePlus 8T",
		device:      &adb.Device{Model: "KB2001"},
	}
	if got := m2.phoneDisplayName(); got != "SJ's OnePlus 8T" {
		t.Errorf("expected deviceModel, got %q", got)
	}

	// 3. device.Model used when deviceModel is empty or generic
	m3 := Model{
		deviceModel: "Android Device",
		device:      &adb.Device{Model: "KB2001"},
	}
	if got := m3.phoneDisplayName(); got != "KB2001" {
		t.Errorf("expected device.Model fallback, got %q", got)
	}

	// 4. Default fallback
	m4 := Model{}
	if got := m4.phoneDisplayName(); got != "GhostHID Phone" {
		t.Errorf("expected default GhostHID Phone, got %q", got)
	}
}

func TestGuideViewDynamicPhoneName(t *testing.T) {
	m := Model{
		deviceModel: "SJ's OnePlus 8T",
		regPlatform: "macos",
		width:       80,
	}
	guideMac := m.renderRegisterGuide()
	if !strings.Contains(guideMac, "SJ's OnePlus 8T") {
		t.Errorf("macOS guide missing dynamic phone name: %s", guideMac)
	}

	m.regPlatform = "windows"
	guideWin := m.renderRegisterGuide()
	if !strings.Contains(guideWin, "SJ's OnePlus 8T") {
		t.Errorf("windows guide missing dynamic phone name: %s", guideWin)
	}
}

func TestDeviceRegisterPickerView(t *testing.T) {
	m := Model{
		state: StateDeviceRegisterPicker,
		width: 80,
		pickerDevices: []adb.BondedDevice{
			{Address: "A0:9A:8E:57:36:DA", Name: "Shubham’s MacBook Air", Type: "DUAL"},
			{Address: "38:FC:98:E8:E3:87", Name: "LP-TZD-0X114650", Type: "BR/EDR"},
		},
		pickerIndex: 0,
	}

	view := m.renderRegisterPicker()
	if !strings.Contains(view, "Shubham’s MacBook Air") {
		t.Errorf("expected MacBook Air in picker view, got: %s", view)
	}
	if !strings.Contains(view, "A0:9A:8E:57:36:DA") {
		t.Errorf("expected MAC address in picker view, got: %s", view)
	}
	if !strings.Contains(view, "Manual MAC address entry...") {
		t.Errorf("expected manual option in picker view, got: %s", view)
	}
}

func TestDeviceRegisterPickerNavigation(t *testing.T) {
	devName := textinput.New()
	devAddr := textinput.New()
	m := Model{
		devNameInput: devName,
		devAddrInput: devAddr,
		state:        StateDeviceRegisterPicker,
		width:        80,
		pickerDevices: []adb.BondedDevice{
			{Address: "A0:9A:8E:57:36:DA", Name: "Shubham’s MacBook Air", Type: "DUAL"},
			{Address: "38:FC:98:E8:E3:87", Name: "LP-TZD-0X114650", Type: "BR/EDR"},
		},
		pickerIndex: 0,
		regPlatform: "macos",
	}

	// Down arrow moves to item 1
	mUpdated, _ := m.updateRegisterPicker(tea.KeyMsg{Type: tea.KeyDown})
	m = mUpdated.(Model)
	if m.pickerIndex != 1 {
		t.Errorf("expected pickerIndex=1, got %d", m.pickerIndex)
	}

	// Down arrow moves to manual option (index 2)
	mUpdated, _ = m.updateRegisterPicker(tea.KeyMsg{Type: tea.KeyDown})
	m = mUpdated.(Model)
	if m.pickerIndex != 2 {
		t.Errorf("expected pickerIndex=2, got %d", m.pickerIndex)
	}

	// Enter on manual option opens input form
	mUpdated, _ = m.updateRegisterPicker(tea.KeyMsg{Type: tea.KeyEnter})
	m = mUpdated.(Model)
	if m.state != StateDeviceRegisterInput {
		t.Errorf("expected StateDeviceRegisterInput, got %v", m.state)
	}

	// Esc from input goes back to picker
	mUpdated, _ = m.updateRegisterInput(tea.KeyMsg{Type: tea.KeyEsc})
	m = mUpdated.(Model)
	if m.state != StateDeviceRegisterPicker {
		t.Errorf("expected StateDeviceRegisterPicker after Esc from input, got %v", m.state)
	}

	// Select first device and press Enter -> registers and connects
	m.pickerIndex = 0
	mUpdated, _ = m.updateRegisterPicker(tea.KeyMsg{Type: tea.KeyEnter})
	m = mUpdated.(Model)
	if m.state != StateDevicesList {
		t.Errorf("expected StateDevicesList after selecting device, got %v", m.state)
	}
	if len(m.cfg.Devices) != 1 || m.cfg.Devices[0].Address != "A0:9A:8E:57:36:DA" {
		t.Errorf("expected registered device in config, got %+v", m.cfg.Devices)
	}
}

func TestTelemetryCardEmptyWhenNoDevice(t *testing.T) {
	m := Model{
		width: 80,
		cfg:   config.DefaultConfig(),
	}
	card := m.renderTelemetryCard()
	if strings.Contains(card, "LP-TZD") || strings.Contains(card, "38:FC:98:E8:E3:87") {
		t.Fatalf("telemetry card should NOT display dummy device when no devices registered:\n%s", card)
	}
	if !strings.Contains(card, "None (press [d]") {
		t.Fatalf("telemetry card should prompt to pair/select when empty:\n%s", card)
	}
}
