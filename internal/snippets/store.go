package snippets

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/ghost-hid/ghid/internal/hid"
	"github.com/ghost-hid/ghid/internal/vault"
)

// Store manages snippets stored on disk (JSON) and secure snippets backed by Vault (Keychain).
type Store struct {
	Path  string
	Vault vault.Vault
	mu    sync.RWMutex
}

// GetDefaultPath returns the default path to snippets.json in the user config dir.
func GetDefaultPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".config", "ghid", "snippets.json")
	}
	return filepath.Join(home, ".config", "ghid", "snippets.json")
}

// NewStore initializes a snippets Store.
func NewStore(path string, v vault.Vault) *Store {
	if strings.TrimSpace(path) == "" {
		path = GetDefaultPath()
	}
	return &Store{
		Path:  path,
		Vault: v,
	}
}

func (s *Store) vaultKey(name string) string {
	return "ghid_snippet_" + strings.ToLower(strings.TrimSpace(name))
}

// Load loads all snippets from disk and populates secure snippet values from Vault.
func (s *Store) Load() ([]Snippet, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	data, err := os.ReadFile(s.Path)
	if err != nil {
		if os.IsNotExist(err) {
			return []Snippet{}, nil
		}
		return nil, err
	}

	if len(strings.TrimSpace(string(data))) == 0 {
		return []Snippet{}, nil
	}

	var list []Snippet
	if err := json.Unmarshal(data, &list); err != nil {
		return nil, fmt.Errorf("parse snippets: %w", err)
	}

	// Populate secure snippets from vault and normalize types
	for i := range list {
		if list[i].Type == "" {
			list[i].Type = "type"
		}
		if list[i].Secure {
			if s.Vault != nil && s.Vault.IsAvailable() {
				val, err := s.Vault.Get(s.vaultKey(list[i].Name))
				if err == nil && val != "" {
					list[i].Value = val
				}
			}
			// Fallback check for legacy password key
			if strings.EqualFold(list[i].Name, "password") && list[i].Value == "" && s.Vault != nil && s.Vault.IsAvailable() {
				legacyPass, err := s.Vault.Get("ghid_password")
				if err == nil && legacyPass != "" {
					list[i].Value = legacyPass
				}
			}
		}
	}

	return list, nil
}

// Resolver returns a hid.VariableResolver backed by this store.
func (s *Store) Resolver() hid.VariableResolver {
	return func(name string) (string, error) {
		if s == nil {
			return "", fmt.Errorf("snippets store is unavailable")
		}
		snip, ok, err := s.Get(name)
		if err != nil {
			return "", err
		}
		if !ok {
			// Handle aliases
			if strings.EqualFold(name, "sleep") {
				if sn, ok2, _ := s.Get("sleep_windows"); ok2 {
					return sn.Value, nil
				}
			} else if strings.EqualFold(name, "sleep_windows") {
				if sn, ok2, _ := s.Get("sleep"); ok2 {
					return sn.Value, nil
				}
			} else if strings.EqualFold(name, "playpause") {
				if sn, ok2, _ := s.Get("play_pause"); ok2 {
					return sn.Value, nil
				}
			} else if strings.EqualFold(name, "play_pause") {
				if sn, ok2, _ := s.Get("playpause"); ok2 {
					return sn.Value, nil
				}
			}
			return "", fmt.Errorf("snippet %q not found", name)
		}
		return snip.Value, nil
	}
}

// List returns all snippets in the store (alias for Load).
func (s *Store) List() ([]Snippet, error) {
	return s.Load()
}

// SaveAll persists a complete list of snippets to disk, properly delegating secure values.
func (s *Store) SaveAll(list []Snippet) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.save(list)
}

func (s *Store) save(list []Snippet) error {
	if list == nil {
		list = []Snippet{}
	}
	if err := os.MkdirAll(filepath.Dir(s.Path), 0700); err != nil {
		return err
	}

	// Make a copy so secure snippet values are not persisted in plaintext to disk
	toSave := make([]Snippet, len(list))
	for i, sn := range list {
		toSave[i] = sn
		if toSave[i].Type == "" {
			toSave[i].Type = "type"
		}
		if sn.Secure && s.Vault != nil && s.Vault.IsAvailable() {
			toSave[i].Value = ""
		}
	}

	data, err := json.MarshalIndent(toSave, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.Path, data, 0600)
}

// EnsureDefaults seeds the default snippets:
// - "username" (type: "type", secure: false)
// - "password" (type: "type", secure: true)
// - "email" (type: "type", secure: true)
// - "sleep_windows" (type: "raw", value: "02", secure: false)
// - "play_pause" (type: "raw", value: "cd00", secure: false)
func (s *Store) EnsureDefaults() ([]Snippet, error) {
	list, err := s.Load()
	if err != nil {
		return nil, err
	}

	hasUser := false
	hasPass := false
	hasEmail := false
	hasSleep := false
	hasPlayPause := false

	for i := range list {
		if strings.EqualFold(list[i].Name, "username") {
			hasUser = true
		}
		if strings.EqualFold(list[i].Name, "password") {
			hasPass = true
		}
		if strings.EqualFold(list[i].Name, "email") {
			hasEmail = true
			if !list[i].Secure {
				list[i].Secure = true
			}
		}
		if strings.EqualFold(list[i].Name, "sleep_windows") || strings.EqualFold(list[i].Name, "sleep") {
			hasSleep = true
			if list[i].Type != "raw" || list[i].Value == "" {
				list[i].Type = "raw"
				list[i].Value = "02"
			}
		}
		if strings.EqualFold(list[i].Name, "play_pause") || strings.EqualFold(list[i].Name, "playpause") {
			hasPlayPause = true
			if list[i].Type != "raw" || list[i].Value == "" {
				list[i].Type = "raw"
				list[i].Value = "cd00"
			}
		}
	}

	now := time.Now().UTC()
	changed := false

	if !hasUser {
		list = append(list, Snippet{
			Name:      "username",
			Type:      "type",
			Value:     "",
			Secure:    false,
			CreatedAt: now,
			UpdatedAt: now,
		})
		changed = true
	}
	if !hasPass {
		list = append(list, Snippet{
			Name:      "password",
			Type:      "type",
			Value:     "",
			Secure:    true,
			CreatedAt: now,
			UpdatedAt: now,
		})
		changed = true
	}
	if !hasEmail {
		list = append(list, Snippet{
			Name:      "email",
			Type:      "type",
			Value:     "",
			Secure:    true,
			CreatedAt: now,
			UpdatedAt: now,
		})
		changed = true
	}
	if !hasSleep {
		list = append(list, Snippet{
			Name:      "sleep_windows",
			Type:      "raw",
			Value:     "02",
			Secure:    false,
			CreatedAt: now,
			UpdatedAt: now,
		})
		changed = true
	}
	if !hasPlayPause {
		list = append(list, Snippet{
			Name:      "play_pause",
			Type:      "raw",
			Value:     "cd00",
			Secure:    false,
			CreatedAt: now,
			UpdatedAt: now,
		})
		changed = true
	}

	if changed {
		s.mu.Lock()
		defer s.mu.Unlock()
		if err := s.save(list); err != nil {
			return nil, err
		}
	}
	return list, nil
}

// Get returns the snippet with the given name (case-insensitive).
func (s *Store) Get(name string) (Snippet, bool, error) {
	list, err := s.Load()
	if err != nil {
		return Snippet{}, false, err
	}
	clean := strings.TrimSpace(name)
	for _, sn := range list {
		if strings.EqualFold(sn.Name, clean) {
			return sn, true, nil
		}
	}
	return Snippet{}, false, nil
}

// Resolve returns the string value of a snippet, suitable for variable expansion.
func (s *Store) Resolve(name string) (string, bool, error) {
	sn, ok, err := s.Get(name)
	if err != nil {
		return "", false, err
	}
	if !ok {
		return "", false, nil
	}
	return sn.Value, true, nil
}

// Set creates or updates a snippet.
// Optional snType can be "type" or "raw" (defaults to "type").
func (s *Store) Set(name, value string, secure bool, snType ...string) (Snippet, error) {
	cleanName, err := ValidateName(name)
	if err != nil {
		return Snippet{}, err
	}

	targetType := "type"
	if len(snType) > 0 && strings.EqualFold(snType[0], "raw") {
		targetType = "raw"
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Load directly without locking inside Load
	data, err := os.ReadFile(s.Path)
	var list []Snippet
	if err == nil && len(strings.TrimSpace(string(data))) > 0 {
		_ = json.Unmarshal(data, &list)
	}

	now := time.Now().UTC()
	var result Snippet
	found := false

	// Handle secure storage
	if secure && s.Vault != nil && s.Vault.IsAvailable() {
		if err := s.Vault.Set(s.vaultKey(cleanName), value); err != nil {
			return Snippet{}, fmt.Errorf("secure storage: %w", err)
		}
	}

	for i := range list {
		if strings.EqualFold(list[i].Name, cleanName) {
			list[i].Name = cleanName
			list[i].Type = targetType
			list[i].Secure = secure
			if !secure || s.Vault == nil || !s.Vault.IsAvailable() {
				list[i].Value = value
			} else {
				list[i].Value = ""
			}
			list[i].UpdatedAt = now
			result = list[i]
			result.Value = value // return full in-memory value
			found = true
			break
		}
	}

	if !found {
		if len(list) >= 200 {
			return Snippet{}, errors.New("snippet library is full (200 entries)")
		}
		newSn := Snippet{
			Name:      cleanName,
			Type:      targetType,
			Secure:    secure,
			CreatedAt: now,
			UpdatedAt: now,
		}
		if !secure || s.Vault == nil || !s.Vault.IsAvailable() {
			newSn.Value = value
		}
		list = append(list, newSn)
		result = newSn
		result.Value = value
	}

	if err := s.save(list); err != nil {
		return Snippet{}, err
	}
	return result, nil
}

// Delete removes a snippet by name.
func (s *Store) Delete(name string) ([]Snippet, bool, error) {
	cleanName := strings.TrimSpace(name)
	if cleanName == "" {
		return nil, false, errors.New("snippet name is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.Path)
	var list []Snippet
	if err == nil && len(strings.TrimSpace(string(data))) > 0 {
		_ = json.Unmarshal(data, &list)
	}

	found := false
	filtered := make([]Snippet, 0, len(list))
	for _, sn := range list {
		if strings.EqualFold(sn.Name, cleanName) {
			found = true
			if sn.Secure && s.Vault != nil && s.Vault.IsAvailable() {
				_ = s.Vault.Delete(s.vaultKey(cleanName))
			}
		} else {
			filtered = append(filtered, sn)
		}
	}

	if !found {
		return list, false, nil
	}

	if err := s.save(filtered); err != nil {
		return nil, false, err
	}
	return filtered, true, nil
}
