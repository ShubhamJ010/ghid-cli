package hid

import (
	"testing"
	"time"
)

func TestParseSequenceCanonicalExample(t *testing.T) {
	input := "samxn81@gmail.com{Tab}{PASSWORD}{Enter}{500ms}{mod+s}{100ms}{WIN_SLEEP}"
	steps, err := ParseSequence(input)
	if err != nil {
		t.Fatalf("unexpected error parsing canonical sequence: %v", err)
	}

	if len(steps) != 8 {
		t.Fatalf("expected 8 steps, got %d", len(steps))
	}

	// 0: plain text "samxn81@gmail.com"
	if steps[0].Text != "samxn81@gmail.com" {
		t.Errorf("step 0: expected text 'samxn81@gmail.com', got %q", steps[0].Text)
	}

	// 1: {Tab}
	if steps[1].KeyCode != KeyTab {
		t.Errorf("step 1: expected KeyTab (0x%02x), got 0x%02x", KeyTab, steps[1].KeyCode)
	}

	// 2: {PASSWORD}
	if steps[2].Payload != "PASSWORD" {
		t.Errorf("step 2: expected payload 'PASSWORD', got %q", steps[2].Payload)
	}

	// 3: {Enter}
	if steps[3].KeyCode != KeyEnter {
		t.Errorf("step 3: expected KeyEnter (0x%02x), got 0x%02x", KeyEnter, steps[3].KeyCode)
	}

	// 4: {500ms}
	if steps[4].Delay != 500*time.Millisecond {
		t.Errorf("step 4: expected delay 500ms, got %v", steps[4].Delay)
	}

	// 5: {mod+s}
	if steps[5].KeyCode != KeyS {
		t.Errorf("step 5: expected KeyS, got 0x%02x", steps[5].KeyCode)
	}
	if steps[5].Modifier == 0 {
		t.Errorf("step 5: expected non-zero modifier for mod+s, got 0")
	}

	// 6: {100ms}
	if steps[6].Delay != 100*time.Millisecond {
		t.Errorf("step 6: expected delay 100ms, got %v", steps[6].Delay)
	}

	// 7: {WIN_SLEEP}
	if steps[7].Payload != "WIN_SLEEP" {
		t.Errorf("step 7: expected payload 'WIN_SLEEP', got %q", steps[7].Payload)
	}
}

func TestParseSequenceLiteralBraces(t *testing.T) {
	input := "hello{{world}}"
	steps, err := ParseSequence(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(steps))
	}
	if steps[0].Text != "hello{world}}" {
		t.Errorf("expected 'hello{world}}', got %q", steps[0].Text)
	}
}

func TestExtractPayloads(t *testing.T) {
	input := "samxn81@gmail.com{Tab}{PASSWORD}{Enter}{500ms}{mod+s}{100ms}{WIN_SLEEP}"
	payloads := ExtractPayloads(input)
	if len(payloads) != 2 {
		t.Fatalf("expected 2 payloads, got %d", len(payloads))
	}
	if payloads[0] != "PASSWORD" || payloads[1] != "WIN_SLEEP" {
		t.Errorf("unexpected payloads: %v", payloads)
	}
}
