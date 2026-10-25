package snippets

import (
	"errors"
	"strings"
	"time"

	"github.com/ghost-hid/ghid/internal/hid"
)

// Snippet represents a named snippet or variable that can be injected into
// combos (e.g. $USERNAME, $PASSWORD, $SLEEP_WINDOWS) or sent directly via HID.
// Supports "type" (typed text) and "raw" (raw HID hex report) snippets,
// and secure vs non-secure storage.
type Snippet struct {
	Name      string    `json:"name"`
	Type      string    `json:"type"` // "type" (keystroke typing) or "raw" (raw HID report)
	Value     string    `json:"value,omitempty"`
	Secure    bool      `json:"secure"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// IsRaw returns true if the snippet is a raw HID report snippet.
func (s Snippet) IsRaw() bool {
	return strings.EqualFold(s.Type, "raw")
}

// EffectiveType returns "raw" or "type", defaulting empty type to "type".
func (s Snippet) EffectiveType() string {
	if strings.EqualFold(s.Type, "raw") {
		return "raw"
	}
	return "type"
}

// ValidateName checks that a snippet identifier is valid.
// Allowed characters: alphanumeric, underscores, hyphens. Max 40 runes.
func ValidateName(name string) (string, error) {
	clean := hid.NormalizeVariableName(name)
	if clean == "" {
		return "", errors.New("snippet name is required")
	}
	if len([]rune(clean)) > 40 {
		return "", errors.New("snippet name must be 40 characters or fewer")
	}
	for _, r := range clean {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-') {
			return "", errors.New("snippet name can only contain letters, numbers, underscores and hyphens")
		}
	}
	return clean, nil
}

// MaskedValue returns a safe display representation for lists and UI cards.
// Non-secure snippets will never be hidden and always return their plain value.
func (s Snippet) MaskedValue() string {
	if s.Secure {
		if s.Value != "" {
			return "••••••••"
		}
		return "(empty)"
	}
	if s.Value != "" {
		return s.Value
	}
	return "(empty)"
}
