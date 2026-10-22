package hid

import (
	"fmt"
	"strings"
)

// MouseAction describes a pointer gesture that is transmitted as its own HID
// mouse report. Kept separate from keyboard chords because Windows interprets
// mouse buttons and wheel ticks on a different HID report ID.
type MouseAction struct {
	Name    string // canonical token, e.g. "click-left"
	Buttons byte   // HID button bitmap (bit 0 = left, bit 1 = right, bit 2 = middle)
	Wheel   int8   // +1 scroll up, -1 scroll down
	Double  bool   // repeat the click twice
}

// Canonical mouse gestures understood by the Windows gateway.
const (
	MouseClickLeft   = "click-left"
	MouseClickRight  = "click-right"
	MouseClickMiddle = "click-middle"
	MouseDoubleClick = "double-click"
	MouseScrollUp    = "scroll-up"
	MouseScrollDown  = "scroll-down"
)

var mouseAliases = map[string]string{
	"click-left":   MouseClickLeft,
	"left-click":   MouseClickLeft,
	"leftclick":    MouseClickLeft,
	"lclick":       MouseClickLeft,
	"click-right":  MouseClickRight,
	"right-click":  MouseClickRight,
	"rightclick":   MouseClickRight,
	"rclick":       MouseClickRight,
	"click-middle": MouseClickMiddle,
	"middle-click": MouseClickMiddle,
	"middleclick":  MouseClickMiddle,
	"mclick":       MouseClickMiddle,
	"double-click": MouseDoubleClick,
	"doubleclick":  MouseDoubleClick,
	"dblclick":     MouseDoubleClick,
	"scroll-up":    MouseScrollUp,
	"scrollup":     MouseScrollUp,
	"wheel-up":     MouseScrollUp,
	"scroll-down":  MouseScrollDown,
	"scrolldown":   MouseScrollDown,
	"wheel-down":   MouseScrollDown,
}

// MouseActions returns every canonical mouse token for help and validation.
func MouseActions() []string {
	return []string{
		MouseClickLeft,
		MouseClickRight,
		MouseClickMiddle,
		MouseDoubleClick,
		MouseScrollUp,
		MouseScrollDown,
	}
}

// ParseMouseAction resolves a mouse token. The boolean reports whether the
// token looked like a mouse gesture at all; an unknown click/scroll token
// returns true with an error so validation can point at the typo.
func ParseMouseAction(token string) (MouseAction, bool, error) {
	normalized := strings.ToLower(strings.TrimSpace(token))
	if normalized == "" {
		return MouseAction{}, false, nil
	}

	if canonical, ok := mouseAliases[normalized]; ok {
		return mouseActionFor(canonical), true, nil
	}

	if strings.HasPrefix(normalized, "click:") {
		return MouseAction{}, true, fmt.Errorf("unsupported mouse button: %q (use click-left, click-right, click-middle)", token)
	}
	if strings.HasPrefix(normalized, "scroll:") || strings.HasPrefix(normalized, "wheel:") {
		return MouseAction{}, true, fmt.Errorf("unsupported scroll direction: %q (use scroll-up or scroll-down)", token)
	}

	return MouseAction{}, false, nil
}

func mouseActionFor(canonical string) MouseAction {
	switch canonical {
	case MouseClickLeft:
		return MouseAction{Name: canonical, Buttons: 0x01}
	case MouseClickRight:
		return MouseAction{Name: canonical, Buttons: 0x02}
	case MouseClickMiddle:
		return MouseAction{Name: canonical, Buttons: 0x04}
	case MouseDoubleClick:
		return MouseAction{Name: canonical, Buttons: 0x01, Double: true}
	case MouseScrollUp:
		return MouseAction{Name: canonical, Wheel: 1}
	case MouseScrollDown:
		return MouseAction{Name: canonical, Wheel: -1}
	default:
		return MouseAction{Name: canonical}
	}
}

// ParseActionSequence parses a combo in which every step is either a Windows
// keyboard chord or a mouse gesture. Steps are joined with "->" and may carry
// a delay, e.g. "win+r ->{0.9} tab ->{0.2} click-left".
func ParseActionSequence(input string) ([]ComboStep, error) {
	return parseSequence(input, parseActionSegment)
}

func parseActionSegment(segment string) (ComboStep, error) {
	if text, ok, err := parseTextStep(segment); ok {
		if err != nil {
			return ComboStep{}, err
		}
		return ComboStep{Combo: segment, Text: text}, nil
	}

	if action, ok, err := ParseMouseAction(segment); ok {
		if err != nil {
			return ComboStep{}, err
		}
		return ComboStep{Combo: action.Name, Mouse: &action}, nil
	}

	mod, code, err := ParseCombo(segment)
	if err != nil {
		return ComboStep{}, err
	}
	return ComboStep{Combo: segment, Modifier: mod, KeyCode: code}, nil
}

// parseTextStep recognises literal typing steps: type:rustdesk,
// type:"rust desk", text:hello or text 'hello world'.
func parseTextStep(segment string) (string, bool, error) {
	lower := strings.ToLower(strings.TrimSpace(segment))
	var rest string

	switch {
	case strings.HasPrefix(lower, "type:"):
		rest = segment[strings.Index(segment, ":")+1:]
	case strings.HasPrefix(lower, "text:"):
		rest = segment[strings.Index(segment, ":")+1:]
	case strings.HasPrefix(lower, "type "):
		rest = segment[len("type "):]
	case strings.HasPrefix(lower, "text "):
		rest = segment[len("text "):]
	default:
		return "", false, nil
	}

	rest = strings.TrimSpace(rest)
	if len(rest) >= 2 && ((rest[0] == '"' && rest[len(rest)-1] == '"') || (rest[0] == '\'' && rest[len(rest)-1] == '\'')) {
		rest = rest[1 : len(rest)-1]
	} else if len(rest) > 0 && (rest[0] == '"' || rest[0] == '\'') {
		return "", true, fmt.Errorf("unterminated quoted text in %q", segment)
	}

	if rest == "" {
		return "", true, fmt.Errorf("type step needs text, e.g. type:rustdesk or type:\"rust desk\"")
	}
	return rest, true, nil
}

// IsWindowsKeyToken reports whether a single token names a key that exists on
// a real Windows keyboard. It intentionally rejects macOS-only aliases such as
// cmd, super, meta, option and opt.
func IsWindowsKeyToken(token string) bool {
	p := strings.ToLower(strings.TrimSpace(token))
	if p == "" {
		return false
	}

	switch p {
	case "win", "gui", "ctrl", "control", "alt", "shift":
		return true
	case "backspace", "esc", "escape", "enter", "return", "tab", "space":
		return true
	case "up", "down", "left", "right", "home", "end", "pageup", "pgup", "pagedown", "pgdn":
		return true
	case "insert", "del", "delete", "capslock", "caps", "numlock", "scrolllock", "scroll":
		return true
	case "printscreen", "prtsc", "print", "pause", "break", "menu", "apps", "context":
		return true
	case "minus", "equal", "equals", "leftbracket", "lbracket", "rightbracket", "rbracket":
		return true
	case "backslash", "semicolon", "quote", "apostrophe", "grave", "backtick", "tilde":
		return true
	case "comma", "period", "dot", "slash", "forwardslash":
		return true
	}

	if len(p) == 1 {
		return (p[0] >= 'a' && p[0] <= 'z') || (p[0] >= '0' && p[0] <= '9')
	}

	if strings.HasPrefix(p, "f") && len(p) >= 2 && len(p) <= 3 {
		n := 0
		for _, r := range p[1:] {
			if r < '0' || r > '9' {
				return false
			}
			n = n*10 + int(r-'0')
		}
		return n >= 1 && n <= 12
	}

	return false
}

// WindowsKeyboardTokens lists every key token the combo grammar accepts.
func WindowsKeyboardTokens() []string {
	tokens := []string{
		"win", "ctrl", "alt", "shift",
		"enter", "esc", "tab", "space", "backspace", "del", "insert",
		"up", "down", "left", "right", "home", "end", "pgup", "pgdn",
		"printscreen", "capslock", "numlock", "scrolllock", "pause", "menu",
		"minus", "equal", "leftbracket", "rightbracket", "backslash",
		"semicolon", "quote", "grave", "comma", "period", "slash",
	}
	for c := 'a'; c <= 'z'; c++ {
		tokens = append(tokens, string(c))
	}
	for c := '0'; c <= '9'; c++ {
		tokens = append(tokens, string(c))
	}
	for i := 1; i <= 12; i++ {
		tokens = append(tokens, fmt.Sprintf("f%d", i))
	}
	return tokens
}

// StepDescription is a machine-readable view of one parsed combo step.
type StepDescription struct {
	Type    string `json:"type"` // keyboard, mouse or text
	Value   string `json:"value"`
	DelayMS int64  `json:"delay_ms"`
}

// DescribeSequence parses a combo and returns its steps for agents and tooling.
func DescribeSequence(input string) ([]StepDescription, error) {
	steps, err := ParseActionSequence(input)
	if err != nil {
		return nil, err
	}
	described := make([]StepDescription, 0, len(steps))
	for _, step := range steps {
		desc := StepDescription{DelayMS: step.Delay.Milliseconds()}
		switch {
		case step.Text != "":
			desc.Type = "text"
			desc.Value = step.Text
		case step.Mouse != nil:
			desc.Type = "mouse"
			desc.Value = step.Mouse.Name
		default:
			desc.Type = "keyboard"
			desc.Value = NormalizeComboForAndroid(step.Combo)
		}
		described = append(described, desc)
	}
	return described, nil
}
