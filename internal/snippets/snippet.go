package snippets

import (
	"errors"
	"github.com/ghost-hid/ghid/internal/hid"
	"time"
)

// Snippet represents a named text snippet or variable that can be injected into
// combos (e.g. $USERNAME, $PASSWORD) or typed via HID.
type Snippet struct {
	Name      string    `json:"name"`
	Value     string    `json:"value,omitempty"`
	Secure    bool      `json:"secure"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
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
