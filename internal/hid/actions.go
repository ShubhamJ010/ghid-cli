package hid

import (
	"errors"
	"fmt"
	"regexp"
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


// Control and lifecycle directives recognized by the GhostHID sequence runner.
const (
	DirectiveExit       = "!EXIT"
	DirectiveConnect    = "!CONNECT"
	DirectiveDisconnect = "!DISCONNECT"
	DirectiveReconnect  = "!RECONNECT"
	DirectiveStart      = "!START"
	DirectiveStop       = "!STOP"
	DirectiveRestart    = "!RESTART"
	DirectiveUnpair     = "!UNPAIR"
	DirectiveComboSend  = "!COMBO:SEND:"
)

// ErrExitRequested is returned when an !EXIT directive executes successfully.
var ErrExitRequested = errors.New("exit requested")

// Directives lists every control directive accepted by the combo grammar.
func Directives() []string {
	return []string{
		"!EXIT",
		"!CONNECT",
		"!DISCONNECT",
		"!RECONNECT",
		"!START",
		"!STOP",
		"!RESTART",
		"!UNPAIR",
		"!COMBO:SEND:<name>",
	}
}

// parseDirectiveStep parses event/lifecycle directives prefixed with "!", e.g.:
//   - "!EXIT"
//   - "!DISCONNECT"
//   - "!CONNECT" (or "!CONNECT:38:FC:98:E8:E3:87")
//   - "!RECONNECT"
//   - "!START"
//   - "!STOP"
//   - "!RESTART"
//   - "!UNPAIR"
//   - "!COMBO:SEND:lock" (or "!COMBO:lock")
func parseDirectiveStep(segment string) (string, bool, error) {
	trimmed := strings.TrimSpace(segment)
	if !strings.HasPrefix(trimmed, "!") {
		return "", false, nil
	}
	parts := strings.Split(trimmed, ":")
	verb := strings.ToUpper(strings.TrimSpace(parts[0]))
	switch verb {
	case "!EXIT":
		return DirectiveExit, true, nil
	case "!DISCONNECT":
		return DirectiveDisconnect, true, nil
	case "!RECONNECT":
		return DirectiveReconnect, true, nil
	case "!START":
		return DirectiveStart, true, nil
	case "!STOP":
		return DirectiveStop, true, nil
	case "!RESTART":
		return DirectiveRestart, true, nil
	case "!UNPAIR":
		return DirectiveUnpair, true, nil
	case "!CONNECT":
		if len(parts) > 1 {
			target := strings.TrimSpace(strings.Join(parts[1:], ":"))
			if target != "" {
				return DirectiveConnect + ":" + target, true, nil
			}
		}
		return DirectiveConnect, true, nil
	case "!COMBO":
		sub := parts[1:]
		if len(sub) == 0 {
			return "", true, errors.New("directive !COMBO requires a target name, e.g. !COMBO:SEND:lock")
		}
		if strings.EqualFold(strings.TrimSpace(sub[0]), "SEND") {
			sub = sub[1:]
		}
		name := strings.TrimSpace(strings.Join(sub, ":"))
		if name == "" {
			return "", true, errors.New("directive !COMBO requires a target name, e.g. !COMBO:SEND:lock")
		}
		return DirectiveComboSend + name, true, nil
	default:
		return "", true, fmt.Errorf("unknown directive %q (supported: %s)", trimmed, strings.Join(Directives(), ", "))
	}
}

// ParseActionSequence parses a combo in which every step is either a Windows
// keyboard chord, a mouse gesture, a literal text type step, or a variable ($VAR).
// Steps are joined with "->" and may carry a delay, e.g. "win+r ->{0.9} tab ->{0.2} $USERNAME".
func ParseActionSequence(input string) ([]ComboStep, error) {
	return parseSequence(input, parseActionSegment)
}

func parseActionSegment(segment string) (ComboStep, error) {
	if dir, ok, err := parseDirectiveStep(segment); ok {
		if err != nil {
			return ComboStep{}, err
		}
		return ComboStep{Combo: dir, Directive: dir}, nil
	}

	if varName, ok := parseVariableStep(segment); ok {
		return ComboStep{Combo: segment, Variable: varName}, nil
	}

	if text, ok, err := parseTextStep(segment); ok {
		if err != nil {
			return ComboStep{}, err
		}
		if varName, ok := parseVariableStep(text); ok {
			return ComboStep{Combo: segment, Variable: varName}, nil
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

// parseVariableStep recognises variable tokens like $USERNAME, ${PASSWORD}, or $api_token.
func parseVariableStep(segment string) (string, bool) {
	trimmed := strings.TrimSpace(segment)
	if !strings.HasPrefix(trimmed, "$") {
		return "", false
	}
	varName := strings.TrimPrefix(trimmed, "$")
	if strings.HasPrefix(varName, "{") && strings.HasSuffix(varName, "}") {
		varName = varName[1 : len(varName)-1]
	}
	varName = strings.TrimSpace(varName)
	if varName == "" {
		return "", false
	}
	for _, r := range varName {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-') {
			return "", false
		}
	}
	return varName, true
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
	Type    string `json:"type"` // keyboard, mouse, text or variable
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
		case step.Directive != "":
			desc.Type = "directive"
			desc.Value = step.Directive
		case step.Variable != "":
			desc.Type = "variable"
			desc.Value = "$" + step.Variable
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

var varRegex = regexp.MustCompile(`\$([a-zA-Z0-9_-]+)|\$\{([a-zA-Z0-9_-]+)\}`)

// ExtractVariables scans a combo sequence for all referenced snippet variables.
func ExtractVariables(input string) []string {
	steps, err := ParseActionSequence(input)
	if err != nil {
		return extractVariablesFromRaw(input)
	}

	seen := make(map[string]bool)
	var result []string

	add := func(v string) {
		clean := strings.TrimSpace(v)
		if clean == "" {
			return
		}
		key := strings.ToLower(clean)
		if !seen[key] {
			seen[key] = true
			result = append(result, clean)
		}
	}

	for _, step := range steps {
		if step.Variable != "" {
			add(step.Variable)
		}
		if step.Text != "" {
			for _, match := range varRegex.FindAllStringSubmatch(step.Text, -1) {
				if len(match) > 1 && match[1] != "" {
					add(match[1])
				} else if len(match) > 2 && match[2] != "" {
					add(match[2])
				}
			}
		}
	}
	return result
}

func extractVariablesFromRaw(input string) []string {
	seen := make(map[string]bool)
	var result []string
	for _, match := range varRegex.FindAllStringSubmatch(input, -1) {
		name := ""
		if len(match) > 1 && match[1] != "" {
			name = match[1]
		} else if len(match) > 2 && match[2] != "" {
			name = match[2]
		}
		if name != "" {
			key := strings.ToLower(name)
			if !seen[key] {
				seen[key] = true
				result = append(result, name)
			}
		}
	}
	return result
}

// VariableResolver resolves a variable name to its string value.
type VariableResolver func(name string) (string, error)

// NormalizeVariableName strips leading "$" and enclosing "{...}".
func NormalizeVariableName(token string) string {
	clean := strings.TrimSpace(token)
	clean = strings.TrimPrefix(clean, "$")
	if strings.HasPrefix(clean, "{") && strings.HasSuffix(clean, "}") {
		clean = clean[1 : len(clean)-1]
	}
	return clean
}

// ExpandTextVariables replaces $VAR and ${VAR} placeholders using the provided resolver.
func ExpandTextVariables(text string, resolver VariableResolver) string {
	if resolver == nil || !strings.Contains(text, "$") {
		return text
	}
	return varRegex.ReplaceAllStringFunc(text, func(match string) string {
		name := NormalizeVariableName(match)
		if val, err := resolver(name); err == nil && val != "" {
			return val
		}
		return match
	})
}
