package hid

import (
	"testing"
	"time"
)

func TestParseCombo(t *testing.T) {
	tests := []struct {
		input       string
		wantMod     byte
		wantKey     byte
		expectError bool
	}{
		{input: "win+l", wantMod: ModLeftGUI, wantKey: KeyA + ('l' - 'a'), expectError: false},
		{input: "super+l", wantMod: ModLeftGUI, wantKey: KeyA + ('l' - 'a'), expectError: false},
		{input: "cmd+l", wantMod: ModLeftGUI, wantKey: KeyA + ('l' - 'a'), expectError: false},
		{input: "super", wantMod: ModLeftGUI, wantKey: 0, expectError: false},
		{input: "cmd", wantMod: ModLeftGUI, wantKey: 0, expectError: false},
		{input: "ctrl+alt+del", wantMod: ModLeftCtrl | ModLeftAlt, wantKey: KeyDelFwd, expectError: false},
		{input: "win+x", wantMod: ModLeftGUI, wantKey: KeyA + ('x' - 'a'), expectError: false},
		{input: "u", wantMod: 0, wantKey: KeyA + ('u' - 'a'), expectError: false},
		{input: "s", wantMod: 0, wantKey: KeyA + ('s' - 'a'), expectError: false},
		{input: "esc", wantMod: 0, wantKey: KeyEscape, expectError: false},
		{input: "escape", wantMod: 0, wantKey: KeyEscape, expectError: false},
		{input: "tab", wantMod: 0, wantKey: KeyTab, expectError: false},
		{input: "enter", wantMod: 0, wantKey: KeyEnter, expectError: false},
		{input: "return", wantMod: 0, wantKey: KeyEnter, expectError: false},
		{input: "shift+tab", wantMod: ModLeftShift, wantKey: KeyTab, expectError: false},
		{input: "cmd+space", wantMod: ModLeftGUI, wantKey: KeySpace, expectError: false},
		{input: "backspace", wantMod: 0, wantKey: KeyDelete, expectError: false},
		{input: "up", wantMod: 0, wantKey: KeyUp, expectError: false},
		{input: "down", wantMod: 0, wantKey: KeyDown, expectError: false},
		{input: "left", wantMod: 0, wantKey: KeyLeft, expectError: false},
		{input: "right", wantMod: 0, wantKey: KeyRight, expectError: false},
		{input: "f5", wantMod: 0, wantKey: KeyF5, expectError: false},
		{input: "f12", wantMod: 0, wantKey: KeyF12, expectError: false},
		{input: "", expectError: true},
		{input: "unknownkeyname", expectError: true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			mod, key, err := ParseCombo(tt.input)
			if (err != nil) != tt.expectError {
				t.Fatalf("ParseCombo(%q) error = %v, expectError = %v", tt.input, err, tt.expectError)
			}
			if !tt.expectError {
				if mod != tt.wantMod {
					t.Errorf("ParseCombo(%q) modifier = 0x%02X, want 0x%02X", tt.input, mod, tt.wantMod)
				}
				if key != tt.wantKey {
					t.Errorf("ParseCombo(%q) keyCode = 0x%02X, want 0x%02X", tt.input, key, tt.wantKey)
				}
			}
		})
	}
}

func TestNormalizeComboForAndroid(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"super+l", "win+l"},
		{"cmd+space", "win+space"},
		{"super", "win"},
		{"escape", "esc"},
		{"control+alt+delete", "ctrl+alt+del"},
		{"return", "enter"},
	}

	for _, tt := range tests {
		got := NormalizeComboForAndroid(tt.input)
		if got != tt.want {
			t.Errorf("NormalizeComboForAndroid(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestParseComboSequence(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expectError bool
		wantSteps   []ComboStep
	}{
		{
			name:        "single combo without arrow",
			input:       "super+l",
			expectError: false,
			wantSteps: []ComboStep{
				{Combo: "super+l", Modifier: ModLeftGUI, KeyCode: KeyA + ('l' - 'a'), Delay: 0},
			},
		},
		{
			name:        "esc with 0.9s delay then super+l",
			input:       "esc ->{0.9} super+l",
			expectError: false,
			wantSteps: []ComboStep{
				{Combo: "esc", Modifier: 0, KeyCode: KeyEscape, Delay: 900 * time.Millisecond},
				{Combo: "super+l", Modifier: ModLeftGUI, KeyCode: KeyA + ('l' - 'a'), Delay: 0},
			},
		},
		{
			name:        "user syntax: esc >{0.9}. super+l",
			input:       "esc >{0.9}. super+l",
			expectError: false,
			wantSteps: []ComboStep{
				{Combo: "esc", Modifier: 0, KeyCode: KeyEscape, Delay: 900 * time.Millisecond},
				{Combo: "super+l", Modifier: ModLeftGUI, KeyCode: KeyA + ('l' - 'a'), Delay: 0},
			},
		},
		{
			name:        "user syntax: esc ->{0.9) super+l",
			input:       "esc ->{0.9) super+l",
			expectError: false,
			wantSteps: []ComboStep{
				{Combo: "esc", Modifier: 0, KeyCode: KeyEscape, Delay: 900 * time.Millisecond},
				{Combo: "super+l", Modifier: ModLeftGUI, KeyCode: KeyA + ('l' - 'a'), Delay: 0},
			},
		},
		{
			name:        "plain arrow with default 1.0s delay",
			input:       "esc -> super+l",
			expectError: false,
			wantSteps: []ComboStep{
				{Combo: "esc", Modifier: 0, KeyCode: KeyEscape, Delay: 1000 * time.Millisecond},
				{Combo: "super+l", Modifier: ModLeftGUI, KeyCode: KeyA + ('l' - 'a'), Delay: 0},
			},
		},
		{
			name:        "trailing delay after combo",
			input:       "super+l ->{0.9}",
			expectError: false,
			wantSteps: []ComboStep{
				{Combo: "super+l", Modifier: ModLeftGUI, KeyCode: KeyA + ('l' - 'a'), Delay: 900 * time.Millisecond},
			},
		},
		{
			name:        "multi-step sleep sequence",
			input:       "esc ->{0.3} esc ->{0.4} win+x ->{0.6} u ->{0.4} s",
			expectError: false,
			wantSteps: []ComboStep{
				{Combo: "esc", Modifier: 0, KeyCode: KeyEscape, Delay: 300 * time.Millisecond},
				{Combo: "esc", Modifier: 0, KeyCode: KeyEscape, Delay: 400 * time.Millisecond},
				{Combo: "win+x", Modifier: ModLeftGUI, KeyCode: KeyA + ('x' - 'a'), Delay: 600 * time.Millisecond},
				{Combo: "u", Modifier: 0, KeyCode: KeyA + ('u' - 'a'), Delay: 400 * time.Millisecond},
				{Combo: "s", Modifier: 0, KeyCode: KeyA + ('s' - 'a'), Delay: 0},
			},
		},
		{
			name:        "empty sequence error",
			input:       "",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			steps, err := ParseComboSequence(tt.input)
			if (err != nil) != tt.expectError {
				t.Fatalf("ParseComboSequence(%q) error = %v, expectError = %v", tt.input, err, tt.expectError)
			}
			if !tt.expectError {
				if len(steps) != len(tt.wantSteps) {
					t.Fatalf("ParseComboSequence(%q) got %d steps, want %d", tt.input, len(steps), len(tt.wantSteps))
				}
				for i := range steps {
					if steps[i].Combo != tt.wantSteps[i].Combo {
						t.Errorf("step %d combo = %q, want %q", i, steps[i].Combo, tt.wantSteps[i].Combo)
					}
					if steps[i].Modifier != tt.wantSteps[i].Modifier {
						t.Errorf("step %d modifier = 0x%02X, want 0x%02X", i, steps[i].Modifier, tt.wantSteps[i].Modifier)
					}
					if steps[i].KeyCode != tt.wantSteps[i].KeyCode {
						t.Errorf("step %d keyCode = 0x%02X, want 0x%02X", i, steps[i].KeyCode, tt.wantSteps[i].KeyCode)
					}
					if steps[i].Delay != tt.wantSteps[i].Delay {
						t.Errorf("step %d delay = %v, want %v", i, steps[i].Delay, tt.wantSteps[i].Delay)
					}
				}
			}
		})
	}
}

func TestValidateRaw(t *testing.T) {
	tests := []struct {
		hexStr   string
		wantLen  int
		wantID   int
	}{
		{"02", 1, ReportIDSystem},
		{"cd00", 2, ReportIDConsumer},
		{"01000000", 4, ReportIDMouse},
		{"08000f0000000000", 8, ReportIDKeyboard},
	}

	for _, tt := range tests {
		b, err := ValidateRaw(tt.hexStr)
		if err != nil {
			t.Fatalf("ValidateRaw(%q) unexpected error: %v", tt.hexStr, err)
		}
		if len(b) != tt.wantLen {
			t.Errorf("ValidateRaw(%q) got %d bytes, want %d", tt.hexStr, len(b), tt.wantLen)
		}
		id, err := ReportIDForLength(len(b))
		if err != nil {
			t.Fatalf("ReportIDForLength(%d) unexpected error: %v", len(b), err)
		}
		if id != tt.wantID {
			t.Errorf("ReportIDForLength(%d) = %d, want %d", len(b), id, tt.wantID)
		}
	}

	// Invalid lengths (3 bytes = 6 hex, 5 bytes = 10 hex, etc.)
	invalidLengths := []string{"08000f", "08000f0000", "010"}
	for _, inv := range invalidLengths {
		if _, err := ValidateRaw(inv); err == nil {
			t.Errorf("expected error for invalid hex length %q", inv)
		}
	}

	// Invalid characters
	if _, err := ValidateRaw("zzzzzzzzzzzzzzzz"); err == nil {
		t.Errorf("expected error for non-hex characters")
	}
}

func TestKeyReport(t *testing.T) {
	rep := NewKeyReport(ModLeftGUI, KeyA+('l'-'a'))
	hexStr := rep.HexString()
	if hexStr != "08000f0000000000" {
		t.Errorf("NewKeyReport HexString() = %s, want 08000f0000000000", hexStr)
	}
}
