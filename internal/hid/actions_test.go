package hid

import (
	"reflect"
	"testing"
)

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

func TestParseActionSequenceRaw(t *testing.T) {
	steps, err := ParseActionSequence("raw:02 ->{0.06} raw:00 -> raw:cd00 ->{0.05} raw:0000")
	if err != nil {
		t.Fatalf("ParseActionSequence raw failed: %v", err)
	}
	if len(steps) != 4 {
		t.Fatalf("expected 4 steps, got %d", len(steps))
	}
	if steps[0].Raw != "02" {
		t.Errorf("expected raw:02, got %q", steps[0].Raw)
	}
	if steps[1].Raw != "00" {
		t.Errorf("expected raw:00, got %q", steps[1].Raw)
	}
	if steps[2].Raw != "cd00" {
		t.Errorf("expected raw:cd00, got %q", steps[2].Raw)
	}
	if steps[3].Raw != "0000" {
		t.Errorf("expected raw:0000, got %q", steps[3].Raw)
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

func TestParseVariables(t *testing.T) {
	// Variable without type: prefix
	seq := "ctrl+alt+del ->{1.0} esc ->{0.5} $USERNAME ->{0.2} tab ->{0.2} ${PASSWORD} ->{0.2} enter"
	steps, err := ParseActionSequence(seq)
	if err != nil {
		t.Fatalf("ParseActionSequence failed for variables: %v", err)
	}
	if len(steps) != 6 {
		t.Fatalf("expected 6 steps, got %d", len(steps))
	}
	if steps[2].Variable != "USERNAME" {
		t.Errorf("step 2 variable = %q, want USERNAME", steps[2].Variable)
	}
	if steps[4].Variable != "PASSWORD" {
		t.Errorf("step 4 variable = %q, want PASSWORD", steps[4].Variable)
	}

	// Variable with type: prefix
	typeSeq := "type:$USERNAME -> type:${PASSWORD}"
	typeSteps, err := ParseActionSequence(typeSeq)
	if err != nil {
		t.Fatalf("ParseActionSequence failed for type variables: %v", err)
	}
	if len(typeSteps) != 2 {
		t.Fatalf("expected 2 steps, got %d", len(typeSteps))
	}
	if typeSteps[0].Variable != "USERNAME" || typeSteps[1].Variable != "PASSWORD" {
		t.Errorf("unexpected variables in type: steps: %+v", typeSteps)
	}

	// Extract variables
	vars := ExtractVariables(seq)
	want := []string{"USERNAME", "PASSWORD"}
	if !reflect.DeepEqual(vars, want) {
		t.Errorf("ExtractVariables = %v, want %v", vars, want)
	}

	// DescribeSequence
	desc, err := DescribeSequence("$USERNAME")
	if err != nil || len(desc) != 1 {
		t.Fatalf("DescribeSequence failed: %v", err)
	}
	if desc[0].Type != "variable" || desc[0].Value != "$USERNAME" {
		t.Errorf("unexpected description: %+v", desc[0])
	}
}

func TestDirectivesSequence(t *testing.T) {
	seq := "win+x ->{0.5} u ->{0.3} s ->{5.0} !DISCONNECT ->{0.5} !EXIT"
	steps, err := ParseActionSequence(seq)
	if err != nil {
		t.Fatalf("ParseActionSequence failed: %v", err)
	}
	if len(steps) != 5 {
		t.Fatalf("expected 5 steps, got %d", len(steps))
	}
	if steps[3].Directive != "!DISCONNECT" {
		t.Errorf("expected !DISCONNECT, got %q", steps[3].Directive)
	}
	if steps[4].Directive != "!EXIT" {
		t.Errorf("expected !EXIT, got %q", steps[4].Directive)
	}

	// DescribeSequence
	desc, err := DescribeSequence(seq)
	if err != nil {
		t.Fatalf("DescribeSequence failed: %v", err)
	}
	if desc[3].Type != "directive" || desc[3].Value != "!DISCONNECT" {
		t.Errorf("unexpected desc[3]: %+v", desc[3])
	}
	if desc[4].Type != "directive" || desc[4].Value != "!EXIT" {
		t.Errorf("unexpected desc[4]: %+v", desc[4])
	}

	// Directives with target
	connSeq := "!CONNECT:38:FC:98:E8:E3:87 ->{1.0} !COMBO:SEND:lock"
	connSteps, err := ParseActionSequence(connSeq)
	if err != nil {
		t.Fatalf("ParseActionSequence connSeq failed: %v", err)
	}
	if connSteps[0].Directive != "!CONNECT:38:FC:98:E8:E3:87" {
		t.Errorf("expected !CONNECT:38:FC:98:E8:E3:87, got %q", connSteps[0].Directive)
	}
	if connSteps[1].Directive != "!COMBO:SEND:lock" {
		t.Errorf("expected !COMBO:SEND:lock, got %q", connSteps[1].Directive)
	}

	// Invalid directive
	if _, err := ParseActionSequence("!UNKNOWN"); err == nil {
		t.Errorf("expected error on !UNKNOWN directive")
	}
}
