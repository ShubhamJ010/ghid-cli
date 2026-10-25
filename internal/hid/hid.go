package hid

import (
	"encoding/hex"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"time"
)

// Standard USB HID Keyboard Modifier Bitmasks
const (
	ModLeftCtrl   byte = 0x01
	ModLeftShift  byte = 0x02
	ModLeftAlt    byte = 0x04
	ModLeftGUI    byte = 0x08 // Windows key / Command key / Super key
	ModRightCtrl  byte = 0x10
	ModRightShift byte = 0x20
	ModRightAlt   byte = 0x40
	ModRightGUI   byte = 0x80
)

// Standard USB HID Usage Table Keycodes (Page 0x07)
const (
	KeyNone      byte = 0x00
	KeyA         byte = 0x04
	KeyS         byte = 0x16
	KeyZ         byte = 0x1D
	Key1         byte = 0x1E
	Key0         byte = 0x27
	KeyEnter     byte = 0x28
	KeyEscape    byte = 0x29
	KeyDelete    byte = 0x2A // Backspace
	KeyTab       byte = 0x2B
	KeySpace     byte = 0x2C
	KeyMinus     byte = 0x2D
	KeyEqual     byte = 0x2E
	KeyLBracket  byte = 0x2F
	KeyRBracket  byte = 0x30
	KeyBackslash byte = 0x31
	KeySemicolon byte = 0x33
	KeyQuote     byte = 0x34
	KeyGrave     byte = 0x35
	KeyComma     byte = 0x36
	KeyPeriod    byte = 0x37
	KeySlash     byte = 0x38
	KeyCapsLock  byte = 0x39
	KeyF1        byte = 0x3A
	KeyF2        byte = 0x3B
	KeyF3        byte = 0x3C
	KeyF4        byte = 0x3D
	KeyF5        byte = 0x3E
	KeyF6        byte = 0x3F
	KeyF7        byte = 0x40
	KeyF8        byte = 0x41
	KeyF9        byte = 0x42
	KeyF10       byte = 0x43
	KeyF11       byte = 0x44
	KeyF12       byte = 0x45
	KeyPrintScr  byte = 0x46
	KeyScrollLk  byte = 0x47
	KeyPause     byte = 0x48
	KeyInsert    byte = 0x49
	KeyHome      byte = 0x4A
	KeyPageUp    byte = 0x4B
	KeyDelFwd    byte = 0x4C // Forward delete
	KeyEnd       byte = 0x4D
	KeyPageDown  byte = 0x4E
	KeyRight     byte = 0x4F
	KeyLeft      byte = 0x50
	KeyDown      byte = 0x51
	KeyUp        byte = 0x52
	KeyNumLock   byte = 0x53
	KeyMenu      byte = 0x65 // Windows context-menu / applications key
)

// KeyReport represents a standard 8-byte HID keyboard input report.
// Byte 0: Modifier keys bitmap
// Byte 1: Reserved (always 0x00)
// Bytes 2-7: Up to 6 simultaneous pressed keycodes
type KeyReport [8]byte

// NewKeyReport constructs an 8-byte report from modifier and primary keycode.
func NewKeyReport(modifier, keyCode byte) KeyReport {
	return KeyReport{modifier, 0, keyCode, 0, 0, 0, 0, 0}
}

// HexString returns the 16-character hexadecimal representation of the report.
func (r KeyReport) HexString() string {
	return hex.EncodeToString(r[:])
}

// Report ID constants
const (
	ReportIDKeyboard = 1
	ReportIDMouse    = 2
	ReportIDSystem   = 3
	ReportIDConsumer = 4
)

// ReportIDForLength maps payload byte length to the appropriate HID report ID.
func ReportIDForLength(byteLen int) (int, error) {
	switch byteLen {
	case 1:
		return ReportIDSystem, nil
	case 2:
		return ReportIDConsumer, nil
	case 4:
		return ReportIDMouse, nil
	case 8:
		return ReportIDKeyboard, nil
	default:
		return 0, fmt.Errorf("invalid HID report size: %d bytes (supported: 1=System, 2=Consumer, 4=Mouse, 8=Keyboard)", byteLen)
	}
}

// ValidateRaw validates that a string is a valid hex HID report (1, 2, 4, or 8 bytes).
func ValidateRaw(hexStr string) ([]byte, error) {
	trimmed := strings.TrimSpace(hexStr)
	switch len(trimmed) {
	case 2, 4, 8, 16:
	default:
		return nil, fmt.Errorf("raw report must be 2 hex chars (1-byte system), 4 (2-byte consumer), 8 (4-byte mouse), or 16 (8-byte keyboard), got %d", len(trimmed))
	}
	bytes, err := hex.DecodeString(trimmed)
	if err != nil {
		return nil, fmt.Errorf("invalid hexadecimal characters: %w", err)
	}
	return bytes, nil
}

// ParseCombo translates human-readable key combinations (e.g. "ctrl+alt+del", "super+l", "esc")
// into the corresponding HID modifier mask and primary keycode.
func ParseCombo(combo string) (modifier byte, keyCode byte, err error) {
	trimmed := strings.ToLower(strings.TrimSpace(combo))
	if trimmed == "" {
		return 0, 0, errors.New("empty key combination")
	}

	parts := strings.Split(trimmed, "+")
	for _, rawPart := range parts {
		p := strings.TrimSpace(rawPart)
		switch p {
		case "mod":
			if runtime.GOOS == "darwin" {
				modifier |= ModLeftGUI
			} else {
				modifier |= ModLeftCtrl
			}
		case "win", "gui", "cmd", "meta", "super":
			modifier |= ModLeftGUI
		case "ctrl", "control":
			modifier |= ModLeftCtrl
		case "alt", "opt", "option":
			modifier |= ModLeftAlt
		case "shift":
			modifier |= ModLeftShift
		case "del", "delete":
			keyCode = KeyDelFwd
		case "backspace":
			keyCode = KeyDelete
		case "esc", "escape":
			keyCode = KeyEscape
		case "enter", "return":
			keyCode = KeyEnter
		case "tab":
			keyCode = KeyTab
		case "space":
			keyCode = KeySpace
		case "up":
			keyCode = KeyUp
		case "down":
			keyCode = KeyDown
		case "left":
			keyCode = KeyLeft
		case "right":
			keyCode = KeyRight
		case "home":
			keyCode = KeyHome
		case "end":
			keyCode = KeyEnd
		case "pageup", "pgup":
			keyCode = KeyPageUp
		case "pagedown", "pgdn":
			keyCode = KeyPageDown
		case "insert":
			keyCode = KeyInsert
		case "capslock", "caps":
			keyCode = KeyCapsLock
		case "numlock":
			keyCode = KeyNumLock
		case "scrolllock", "scroll":
			keyCode = KeyScrollLk
		case "printscreen", "prtsc", "print":
			keyCode = KeyPrintScr
		case "pause", "break":
			keyCode = KeyPause
		case "menu", "apps", "context":
			keyCode = KeyMenu
		case "minus":
			keyCode = KeyMinus
		case "equal", "equals":
			keyCode = KeyEqual
		case "leftbracket", "lbracket":
			keyCode = KeyLBracket
		case "rightbracket", "rbracket":
			keyCode = KeyRBracket
		case "backslash":
			keyCode = KeyBackslash
		case "semicolon":
			keyCode = KeySemicolon
		case "quote", "apostrophe":
			keyCode = KeyQuote
		case "grave", "backtick", "tilde":
			keyCode = KeyGrave
		case "comma":
			keyCode = KeyComma
		case "period", "dot":
			keyCode = KeyPeriod
		case "slash", "forwardslash":
			keyCode = KeySlash
		case "f1":
			keyCode = KeyF1
		case "f2":
			keyCode = KeyF2
		case "f3":
			keyCode = KeyF3
		case "f4":
			keyCode = KeyF4
		case "f5":
			keyCode = KeyF5
		case "f6":
			keyCode = KeyF6
		case "f7":
			keyCode = KeyF7
		case "f8":
			keyCode = KeyF8
		case "f9":
			keyCode = KeyF9
		case "f10":
			keyCode = KeyF10
		case "f11":
			keyCode = KeyF11
		case "f12":
			keyCode = KeyF12
		default:
			if len(p) == 1 {
				c := p[0]
				if c >= 'a' && c <= 'z' {
					keyCode = KeyA + (c - 'a')
				} else if c >= '1' && c <= '9' {
					keyCode = Key1 + (c - '1')
				} else if c == '0' {
					keyCode = Key0
				} else {
					return 0, 0, fmt.Errorf("unsupported key: %q", p)
				}
			} else {
				return 0, 0, fmt.Errorf("unknown key token: %q", p)
			}
		}
	}

	return modifier, keyCode, nil
}

// NormalizeComboForAndroid transforms alias tokens like "super" or "cmd" to standard
// Android gateway receiver tokens like "win", and "escape" to "esc".
func NormalizeComboForAndroid(combo string) string {
	parts := strings.Split(strings.ToLower(strings.TrimSpace(combo)), "+")
	normalized := make([]string, 0, len(parts))
	for _, p := range parts {
		token := strings.TrimSpace(p)
		switch token {
		case "mod":
			normalized = append(normalized, "mod")
		case "super", "cmd", "meta", "gui":
			normalized = append(normalized, "win")
		case "control":
			normalized = append(normalized, "ctrl")
		case "option", "opt":
			normalized = append(normalized, "alt")
		case "escape":
			normalized = append(normalized, "esc")
		case "return":
			normalized = append(normalized, "enter")
		case "delete":
			normalized = append(normalized, "del")
		case "pgup":
			normalized = append(normalized, "pageup")
		case "pgdn":
			normalized = append(normalized, "pagedown")
		default:
			normalized = append(normalized, token)
		}
	}
	return strings.Join(normalized, "+")
}

// ComboStep represents a single action in a chained combo sequence.
type ComboStep struct {
	Combo    string        // Human-readable combo or token for this step, e.g. "mod+s", "Tab", "500ms"
	Modifier byte          // Resolved modifier bitmask
	KeyCode  byte          // Resolved primary keycode
	Mouse    *MouseAction  // Non-nil when this step is a mouse gesture
	Text     string        // Non-empty when this step types a literal string
	Payload  string        // Non-empty when this step references a payload name, e.g. "PASSWORD"
	Raw      string        // Non-empty hex string when this step is a raw HID report
	Delay    time.Duration // Pause duration after transmitting this step
}
