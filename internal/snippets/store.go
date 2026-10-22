package snippets

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/ghost-hid/ghid/internal/hid"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/ghost-hid/ghid/internal/vault"
)

// Store manages snippet persistence on disk with secure storage offload to Vault.
type Store struct {
	mu    sync.RWMutex
	Path  string
	Vault vault.Vault
}

// NewStore initializes a snippet store at path. If path is empty, default location is used.
func NewStore(path string, vlt vault.Vault) *Store {
	if strings.TrimSpace(path) == "" {
		path = GetDefaultPath()
	}
	return &Store{
		Path:  path,
		Vault: vlt,
	}
}

// GetDefaultPath returns the per-user snippets JSON file location.
func GetDefaultPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".config", "ghid", "snippets.json")
	}
	return filepath.Join(home, ".config", "ghid", "snippets.json")
}

func (s *Store) vaultKey(name string) string {
	return "ghost-hid:snippet:" + strings.ToLower(strings.TrimSpace(name))
}

// Load retrieves all snippets from disk. For secure snippets,
// their values are populated on-the-fly from Vault if available.
func (s *Store) Load() ([]Snippet, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	data, err := os.ReadFile(s.Path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []Snippet{}, nil
		}
		return nil, err
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return []Snippet{}, nil
	}

	var list []Snippet
	if err := json.Unmarshal(data, &list); err != nil {
		return nil, fmt.Errorf("parse %s: %w", s.Path, err)
	}

	// For secure snippets, populate Value from vault if available
	if s.Vault != nil && s.Vault.IsAvailable() {
		for i := range list {
			if list[i].Secure {
				key := s.vaultKey(list[i].Name)
				if val, err := s.Vault.Get(key); err == nil && val != "" {
					list[i].Value = val
				} else if strings.EqualFold(list[i].Name, "password") {
					// Fallback check for old credential format if present
					if legacyPass, lerr := s.Vault.Get("password"); lerr == nil && legacyPass != "" {
						list[i].Value = legacyPass
					}
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
			return "", fmt.Errorf("snippet %q not found", name)
		}
		return snip.Value, nil
	}
}

// List returns all snippets in the store (alias for Load).
func (s *Store) List() ([]Snippet, error) {
	return s.Load()
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

// EnsureDefaults seeds the two default snippets ("username" and "password")
// if they are not already present in the store. Password is a secure snippet.
func (s *Store) EnsureDefaults() ([]Snippet, error) {
	list, err := s.Load()
	if err != nil {
		return nil, err
	}

	hasUser := false
	hasPass := false
	for _, sn := range list {
		if strings.EqualFold(sn.Name, "username") {
			hasUser = true
		}
		if strings.EqualFold(sn.Name, "password") {
			hasPass = true
		}
	}

	now := time.Now().UTC()
	changed := false
	if !hasUser {
		list = append(list, Snippet{
			Name:      "username",
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
			Value:     "",
			Secure:    true,
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
func (s *Store) Set(name, value string, secure bool) (Snippet, error) {
	cleanName, err := ValidateName(name)
	if err != nil {
		return Snippet{}, err
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

	remaining := make([]Snippet, 0, len(list))
	deleted := false
	for _, sn := range list {
		if !deleted && strings.EqualFold(sn.Name, cleanName) {
			deleted = true
			if sn.Secure && s.Vault != nil && s.Vault.IsAvailable() {
				_ = s.Vault.Delete(s.vaultKey(sn.Name))
			}
			continue
		}
		remaining = append(remaining, sn)
	}

	if !deleted {
		return list, false, nil
	}

	if err := s.save(remaining); err != nil {
		return nil, false, err
	}
	return remaining, true, nil
}
