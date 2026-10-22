package hid

import "testing"

func TestParseActionSequence(t *testing.T) {
	steps, err := ParseActionSequence("win+r ->{0.4} click-left ->{0.1} scroll-up")
	if err != nil {
		t.Fatalf("ParseActionSequence failed: %v", err)
	}
	if len(steps) != 3 {
		t.Fatalf("expected 3 steps, got %d", len(steps))
	}
	if steps[0].Mouse != nil || steps[0].Modifier != ModLeftGUI {
		t.Errorf("first step should be the win+r keyboard chord: %+v", steps[0])
	}
	if steps[1].Mouse == nil || steps[1].Mouse.Name != MouseClickLeft || steps[1].Mouse.Buttons != 0x01 {
		t.Errorf("second step should be a left click: %+v", steps[1])
	}
	if steps[2].Mouse == nil || steps[2].Mouse.Wheel != 1 {
		t.Errorf("third step should scroll up: %+v", steps[2])
	}
	if steps[1].Delay != 100_000_000 {
		t.Errorf("click delay = %v, want 100ms", steps[1].Delay)
	}

	if _, err := ParseActionSequence("lclick"); err != nil {
		t.Errorf("mouse alias rejected: %v", err)
	}
	if _, err := ParseActionSequence("click:left"); err == nil {
		t.Error("expected click:left to be rejected with a helpful error")
	}
}

func TestWindowsKeyTokens(t *testing.T) {
	accepted := []string{"win", "ctrl", "alt", "shift", "a", "9", "f12", "printscreen", "capslock", "menu", "semicolon", "pgdn"}
	for _, token := range accepted {
		if !IsWindowsKeyToken(token) {
			t.Errorf("IsWindowsKeyToken(%q) = false, want true", token)
		}
	}

	rejected := []string{"cmd", "super", "meta", "option", "opt", "hyper", "f13", "ä", ""}
	for _, token := range rejected {
		if IsWindowsKeyToken(token) {
			t.Errorf("IsWindowsKeyToken(%q) = true, want false", token)
		}
	}
}

func TestParseWindowsPunctuation(t *testing.T) {
	tests := map[string]byte{
		"capslock":    KeyCapsLock,
		"numlock":     KeyNumLock,
		"printscreen": KeyPrintScr,
		"menu":        KeyMenu,
		"leftbracket": KeyLBracket,
		"semicolon":   KeySemicolon,
		"grave":       KeyGrave,
		"slash":       KeySlash,
	}
	for input, want := range tests {
		_, got, err := ParseCombo(input)
		if err != nil {
			t.Fatalf("ParseCombo(%q) error: %v", input, err)
		}
		if got != want {
			t.Errorf("ParseCombo(%q) keycode = 0x%02X, want 0x%02X", input, got, want)
		}
	}
}

func TestParseTextSteps(t *testing.T) {
	steps, err := ParseActionSequence(`win+s ->{0.6} type:rustdesk ->{0.3} enter`)
	if err != nil {
		t.Fatalf("ParseActionSequence failed: %v", err)
	}
	if len(steps) != 3 {
		t.Fatalf("expected 3 steps, got %d", len(steps))
	}
	if steps[1].Text != "rustdesk" {
		t.Errorf("text step = %q, want rustdesk", steps[1].Text)
	}
	if steps[1].Delay != 300_000_000 {
		t.Errorf("text delay = %v, want 300ms", steps[1].Delay)
	}

	quoted, err := ParseActionSequence(`type:"rust desk" -> text:'hello world'`)
	if err != nil {
		t.Fatalf("quoted text parse failed: %v", err)
	}
	if len(quoted) != 2 || quoted[0].Text != "rust desk" || quoted[1].Text != "hello world" {
		t.Errorf("quoted text parsed as %+v", quoted)
	}

	if _, err := ParseActionSequence("type:"); err == nil {
		t.Error("empty type step should fail")
	}
	if _, err := ParseActionSequence(`type:"unterminated`); err == nil {
		t.Error("unterminated quote should fail")
	}
}
