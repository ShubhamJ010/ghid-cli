package hid

import "fmt"

// TokenCatalog is the machine-readable inventory of everything the sequence
// grammar accepts. It is the single source of truth for `combo keys`,
// `combo schema`, and docs, so agents never have to probe the validator.
type TokenCatalog struct {
	Keys              []string          `json:"keys"`
	Modifiers         []string          `json:"modifiers"`
	Consumer          []string          `json:"consumer"`
	MacAliases        []string          `json:"mac_aliases"`
	MouseActions      []string          `json:"mouse_actions"`
	FKeyRange         string            `json:"fkey_range"`
	Aliases           map[string]string `json:"aliases"`
	PayloadConvention string            `json:"payload_convention"`
	Notes             []string          `json:"notes"`
}

// Catalog returns the complete token inventory for the current parser.
func Catalog() TokenCatalog {
	keys := []string{
		"enter", "esc", "tab", "space", "backspace", "del", "insert",
		"up", "down", "left", "right", "home", "end", "pageup", "pagedown",
		"capslock", "numlock", "scrolllock", "printscreen", "pause", "menu",
		"minus", "equal", "leftbracket", "rightbracket", "backslash",
		"semicolon", "quote", "grave", "comma", "period", "slash",
	}
	for c := 'a'; c <= 'z'; c++ {
		keys = append(keys, string(c))
	}
	for c := '0'; c <= '9'; c++ {
		keys = append(keys, string(c))
	}
	for n := 1; n <= 24; n++ {
		keys = append(keys, fmt.Sprintf("f%d", n))
	}

	return TokenCatalog{
		Keys:         keys,
		Modifiers:    []string{"mod", "win", "ctrl", "alt", "shift"},
		Consumer:     []string{"volumeup", "volumedown", "mute", "playpause"},
		MacAliases:   []string{"cmd", "command", "super", "meta", "option", "opt"},
		MouseActions: MouseActions(),
		FKeyRange:    "f1..f24",
		Aliases: map[string]string{
			"return":       "enter",
			"escape":       "esc",
			"delete":       "del",
			"pgup":         "pageup",
			"pgdn":         "pagedown",
			"caps":         "capslock",
			"scroll":       "scrolllock",
			"prtsc":        "printscreen",
			"print":        "printscreen",
			"break":        "pause",
			"apps":         "menu",
			"context":      "menu",
			"equals":       "equal",
			"lbracket":     "leftbracket",
			"rbracket":     "rightbracket",
			"apostrophe":   "quote",
			"backtick":     "grave",
			"tilde":        "grave",
			"dot":          "period",
			"forwardslash": "slash",
			"cmd":          "win",
			"command":      "win",
			"gui":          "win",
			"meta":         "win",
			"super":        "win",
			"control":      "ctrl",
			"opt":          "alt",
			"option":       "alt",
			"volume_up":    "volumeup",
			"volup":        "volumeup",
			"volume_down":  "volumedown",
			"voldown":      "volumedown",
		},
		PayloadConvention: "Any braced identifier that is not a key, modifier, mouse action, delay, or raw hex report is a payload reference (e.g. {PASSWORD}). Payload names are case-insensitive; uppercase is convention, not required.",
		Notes: []string{
			"mod resolves to Command on macOS and Ctrl on Windows/Linux.",
			"A bare modifier such as {shift} is valid and sends a modifier-only HID report.",
			"Consumer keys carry their own HID report and cannot be combined with modifiers.",
			"playpause is the media-key token; play_pause is a payload name (the default PLAY_PAUSE raw report).",
			"Typed text outside braces must be printable ASCII.",
		},
	}
}
