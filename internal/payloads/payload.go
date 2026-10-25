package payloads

import (
	"errors"
	"strings"
	"time"
)

// Payload represents a named payload or secret that can be injected into
// combos (e.g. {USERNAME}, {PASSWORD}, {WIN_SLEEP}) or sent directly via HID.
// Supports "type" (typed text) and "raw" (raw HID hex report) payloads,
// and secure vs non-secure storage.
type Payload struct {
	Name      string    `json:"name"`
	Type      string    `json:"type"` // "type" (keystroke typing) or "raw" (raw HID report)
	Value     string    `json:"value,omitempty"`
	Secure    bool      `json:"secure"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// IsRaw returns true if the payload is a raw HID report payload.
func (p Payload) IsRaw() bool {
	return strings.EqualFold(p.Type, "raw")
}

// EffectiveType returns "raw" or "type", defaulting empty type to "type".
func (p Payload) EffectiveType() string {
	if strings.EqualFold(p.Type, "raw") {
		return "raw"
	}
	return "type"
}

// ValidateName checks that a payload identifier is valid.
// Allowed characters: alphanumeric, underscores, hyphens. Max 40 runes.
func ValidateName(name string) (string, error) {
	clean := strings.TrimSpace(name)
	clean = strings.TrimPrefix(clean, "{")
	clean = strings.TrimSuffix(clean, "}")
	clean = strings.TrimSpace(clean)
	if clean == "" {
		return "", errors.New("payload name is required")
	}
	if len([]rune(clean)) > 40 {
		return "", errors.New("payload name must be 40 characters or fewer")
	}
	for _, r := range clean {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-') {
			return "", errors.New("payload name can only contain letters, numbers, underscores and hyphens")
		}
	}
	return clean, nil
}

// MaskedValue returns a safe display representation for lists and UI cards.
// Non-secure payloads will never be hidden and always return their plain value.
func (p Payload) MaskedValue() string {
	if p.Secure {
		if p.Value != "" {
			return "••••••••"
		}
		return "(empty)"
	}
	if p.Value != "" {
		return p.Value
	}
	return "(empty)"
}
