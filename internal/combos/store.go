// Package combos persists named keyboard/mouse shortcuts for the
// interactive combo studio and the CLI.
package combos

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/ghost-hid/ghid/internal/hid"
	"github.com/ghost-hid/ghid/internal/payloads"
)

// Default combo sequences shipped with the application.
const (
	DefaultLoginSequence = "{ctrl+alt+del}{1s}{Escape}{800ms}{USERNAME}{800ms}{Tab}{800ms}{PASSWORD}{800ms}{Enter}"
	DefaultLockSequence  = "{mod+l}"
	DefaultSleepSequence = "{WIN_SLEEP}"

	DefaultMacSleepSequence     = "{mod+ctrl+q}{500ms}{Escape}"
	DefaultMacForceQuitSequence = "{mod+alt+esc}{400ms}{Enter}{300ms}{Enter}"
	DefaultMacPlayPauseSequence = "{PLAY_PAUSE}"
)

// Target operating systems a combo is written for. The tag drives the combo
// icon and the OS filters in the CLI and Android app.
const (
	OSWindows = "windows"
	OSMacOS   = "macos"
	OSLinux   = "linux"
)

// OSTokens lists every valid combo OS tag in display order.
func OSTokens() []string {
	return []string{OSWindows, OSMacOS, OSLinux}
}

// OSLabel returns the human-readable name for an OS tag.
func OSLabel(osName string) string {
	switch NormalizeOSDefault(osName) {
	case OSMacOS:
		return "macOS"
	case OSLinux:
		return "Linux"
	default:
		return "Windows"
	}
}

// NormalizeOS canonicalizes the accepted OS aliases. An unknown token is an
// error; callers persisting combos use it to reject typos.
func NormalizeOS(osName string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(osName)) {
	case "", OSWindows, "win", "windows-11", "windows11":
		return OSWindows, nil
	case OSMacOS, "mac", "osx", "darwin", "apple":
		return OSMacOS, nil
	case OSLinux, "gnu/linux", "ubuntu", "arch", "fedora":
		return OSLinux, nil
	default:
		return "", fmt.Errorf("unknown os %q (use %s)", osName, strings.Join(OSTokens(), ", "))
	}
}

// NormalizeOSDefault canonicalizes an OS tag, falling back to Windows for
// legacy combos saved before OS tagging existed.
func NormalizeOSDefault(osName string) string {
	normalized, err := NormalizeOS(osName)
	if err != nil {
		return OSWindows
	}
	return normalized
}

// MaxEmojiRunes caps a combo icon. Zero-width joiners and variation selectors
// make multi-codepoint emoji long, so the limit is generous.
const MaxEmojiRunes = 12

// ValidateEmoji trims an optional combo icon and rejects plain ASCII text so the
// stored value really is an emoji. An empty value clears the custom icon.
func ValidateEmoji(emoji string) (string, error) {
	clean := strings.TrimSpace(emoji)
	if clean == "" {
		return "", nil
	}
	if len([]rune(clean)) > MaxEmojiRunes {
		return "", fmt.Errorf("combo emoji must be %d characters or fewer", MaxEmojiRunes)
	}
	for _, r := range clean {
		if unicode.IsSpace(r) {
			return "", errors.New("combo emoji cannot contain spaces")
		}
		if r < 0x80 && (unicode.IsLetter(r) || unicode.IsDigit(r)) {
			return "", errors.New("combo emoji must be an emoji, not text")
		}
	}
	return clean, nil
}

// OSShortTag is the compact ASCII badge used in terminal listings.
func OSShortTag(osName string) string {
	switch NormalizeOSDefault(osName) {
	case OSMacOS:
		return "mac"
	case OSLinux:
		return "linux"
	default:
		return "win"
	}
}

// Combo is a named, replayable sequence of keyboard chords, mouse gestures,
// typed text, delays, and named payloads in the {...} DSL format.
type Combo struct {
	Name        string                      `json:"name"`
	Sequence    string                      `json:"sequence"`
	Payloads    map[string]payloads.Payload `json:"payloads,omitempty"`
	OS          string                      `json:"os"`
	Emoji       string                      `json:"emoji,omitempty"`
	Description string                      `json:"description,omitempty"`
	Favorite    bool                        `json:"favorite,omitempty"`
	CreatedAt   time.Time                   `json:"created_at"`
}

// UnmarshalJSON handles migration from legacy "keys" field to "sequence".
func (c *Combo) UnmarshalJSON(data []byte) error {
	type Alias Combo
	aux := &struct {
		LegacyKeys string `json:"keys"`
		*Alias
	}{
		Alias: (*Alias)(c),
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	if c.Sequence == "" && aux.LegacyKeys != "" {
		c.Sequence = aux.LegacyKeys
	}
	return nil
}

// Badge is the short icon used in terminal listings: the custom emoji when set,
// otherwise the bracketed target OS tag.
func (c Combo) Badge() string {
	if emoji := strings.TrimSpace(c.Emoji); emoji != "" {
		return emoji
	}
	return "[" + OSShortTag(c.OS) + "]"
}

// DefaultCombos defines the built-in system combos with their target OS, sequences, and bundled payloads.
var DefaultCombos = []Combo{
	{
		Name:        "Login",
		Sequence:    DefaultLoginSequence,
		OS:          OSWindows,
		Emoji:       "🔑",
		Description: "Windows unlock & credential injection",
		Favorite:    true,
	},
	{
		Name:        "Lock",
		Sequence:    DefaultLockSequence,
		OS:          OSWindows,
		Emoji:       "🔒",
		Description: "Instant lock screen ({mod+l})",
		Favorite:    true,
	},
	{
		Name:     "Sleep",
		Sequence: DefaultSleepSequence,
		Payloads: map[string]payloads.Payload{
			"WIN_SLEEP": {
				Name:  "WIN_SLEEP",
				Type:  "raw",
				Value: "02",
			},
		},
		OS:          OSWindows,
		Emoji:       "😴",
		Description: "Hardware ACPI sleep sequence",
		Favorite:    true,
	},
	{
		Name:        "MacSleep",
		Sequence:    DefaultMacSleepSequence,
		OS:          OSMacOS,
		Emoji:       "🌙",
		Description: "Instant macOS lock and display sleep",
		Favorite:    true,
	},
	{
		Name:        "ForceQuit",
		Sequence:    DefaultMacForceQuitSequence,
		OS:          OSMacOS,
		Emoji:       "⚡",
		Description: "Force quit frontmost unresponsive app",
		Favorite:    true,
	},
	{
		Name:     "PlayPause",
		Sequence: DefaultMacPlayPauseSequence,
		Payloads: map[string]payloads.Payload{
			"PLAY_PAUSE": {
				Name:  "PLAY_PAUSE",
				Type:  "raw",
				Value: "cd00",
			},
		},
		OS:          OSMacOS,
		Emoji:       "⏯️",
		Description: "Consumer media play/pause toggle",
		Favorite:    true,
	},
}

// Store reads and writes the combo library on disk.
type Store struct {
	Path         string
	PayloadStore *payloads.Store
}

// NewStore creates a store at path. An empty path uses the default location
// (~/.config/ghid/combos.json).
func NewStore(path string, ps ...*payloads.Store) *Store {
	if strings.TrimSpace(path) == "" {
		path = GetDefaultPath()
	}
	var pStore *payloads.Store
	if len(ps) > 0 {
		pStore = ps[0]
	}
	return &Store{Path: path, PayloadStore: pStore}
}

// SetPayloadStore connects a payloads store for auto-provisioning.
func (s *Store) SetPayloadStore(ps *payloads.Store) {
	s.PayloadStore = ps
}

// GetDefaultPath returns the per-user combo library location.
func GetDefaultPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".config", "ghid", "combos.json")
	}
	return filepath.Join(home, ".config", "ghid", "combos.json")
}

// AutoProvisionPayloads inspects the combo sequence and provisions any missing
// referenced payloads that are bundled in combo.Payloads.
func (s *Store) AutoProvisionPayloads(c Combo) {
	if s.PayloadStore == nil {
		return
	}
	needed := hid.ExtractPayloads(c.Sequence)
	for _, name := range needed {
		if _, ok, _ := s.PayloadStore.Get(name); !ok {
			for pName, pVal := range c.Payloads {
				if strings.EqualFold(pName, name) {
					_, _ = s.PayloadStore.Set(name, pVal.Value, pVal.Secure, pVal.EffectiveType())
					break
				}
			}
		}
	}
}

// Load returns the saved combos in display order. A missing file is an empty
// library, not an error.
func (s *Store) Load() ([]Combo, error) {
	data, err := os.ReadFile(s.Path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []Combo{}, nil
		}
		return nil, err
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return []Combo{}, nil
	}

	var list []Combo
	if err := json.Unmarshal(data, &list); err != nil {
		return nil, fmt.Errorf("parse %s: %w", s.Path, err)
	}
	for i := range list {
		list[i].OS = NormalizeOSDefault(list[i].OS)
	}
	return list, nil
}

// SaveAll persists a complete list of combos to disk.
func (s *Store) SaveAll(list []Combo) error {
	return s.save(list)
}

func (s *Store) save(list []Combo) error {
	if list == nil {
		list = []Combo{}
	}
	if err := os.MkdirAll(filepath.Dir(s.Path), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.Path, data, 0600)
}

// EnsureDefaults seeds the default built-in combos if empty,
// and ensures existing built-ins have their OS tag, sequence, and payloads populated.
func (s *Store) EnsureDefaults() ([]Combo, error) {
	list, err := s.Load()
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		now := time.Now().UTC()
		var seeded []Combo
		for _, def := range DefaultCombos {
			def.CreatedAt = now
			s.AutoProvisionPayloads(def)
			seeded = append(seeded, def)
		}
		if err := s.save(seeded); err != nil {
			return nil, err
		}
		return seeded, nil
	}

	modified := false
	for i := range list {
		if list[i].OS == "" {
			list[i].OS = OSWindows
			modified = true
		}
	}
	for _, def := range DefaultCombos {
		found := false
		for i := range list {
			if strings.EqualFold(list[i].Name, def.Name) {
				found = true
				s.AutoProvisionPayloads(def)
				if strings.Contains(list[i].Sequence, "->") || strings.Contains(list[i].Sequence, "raw:") || strings.Contains(list[i].Sequence, "type:") || strings.Contains(list[i].Sequence, "$") {
					list[i].Sequence = def.Sequence
					list[i].Payloads = def.Payloads
					list[i].Description = def.Description
					modified = true
				}
				if list[i].OS == "" && def.OS != "" {
					list[i].OS = def.OS
					modified = true
				}
				if list[i].Description == "" && def.Description != "" {
					list[i].Description = def.Description
					modified = true
				}
				if list[i].Emoji == "" && def.Emoji != "" {
					list[i].Emoji = def.Emoji
					modified = true
				}
				break
			}
		}
		if !found {
			def.CreatedAt = time.Now().UTC()
			s.AutoProvisionPayloads(def)
			list = append(list, def)
			modified = true
		}
	}
	if modified {
		if err := s.save(list); err != nil {
			return nil, err
		}
	}
	return list, nil
}

// ToggleFavorite flips the favorite status of a named combo and persists.
func (s *Store) ToggleFavorite(name string) ([]Combo, bool, error) {
	list, err := s.Load()
	if err != nil {
		return nil, false, err
	}
	found := false
	for i := range list {
		if strings.EqualFold(list[i].Name, strings.TrimSpace(name)) {
			list[i].Favorite = !list[i].Favorite
			found = true
			break
		}
	}
	if !found {
		return list, false, nil
	}
	return list, true, s.save(list)
}

// Favorites returns all combos marked as favorite.
func (s *Store) Favorites() ([]Combo, error) {
	list, err := s.Load()
	if err != nil {
		return nil, err
	}
	var favs []Combo
	for _, c := range list {
		if c.Favorite {
			favs = append(favs, c)
		}
	}
	return favs, nil
}

// ValidateName trims and checks a combo name.
func ValidateName(name string) (string, error) {
	clean := strings.TrimSpace(name)
	if clean == "" {
		return "", errors.New("combo name is required")
	}
	if len([]rune(clean)) > 40 {
		return "", errors.New("combo name must be 40 characters or fewer")
	}
	return clean, nil
}

// Validate checks that every step is valid in the bracket sequence DSL.
func Validate(sequence string) error {
	return ValidateForOS(sequence, OSWindows)
}

// ValidateForOS validates a combo sequence against a specific target operating system.
func ValidateForOS(sequence string, osName string) error {
	steps, err := hid.ParseSequence(sequence)
	if err != nil {
		return err
	}
	if len(steps) == 0 {
		return errors.New("combo has no steps")
	}

	isMac := NormalizeOSDefault(osName) == OSMacOS

	for _, step := range steps {
		if step.Payload != "" || step.Raw != "" || step.Mouse != nil || step.Delay > 0 {
			continue
		}
		if step.Text != "" {
			for _, r := range step.Text {
				if r < 0x20 || r > 0x7E {
					return fmt.Errorf("typed text supports printable ASCII only (found %q)", r)
				}
			}
			continue
		}
		for _, token := range strings.Split(step.Combo, "+") {
			trimmed := strings.TrimSpace(token)
			if !isMac && isMacAlias(trimmed) {
				return fmt.Errorf("%q is a macOS key name; use a Windows key such as win, ctrl or alt", trimmed)
			}
			if !hid.IsWindowsKeyToken(trimmed) && !(isMac && isMacAlias(trimmed)) {
				return fmt.Errorf("unknown key %q", trimmed)
			}
		}
	}
	return nil
}

func isMacAlias(token string) bool {
	switch strings.ToLower(strings.TrimSpace(token)) {
	case "cmd", "command", "super", "meta", "option", "opt":
		return true
	}
	return false
}

// Add inserts a new combo or replaces an existing one with the same name
// (case-insensitive) in place. Also auto-provisions any bundled payloads.
func (s *Store) Add(c Combo) ([]Combo, error) {
	name, err := ValidateName(c.Name)
	if err != nil {
		return nil, err
	}
	osName, err := NormalizeOS(c.OS)
	if err != nil {
		return nil, err
	}
	em, err := ValidateEmoji(c.Emoji)
	if err != nil {
		return nil, err
	}
	seq := strings.TrimSpace(c.Sequence)
	if err := ValidateForOS(seq, osName); err != nil {
		return nil, err
	}
	osSpecified := strings.TrimSpace(c.OS) != ""

	list, err := s.Load()
	if err != nil {
		return nil, err
	}

	c.Name = name
	c.Sequence = seq
	c.OS = osName
	c.Emoji = em
	if c.CreatedAt.IsZero() {
		c.CreatedAt = time.Now().UTC()
	}

	s.AutoProvisionPayloads(c)

	replaced := false
	for i := range list {
		if strings.EqualFold(list[i].Name, name) {
			if !c.Favorite && list[i].Favorite {
				c.Favorite = true
			}
			if !osSpecified && list[i].OS != "" {
				c.OS = list[i].OS
			}
			if c.Description == "" && list[i].Description != "" {
				c.Description = list[i].Description
			}
			if c.Emoji == "" && list[i].Emoji != "" {
				c.Emoji = list[i].Emoji
			}
			list[i] = c
			replaced = true
			break
		}
	}
	if !replaced {
		if len(list) >= 200 {
			return nil, errors.New("combo library is full (200 entries)")
		}
		list = append(list, c)
	}

	return list, s.save(list)
}

// Delete removes the combo with the given name.
func (s *Store) Delete(name string) ([]Combo, bool, error) {
	list, err := s.Load()
	if err != nil {
		return nil, false, err
	}

	remaining := make([]Combo, 0, len(list))
	deleted := false
	for _, c := range list {
		if !deleted && strings.EqualFold(c.Name, strings.TrimSpace(name)) {
			deleted = true
			continue
		}
		remaining = append(remaining, c)
	}
	if !deleted {
		return list, false, nil
	}
	return remaining, true, s.save(remaining)
}

// Find returns the combo with the given name (case-insensitive).
// Falls back to built-in system combos (Login, Lock, Sleep) if not in the stored list.
func (s *Store) Find(name string) (Combo, bool) {
	list, err := s.Load()
	if err == nil {
		for _, c := range list {
			if strings.EqualFold(c.Name, strings.TrimSpace(name)) {
				return c, true
			}
		}
	}
	for _, def := range DefaultCombos {
		if strings.EqualFold(def.Name, strings.TrimSpace(name)) {
			return def, true
		}
	}
	return Combo{}, false
}
