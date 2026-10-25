package hid

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// MouseAction describes a pointer gesture that is transmitted as its own HID
// mouse report. Kept separate from keyboard chords because target systems interpret
// mouse buttons and wheel ticks on a different HID report ID.
type MouseAction struct {
	Name    string // canonical token, e.g. "click-left"
	Buttons byte   // HID button bitmap (bit 0 = left, bit 1 = right, bit 2 = middle)
	Wheel   int8   // +1 scroll up, -1 scroll down
	Double  bool   // repeat the click twice
}

// Canonical mouse gestures understood by the HID gateway.
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

// ConsumerKeyReports maps standard consumer/media keys to raw 2-byte reports.
var ConsumerKeyReports = map[string]string{
	"volumeup":   "e900",
	"volumedown": "ea00",
	"mute":       "e200",
	"playpause":  "cd00",
	"play_pause": "cd00",
	"play-pause": "cd00",
}

var (
	bracketRegex = regexp.MustCompile(`\{\{|(\{([^}]+)\})`)
	delayRegex   = regexp.MustCompile(`^(?i)([0-9]+(?:\.[0-9]+)?)(ms|s)$`)
	identRegex   = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
)

func checkBracketsSyntax(input string) error {
	inBrace := false
	runes := []rune(input)
	n := len(runes)
	for i := 0; i < n; i++ {
		if runes[i] == '{' {
			if !inBrace && i+1 < n && runes[i+1] == '{' {
				i++ // skip escaped {{
				continue
			}
			if inBrace {
				return fmt.Errorf("unexpected nested '{' at position %d", i)
			}
			inBrace = true
		} else if runes[i] == '}' {
			if !inBrace {
				continue
			}
			inBrace = false
		}
	}
	if inBrace {
		return errors.New("unclosed '{' in sequence")
	}
	return nil
}

func isStandardKey(token string) bool {
	lower := strings.ToLower(strings.TrimSpace(token))
	switch lower {
	case "tab", "enter", "return", "esc", "escape", "space", "backspace",
		"del", "delete", "insert", "home", "end", "pageup", "pgup", "pagedown", "pgdn",
		"up", "down", "left", "right",
		"capslock", "caps", "numlock", "scrolllock", "scroll",
		"printscreen", "prtsc", "print", "pause", "break", "menu", "apps", "context",
		"minus", "equal", "equals", "leftbracket", "lbracket", "rightbracket", "rbracket",
		"backslash", "semicolon", "quote", "apostrophe", "grave", "backtick", "tilde",
		"comma", "period", "dot", "slash", "forwardslash",
		"win", "ctrl", "alt", "shift", "mod", "cmd", "opt", "option", "super", "gui", "meta", "control":
		return true
	}
	if strings.HasPrefix(lower, "f") && len(lower) >= 2 && len(lower) <= 3 {
		n := 0
		for _, r := range lower[1:] {
			if r < '0' || r > '9' {
				return false
			}
			n = n*10 + int(r-'0')
		}
		return n >= 1 && n <= 24
	}
	return false
}

// ParseSequence parses a canonical {...} bracket sequence into executable ComboSteps.
// Grammar:
// - Plain text outside {} is emitted as virtual typing steps.
// - Inside {}:
//  1. {{ -> Literal "{" (escaped bracket in typed text).
//  2. Number + ms or s ({500ms}, {1s}) -> Delay step.
//  3. Contains "+" ({mod+s}, {ctrl+alt+del}) -> Keyboard chord step.
//  4. Standard W3C / HID key name or consumer key or mouse action -> Single key / raw / mouse step.
//  5. Alphanumeric / Identifier ({PASSWORD}, {WIN_SLEEP}) -> Named Payload reference step.
func ParseSequence(input string) ([]ComboStep, error) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return nil, errors.New("empty combo sequence")
	}

	if err := checkBracketsSyntax(trimmed); err != nil {
		return nil, err
	}

	matches := bracketRegex.FindAllStringSubmatchIndex(trimmed, -1)
	if len(matches) == 0 {
		// Entire input is plain text typing
		return []ComboStep{{Text: trimmed}}, nil
	}

	var steps []ComboStep
	var currentText strings.Builder
	lastIdx := 0

	flushText := func() {
		if currentText.Len() > 0 {
			steps = append(steps, ComboStep{Text: currentText.String()})
			currentText.Reset()
		}
	}

	for _, loc := range matches {
		matchStart := loc[0]
		matchEnd := loc[1]

		// Text before this token
		if matchStart > lastIdx {
			currentText.WriteString(trimmed[lastIdx:matchStart])
		}

		matchedStr := trimmed[matchStart:matchEnd]
		if matchedStr == "{{" {
			currentText.WriteByte('{')
			lastIdx = matchEnd
			continue
		}

		// Group 2 is inside {...}
		inner := strings.TrimSpace(trimmed[loc[4]:loc[5]])
		if inner == "" {
			return nil, errors.New("empty bracket '{}' in sequence")
		}

		// 2. Delay: {500ms}, {1s}
		if m := delayRegex.FindStringSubmatch(inner); len(m) == 3 {
			val, err := strconv.ParseFloat(m[1], 64)
			if err != nil {
				return nil, fmt.Errorf("invalid delay %q: %w", inner, err)
			}
			var d time.Duration
			if strings.ToLower(m[2]) == "ms" {
				d = time.Duration(val * float64(time.Millisecond))
			} else {
				d = time.Duration(val * float64(time.Second))
			}
			flushText()
			steps = append(steps, ComboStep{Combo: inner, Delay: d})
			lastIdx = matchEnd
			continue
		}

		// 3. Contains '+' -> Keyboard chord
		if strings.Contains(inner, "+") {
			mod, code, err := ParseCombo(inner)
			if err != nil {
				return nil, fmt.Errorf("invalid chord %q: %w", inner, err)
			}
			flushText()
			steps = append(steps, ComboStep{Combo: inner, Modifier: mod, KeyCode: code})
			lastIdx = matchEnd
			continue
		}

		// 4. Standard W3C / HID key name or consumer key or mouse gesture
		lowerInner := strings.ToLower(inner)
		if rawReport, isConsumer := ConsumerKeyReports[lowerInner]; isConsumer {
			flushText()
			steps = append(steps, ComboStep{Combo: inner, Raw: rawReport})
			lastIdx = matchEnd
			continue
		}

		if mouseAction, isMouse, mErr := ParseMouseAction(inner); isMouse {
			if mErr != nil {
				return nil, mErr
			}
			flushText()
			steps = append(steps, ComboStep{Combo: mouseAction.Name, Mouse: &mouseAction})
			lastIdx = matchEnd
			continue
		}

		if isStandardKey(inner) {
			mod, code, err := ParseCombo(inner)
			if err == nil {
				flushText()
				steps = append(steps, ComboStep{Combo: inner, Modifier: mod, KeyCode: code})
				lastIdx = matchEnd
				continue
			}
		}

		// 5. Alphanumeric / Identifier -> Named Payload reference
		if identRegex.MatchString(inner) {
			flushText()
			steps = append(steps, ComboStep{Combo: inner, Payload: inner})
			lastIdx = matchEnd
			continue
		}

		return nil, fmt.Errorf("unknown token %q in sequence", inner)
	}

	// Remaining text after last match
	if lastIdx < len(trimmed) {
		currentText.WriteString(trimmed[lastIdx:])
	}
	flushText()

	if len(steps) == 0 {
		return nil, errors.New("empty combo sequence")
	}

	return steps, nil
}

// IsWindowsKeyToken reports whether a single token names a key that exists on
// a real Windows keyboard.
func IsWindowsKeyToken(token string) bool {
	p := strings.ToLower(strings.TrimSpace(token))
	if p == "" {
		return false
	}

	switch p {
	case "win", "gui", "ctrl", "control", "alt", "shift", "mod":
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
		return n >= 1 && n <= 24
	}

	return false
}

// WindowsKeyboardTokens lists every key token the combo grammar accepts.
func WindowsKeyboardTokens() []string {
	tokens := []string{
		"win", "ctrl", "alt", "shift", "mod",
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
	for i := 1; i <= 24; i++ {
		tokens = append(tokens, fmt.Sprintf("f%d", i))
	}
	return tokens
}

// StepDescription is a machine-readable view of one parsed combo step.
type StepDescription struct {
	Type    string `json:"type"` // keyboard, mouse, text, payload, delay, or raw
	Value   string `json:"value"`
	DelayMS int64  `json:"delay_ms"`
}

// DescribeSequence parses a combo and returns its steps for agents and tooling.
func DescribeSequence(input string) ([]StepDescription, error) {
	steps, err := ParseSequence(input)
	if err != nil {
		return nil, err
	}
	described := make([]StepDescription, 0, len(steps))
	for _, step := range steps {
		desc := StepDescription{DelayMS: step.Delay.Milliseconds()}
		switch {
		case step.Payload != "":
			desc.Type = "payload"
			desc.Value = step.Payload
		case step.Text != "":
			desc.Type = "text"
			desc.Value = step.Text
		case step.Raw != "":
			desc.Type = "raw"
			desc.Value = step.Raw
		case step.Mouse != nil:
			desc.Type = "mouse"
			desc.Value = step.Mouse.Name
		case step.Delay > 0 && step.Combo != "" && (step.Modifier == 0 && step.KeyCode == 0):
			desc.Type = "delay"
			desc.Value = step.Combo
		default:
			desc.Type = "keyboard"
			desc.Value = NormalizeComboForAndroid(step.Combo)
		}
		described = append(described, desc)
	}
	return described, nil
}

// ExtractPayloads scans a combo sequence for all referenced payload names ({NAME}).
func ExtractPayloads(input string) []string {
	steps, err := ParseSequence(input)
	if err != nil {
		return nil
	}

	seen := make(map[string]bool)
	var result []string
	for _, step := range steps {
		if step.Payload != "" {
			clean := strings.TrimSpace(step.Payload)
			key := strings.ToLower(clean)
			if !seen[key] {
				seen[key] = true
				result = append(result, clean)
			}
		}
	}
	return result
}

// PayloadResolver resolves a payload name to its string or hex value.
type PayloadResolver func(name string) (string, error)
