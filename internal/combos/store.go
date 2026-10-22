// Package combos persists named Windows keyboard/mouse shortcuts for the
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

	"github.com/ghost-hid/ghid/internal/hid"
)

// Default combo keys shipped with the application.
const (
	DefaultLoginKeys = "ctrl+alt+del ->{1.0} esc ->{0.8} $USERNAME ->{0.8} tab ->{0.8} $PASSWORD ->{0.8} enter"
	DefaultLockKeys  = "win+l"
	DefaultSleepKeys = "win+x ->{0.5} u ->{0.3} s ->{5.0} !DISCONNECT ->{0.5} !EXIT"
)

// Combo is a named, replayable sequence of Windows keyboard chords, mouse
// gestures, typed text and directives. Keys uses the ghid sequence grammar, e.g.
// "win+r ->{0.6} type:rustdesk ->{0.3} enter".
type Combo struct {
	Name        string    `json:"name"`
	Keys        string    `json:"keys"`
	Emoji       string    `json:"emoji,omitempty"`
	Description string    `json:"description,omitempty"`
	Favorite    bool      `json:"favorite,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

// DefaultCombos defines the built-in system combos with their emojis and descriptions.
var DefaultCombos = []Combo{
	{
		Name:        "Login",
		Keys:        DefaultLoginKeys,
		Emoji:       "🔑",
		Description: "Windows unlock & credential injection",
		Favorite:    true,
	},
	{
		Name:        "Lock",
		Keys:        DefaultLockKeys,
		Emoji:       "🔒",
		Description: "Instant Windows lock screen (Win + L)",
		Favorite:    true,
	},
	{
		Name:        "Sleep",
		Keys:        DefaultSleepKeys,
		Emoji:       "🌙",
		Description: "Windows sleep sequence + countdown exit",
		Favorite:    true,
	},
}

// Store reads and writes the combo library on disk.
type Store struct {
	Path string
}

// NewStore creates a store at path. An empty path uses the default location
// (~/.config/ghid/combos.json).
func NewStore(path string) *Store {
	if strings.TrimSpace(path) == "" {
		path = GetDefaultPath()
	}
	return &Store{Path: path}
}

// GetDefaultPath returns the per-user combo library location.
func GetDefaultPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".config", "ghid", "combos.json")
	}
	return filepath.Join(home, ".config", "ghid", "combos.json")
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
	return list, nil
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

// EnsureDefaults seeds the default built-in combos (Login, Lock, Sleep) if empty,
// and ensures existing built-ins have their emoji and description populated.
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
			seeded = append(seeded, def)
		}
		if err := s.save(seeded); err != nil {
			return nil, err
		}
		return seeded, nil
	}

	modified := false
	for _, def := range DefaultCombos {
		found := false
		for i := range list {
			if strings.EqualFold(list[i].Name, def.Name) {
				found = true
				if list[i].Emoji == "" && def.Emoji != "" {
					list[i].Emoji = def.Emoji
					modified = true
				}
				if list[i].Description == "" && def.Description != "" {
					list[i].Description = def.Description
					modified = true
				}
				break
			}
		}
		if !found {
			def.CreatedAt = time.Now().UTC()
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

// Validate checks that every step is a real Windows keyboard chord, variable,
// or one of the supported mouse gestures. macOS-only aliases are rejected on purpose.
func Validate(keys string) error {
	steps, err := hid.ParseActionSequence(keys)
	if err != nil {
		if strings.Contains(err.Error(), "unknown key token") {
			return fmt.Errorf("%w (tip: to type a word, use type:rustdesk)", err)
		}
		return err
	}
	if len(steps) == 0 {
		return errors.New("combo has no steps")
	}

	for _, step := range steps {
		if step.Directive != "" {
			continue
		}
		if step.Variable != "" {
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
		if step.Mouse != nil {
			continue
		}
		for _, token := range strings.Split(step.Combo, "+") {
			if !hid.IsWindowsKeyToken(token) {
				trimmed := strings.TrimSpace(token)
				if isMacAlias(trimmed) {
					return fmt.Errorf("%q is a macOS key name; use a Windows key such as win, ctrl or alt", trimmed)
				}
				return fmt.Errorf("unknown Windows key %q", trimmed)
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
// (case-insensitive) in place. The returned slice is the saved library.
func (s *Store) Add(c Combo) ([]Combo, error) {
	name, err := ValidateName(c.Name)
	if err != nil {
		return nil, err
	}
	keys := strings.TrimSpace(c.Keys)
	if err := Validate(keys); err != nil {
		return nil, err
	}

	list, err := s.Load()
	if err != nil {
		return nil, err
	}

	c.Name = name
	c.Keys = keys
	if c.CreatedAt.IsZero() {
		c.CreatedAt = time.Now().UTC()
	}

	replaced := false
	for i := range list {
		if strings.EqualFold(list[i].Name, name) {
			if !c.Favorite && list[i].Favorite {
				c.Favorite = true
			}
			if c.Emoji == "" && list[i].Emoji != "" {
				c.Emoji = list[i].Emoji
			}
			if c.Description == "" && list[i].Description != "" {
				c.Description = list[i].Description
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
