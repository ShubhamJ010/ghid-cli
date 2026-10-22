package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/ghost-hid/ghid/internal/adb"
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
