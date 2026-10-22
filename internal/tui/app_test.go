package tui

import (
	"testing"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
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
