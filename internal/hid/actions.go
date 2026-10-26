package hid

import (
	"encoding/hex"
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

// Canonical click buttons supported by the HID gateway.
const (
	ClickButtonLeft   = "left"
	ClickButtonRight  = "right"
	ClickButtonMiddle = "middle"
)

// NoCoordinatesExplanation explains the HID hardware constraint for AI agents and scripts.
const NoCoordinatesExplanation = "No pointer coordinates or cursor movement (move x, y / relative dx, dy) are supported. GhostHID operates at the USB HID hardware peripheral layer without screen geometry or cursor coordinate awareness. 'ghid click' only clicks wherever the cursor currently happens to be resting on screen."

// NormalizeClickButton maps button aliases to canonical button name ("left", "right", "middle").
// If the token indicates double-click (e.g. "double", "double-click"), it returns ("left", true, nil).
func NormalizeClickButton(token string) (button string, double bool, err error) {
	norm := strings.ToLower(strings.TrimSpace(token))
	if norm == "" {
		return ClickButtonLeft, false, nil
	}
	switch norm {
	case "left", "l", "click-left", "left-click", "leftclick", "lclick", "primary", "button1", "btn1":
		return ClickButtonLeft, false, nil
	case "right", "r", "click-right", "right-click", "rightclick", "rclick", "secondary", "button2", "btn2", "context":
		return ClickButtonRight, false, nil
	case "middle", "m", "click-middle", "middle-click", "middleclick", "mclick", "button3", "btn3", "wheel":
		return ClickButtonMiddle, false, nil
	case "double", "double-click", "doubleclick", "dblclick", "2x":
		return ClickButtonLeft, true, nil
	default:
		return "", false, fmt.Errorf("unsupported click button %q (accepted: left, right, middle, double)", token)
	}
}

// MouseReportHex returns the 8-hex-character (4-byte) raw HID mouse report for a button press.
func MouseReportHex(button string) string {
	switch strings.ToLower(strings.TrimSpace(button)) {
	case ClickButtonRight:
		return "02000000"
	case ClickButtonMiddle:
		return "04000000"
	default:
		return "01000000"
	}
}

var coordPatterns = []*regexp.Regexp{
	regexp.MustCompile(`^(?i)[xy]=\d+$`),
	regexp.MustCompile(`^(?i)--?[xy]$`),
	regexp.MustCompile(`^(?i)--?(?:x|y|dx|dy|coord|coordinates|pos|position)$`),
	regexp.MustCompile(`^(?i)--?(?:x|y|dx|dy|coord|coordinates|pos|position)=\d+$`),
	regexp.MustCompile(`^\d+,\d+$`),
	regexp.MustCompile(`^\d+x\d+$`),
}

// IsCoordinateOrMoveAttempt inspects arguments to detect if an AI or user tried to supply
// coordinates or cursor movement commands (e.g. "move", "100", "200", "-x 50", etc.).
func IsCoordinateOrMoveAttempt(args []string) (bool, string) {
	for i, arg := range args {
		trimmed := strings.TrimSpace(arg)
		lower := strings.ToLower(trimmed)

		// Check for words like "move", "moveto", "drag", "to", "at"
		if lower == "move" || lower == "moveto" || lower == "drag" || lower == "to" || lower == "at" {
			return true, fmt.Sprintf("cursor movement command %q is not supported", arg)
		}

		// Check regex patterns
		for _, re := range coordPatterns {
			if re.MatchString(trimmed) {
				return true, fmt.Sprintf("coordinate parameter %q is not supported", arg)
			}
		}

		// Check numeric coordinates: bare integer
		if _, err := strconv.Atoi(trimmed); err == nil {
			return true, fmt.Sprintf("coordinate value %q is not supported", arg)
		}

		// Check -x 100, -y 200, etc.
		if (lower == "-x" || lower == "-y" || lower == "--x" || lower == "--y" || lower == "-dx" || lower == "-dy") && i+1 < len(args) {
			return true, fmt.Sprintf("coordinate flag %s %s is not supported", arg, args[i+1])
		}
	}
	return false, ""
}

// ConsumerKeyReports maps standard consumer/media keys to raw 2-byte reports.
// Aliases intentionally match the Android KeyTokens table so a sequence that
// validates on the CLI resolves to the same report on the phone.
var ConsumerKeyReports = map[string]string{
	"volumeup":    "e900",
	"volume_up":   "e900",
	"volup":       "e900",
	"volumedown":  "ea00",
	"volume_down": "ea00",
	"voldown":     "ea00",
	"mute":        "e200",
	"playpause":   "cd00",
}

// consumerTokenFor returns the primary grammar token for a consumer report.
func consumerTokenFor(canonical string) string {
	switch canonical {
	case "VOLUME_UP":
		return "volumeup"
	case "VOLUME_DOWN":
		return "volumedown"
	case "MUTE":
		return "mute"
	case "PLAY_PAUSE":
		return "playpause"
	default:
		return strings.ToLower(canonical)
	}
}

// CanonicalConsumerName maps a consumer alias to the stable display name used
// in JSON output and the Android preview.
func CanonicalConsumerName(token string) string {
	switch strings.ToLower(strings.TrimSpace(token)) {
	case "volumeup", "volume_up", "volup":
		return "VOLUME_UP"
	case "volumedown", "volume_down", "voldown":
		return "VOLUME_DOWN"
	case "mute":
		return "MUTE"
	case "playpause":
		return "PLAY_PAUSE"
	default:
		return strings.ToUpper(strings.TrimSpace(token))
	}
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
		"win", "ctrl", "alt", "shift", "mod", "cmd", "command", "opt", "option", "super", "gui", "meta", "control":
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

		// Check if inner has a duration suffix separated by ':' (e.g. WIN_SLEEP:3s, Backspace:800ms)
		var holdDuration time.Duration
		tokenBody := inner
		if colonIdx := strings.LastIndex(inner, ":"); colonIdx != -1 {
			candidateToken := strings.TrimSpace(inner[:colonIdx])
			candidateDur := strings.TrimSpace(inner[colonIdx+1:])
			if m := delayRegex.FindStringSubmatch(candidateDur); len(m) == 3 && candidateToken != "" {
				val, err := strconv.ParseFloat(m[1], 64)
				if err == nil && val > 0 {
					if strings.ToLower(m[2]) == "ms" {
						holdDuration = time.Duration(val * float64(time.Millisecond))
					} else {
						holdDuration = time.Duration(val * float64(time.Second))
					}
					tokenBody = candidateToken
				}
			}
		}

		// 2. Pure Delay: {500ms}, {1s}
		if holdDuration == 0 {
			if m := delayRegex.FindStringSubmatch(tokenBody); len(m) == 3 {
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
		}

		// 3. Contains '+' -> Keyboard chord. Consumer keys cannot carry keyboard
		// modifiers (they are a different HID report), so reject the mix loudly
		// instead of silently dropping the consumer usage.
		if strings.Contains(tokenBody, "+") {
			for _, part := range strings.Split(tokenBody, "+") {
				if canonical, isConsumer := ConsumerToken(part); isConsumer {
					return nil, fmt.Errorf("consumer key %q cannot be combined with modifiers (use {%s})", strings.TrimSpace(part), consumerTokenFor(canonical))
				}
			}
			mod, code, err := ParseCombo(tokenBody)
			if err != nil {
				return nil, fmt.Errorf("invalid chord %q: %w", inner, err)
			}
			flushText()
			steps = append(steps, ComboStep{Combo: tokenBody, Modifier: mod, KeyCode: code, Hold: holdDuration})
			lastIdx = matchEnd
			continue
		}

		// 4. Standard W3C / HID key name or consumer key or mouse gesture
		lowerBody := strings.ToLower(tokenBody)
		if rawReport, isConsumer := ConsumerKeyReports[lowerBody]; isConsumer {
			flushText()
			steps = append(steps, ComboStep{Combo: tokenBody, Raw: rawReport, Hold: holdDuration})
			lastIdx = matchEnd
			continue
		}

		if mouseAction, isMouse, mErr := ParseMouseAction(tokenBody); isMouse {
			if mErr != nil {
				return nil, mErr
			}
			flushText()
			steps = append(steps, ComboStep{Combo: mouseAction.Name, Mouse: &mouseAction, Hold: holdDuration})
			lastIdx = matchEnd
			continue
		}

		if isStandardKey(tokenBody) {
			mod, code, err := ParseCombo(tokenBody)
			if err == nil {
				flushText()
				steps = append(steps, ComboStep{Combo: tokenBody, Modifier: mod, KeyCode: code, Hold: holdDuration})
				lastIdx = matchEnd
				continue
			}
		}

		// Explicit raw report prefix: {raw:02}, {raw:cd00}, {raw:08000f0000000000}
		if strings.HasPrefix(lowerBody, "raw:") {
			rawHex := strings.ReplaceAll(strings.TrimSpace(tokenBody[4:]), " ", "")
			if _, rawErr := ValidateRaw(rawHex); rawErr == nil {
				flushText()
				steps = append(steps, ComboStep{Combo: tokenBody, Raw: strings.ToLower(rawHex), Hold: holdDuration})
				lastIdx = matchEnd
				continue
			} else {
				return nil, fmt.Errorf("invalid raw hex in %q: %w", inner, rawErr)
			}
		}

		// Check for direct raw report hex (e.g. 02, cd00, 08000f0000000000)
		// Note: All-digit tokens of length >= 4 (e.g. 1234) are payload references, not raw reports.
		cleanHex := strings.ReplaceAll(tokenBody, " ", "")
		isAllDigits := isPureDigits(cleanHex)
		isRawCandidate := (len(cleanHex) == 2 && isAllDigits) || (!isAllDigits && (len(cleanHex) == 2 || len(cleanHex) == 4 || len(cleanHex) == 8 || len(cleanHex) == 16))
		if isRawCandidate {
			if _, hexErr := hex.DecodeString(cleanHex); hexErr == nil {
				flushText()
				steps = append(steps, ComboStep{Combo: tokenBody, Raw: strings.ToLower(cleanHex), Hold: holdDuration})
				lastIdx = matchEnd
				continue
			}
		}

		// 5. Alphanumeric / Identifier -> Named Payload reference
		if identRegex.MatchString(tokenBody) {
			flushText()
			steps = append(steps, ComboStep{Combo: tokenBody, Payload: tokenBody, Hold: holdDuration})
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
	case "win", "gui", "cmd", "command", "meta", "super", "ctrl", "control", "alt", "shift", "mod", "opt", "option":
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

// StepDescription is a machine-readable view of one parsed combo step. The
// HID fields let agents verify exactly what would go over the wire.
type StepDescription struct {
	Type      string `json:"type"` // keyboard, mouse, text, payload, delay, or raw
	Value     string `json:"value"`
	DelayMS   int64  `json:"delay_ms"`
	HoldMS    int64  `json:"hold_ms,omitempty"`
	Modifier  byte   `json:"modifier,omitempty"`
	Keycode   byte   `json:"keycode,omitempty"`
	ReportHex string `json:"report_hex,omitempty"`
	RawHex    string `json:"raw_hex,omitempty"`
	ReportID  int    `json:"report_id,omitempty"`
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
		if step.Hold > 0 {
			desc.HoldMS = step.Hold.Milliseconds()
		}
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
			desc.RawHex = step.Raw
			if rawBytes, err := hex.DecodeString(step.Raw); err == nil {
				desc.ReportID, _ = ReportIDForLength(len(rawBytes))
			}
		case step.Mouse != nil:
			desc.Type = "mouse"
			desc.Value = step.Mouse.Name
			desc.ReportID = ReportIDMouse
		case step.Delay > 0 && step.Combo != "" && (step.Modifier == 0 && step.KeyCode == 0):
			desc.Type = "delay"
			desc.Value = step.Combo
		default:
			desc.Type = "keyboard"
			desc.Value = NormalizeComboForAndroid(step.Combo)
			desc.Modifier = step.Modifier
			desc.Keycode = step.KeyCode
			desc.ReportHex = NewKeyReport(step.Modifier, step.KeyCode).HexString()
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

func isPureDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// PayloadResolver resolves a payload name to its string value, whether it is a raw HID report, and error.
type PayloadResolver func(name string) (value string, isRaw bool, err error)
