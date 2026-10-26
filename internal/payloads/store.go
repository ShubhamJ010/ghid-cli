package payloads

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

// Store manages payloads stored on disk (JSON) and secure payloads backed by Vault (Keychain).
type Store struct {
	Path  string
	Vault vault.Vault
	mu    sync.RWMutex
}

// GetDefaultPath returns the default path to payloads.json in the user config dir.
func GetDefaultPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".config", "ghid", "payloads.json")
	}
	return filepath.Join(home, ".config", "ghid", "payloads.json")
}

// NewStore initializes a payloads Store.
func NewStore(path string, v vault.Vault) *Store {
	if strings.TrimSpace(path) == "" {
		path = GetDefaultPath()
	}
	return &Store{
		Path:  path,
		Vault: v,
	}
}

// KeychainAccount is the Keychain account name used for a secure payload.
// Exposed so `payload get` and the docs can point at the real item.
func KeychainAccount(name string) string {
	return "ghid_payload_" + strings.ToLower(strings.TrimSpace(name))
}

func (s *Store) vaultKey(name string) string {
	return KeychainAccount(name)
}

// Load loads all payloads from disk and populates secure payload values from Vault.
func (s *Store) Load() ([]Payload, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	data, err := os.ReadFile(s.Path)
	if err != nil {
		if os.IsNotExist(err) {
			// Check legacy snippets.json path for seamless migration
			legacyPath := filepath.Join(filepath.Dir(s.Path), "snippets.json")
			if legData, legErr := os.ReadFile(legacyPath); legErr == nil && len(strings.TrimSpace(string(legData))) > 0 {
				data = legData
			} else {
				return []Payload{}, nil
			}
		} else {
			return nil, err
		}
	}

	if len(strings.TrimSpace(string(data))) == 0 {
		return []Payload{}, nil
	}

	var list []Payload
	if err := json.Unmarshal(data, &list); err != nil {
		return nil, fmt.Errorf("parse payloads: %w", err)
	}

	// Populate secure payloads from vault and normalize types
	for i := range list {
		if list[i].Type == "" {
			list[i].Type = "type"
		}
		if list[i].Secure {
			if s.Vault != nil && s.Vault.IsAvailable() {
				val, err := s.Vault.Get(s.vaultKey(list[i].Name))
				if err == nil && val != "" {
					list[i].Value = val
				} else {
					// Fallback to legacy snippet vault key
					legVal, err := s.Vault.Get("ghid_snippet_" + strings.ToLower(strings.TrimSpace(list[i].Name)))
					if err == nil && legVal != "" {
						list[i].Value = legVal
					}
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

// Resolver returns a hid.PayloadResolver backed by this store.
func (s *Store) Resolver() hid.PayloadResolver {
	return func(name string) (string, error) {
		if s == nil {
			return "", fmt.Errorf("payload store is unavailable")
		}
		p, ok, err := s.Get(name)
		if err != nil {
			return "", err
		}
		if !ok {
			// Handle common legacy aliases
			if strings.EqualFold(name, "sleep") {
				if p2, ok2, _ := s.Get("win_sleep"); ok2 {
					return p2.Value, nil
				}
			} else if strings.EqualFold(name, "sleep_windows") {
				if p2, ok2, _ := s.Get("win_sleep"); ok2 {
					return p2.Value, nil
				}
			} else if strings.EqualFold(name, "playpause") {
				if p2, ok2, _ := s.Get("play_pause"); ok2 {
					return p2.Value, nil
				}
			}
			return "", fmt.Errorf("payload %q not found", name)
		}
		return p.Value, nil
	}
}

// List returns all payloads in the store (alias for Load).
func (s *Store) List() ([]Payload, error) {
	return s.Load()
}

// RefReport classifies payload references from a combo sequence.
// Unresolved means no payload with that name exists; Empty means the payload
// exists but has no usable value (for example a secure payload whose Keychain
// item was never created).
type RefReport struct {
	Referenced []string `json:"referenced"`
	Resolved   []string `json:"resolved"`
	Unresolved []string `json:"unresolved"`
	Empty      []string `json:"empty"`
}

// ResolveRefs resolves payload names the exact way the send path does,
// including legacy aliases, and reports what is missing.
func (s *Store) ResolveRefs(names []string) RefReport {
	report := RefReport{Referenced: []string{}, Resolved: []string{}, Unresolved: []string{}, Empty: []string{}}
	if s == nil {
		for _, name := range names {
			report.Referenced = append(report.Referenced, name)
			report.Unresolved = append(report.Unresolved, name)
		}
		return report
	}

	seen := make(map[string]bool, len(names))
	resolve := s.Resolver()
	for _, name := range names {
		clean := strings.TrimSpace(name)
		key := strings.ToLower(clean)
		if clean == "" || seen[key] {
			continue
		}
		seen[key] = true
		report.Referenced = append(report.Referenced, clean)

		value, err := resolve(clean)
		if err != nil {
			report.Unresolved = append(report.Unresolved, clean)
			continue
		}
		if strings.TrimSpace(value) == "" {
			report.Empty = append(report.Empty, clean)
			continue
		}
		report.Resolved = append(report.Resolved, clean)
	}
	return report
}

// SaveAll persists a complete list of payloads to disk, properly delegating secure values.
func (s *Store) SaveAll(list []Payload) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.save(list)
}

func (s *Store) save(list []Payload) error {
	if list == nil {
		list = []Payload{}
	}
	if err := os.MkdirAll(filepath.Dir(s.Path), 0700); err != nil {
		return err
	}

	// Make a copy so secure payload values are not persisted in plaintext to disk
	toSave := make([]Payload, len(list))
	for i, p := range list {
		toSave[i] = p
		if toSave[i].Type == "" {
			toSave[i].Type = "type"
		}
		if p.Secure && s.Vault != nil && s.Vault.IsAvailable() {
			toSave[i].Value = ""
		}
	}

	data, err := json.MarshalIndent(toSave, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.Path, data, 0600)
}

// EnsureDefaults seeds the default payloads:
// - "username" (type: "type", secure: false)
// - "password" (type: "type", secure: true)
// - "email" (type: "type", secure: true)
// - "WIN_SLEEP" (type: "raw", value: "02", secure: false)
// - "PLAY_PAUSE" (type: "raw", value: "cd00", secure: false)
func (s *Store) EnsureDefaults() ([]Payload, error) {
	list, err := s.Load()
	if err != nil {
		return nil, err
	}

	hasUser := false
	hasPass := false
	hasEmail := false
	hasWinSleep := false
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
		if strings.EqualFold(list[i].Name, "win_sleep") || strings.EqualFold(list[i].Name, "sleep_windows") || strings.EqualFold(list[i].Name, "sleep") {
			hasWinSleep = true
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
		list = append(list, Payload{
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
		list = append(list, Payload{
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
		list = append(list, Payload{
			Name:      "email",
			Type:      "type",
			Value:     "",
			Secure:    true,
			CreatedAt: now,
			UpdatedAt: now,
		})
		changed = true
	}
	if !hasWinSleep {
		list = append(list, Payload{
			Name:      "WIN_SLEEP",
			Type:      "raw",
			Value:     "02",
			Secure:    false,
			CreatedAt: now,
			UpdatedAt: now,
		})
		changed = true
	}
	if !hasPlayPause {
		list = append(list, Payload{
			Name:      "PLAY_PAUSE",
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

// Get returns the payload with the given name (case-insensitive).
func (s *Store) Get(name string) (Payload, bool, error) {
	list, err := s.Load()
	if err != nil {
		return Payload{}, false, err
	}
	clean := strings.TrimSpace(name)
	for _, p := range list {
		if strings.EqualFold(p.Name, clean) {
			return p, true, nil
		}
	}
	return Payload{}, false, nil
}

// Resolve returns the string value of a payload.
func (s *Store) Resolve(name string) (string, bool, error) {
	p, ok, err := s.Get(name)
	if err != nil {
		return "", false, err
	}
	if !ok {
		return "", false, nil
	}
	return p.Value, true, nil
}

// Set creates or updates a payload.
// Optional pType can be "type" or "raw" (defaults to "type").
func (s *Store) Set(name, value string, secure bool, pType ...string) (Payload, error) {
	cleanName, err := ValidateName(name)
	if err != nil {
		return Payload{}, err
	}

	targetType := "type"
	if len(pType) > 0 && strings.EqualFold(pType[0], "raw") {
		targetType = "raw"
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Load directly without locking inside Load
	data, err := os.ReadFile(s.Path)
	var list []Payload
	if err == nil && len(strings.TrimSpace(string(data))) > 0 {
		_ = json.Unmarshal(data, &list)
	}

	now := time.Now().UTC()
	var result Payload
	found := false

	// Handle secure storage. Never persist a value in plaintext while marking
	// it secure: if the vault is unavailable, refuse the write.
	if secure {
		if s.Vault == nil || !s.Vault.IsAvailable() {
			return Payload{}, errors.New("secure payloads require an available vault (macOS Keychain); use a plain payload on this platform")
		}
		if strings.TrimSpace(value) == "" {
			_ = s.Vault.Delete(s.vaultKey(cleanName))
		} else if err := s.Vault.Set(s.vaultKey(cleanName), value); err != nil {
			return Payload{}, fmt.Errorf("secure storage: %w", err)
		}
	} else if s.Vault != nil && s.Vault.IsAvailable() {
		// A payload that used to be secure must not leave a stale Keychain item.
		_ = s.Vault.Delete(s.vaultKey(cleanName))
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
			return Payload{}, errors.New("payload library is full (200 entries)")
		}
		newP := Payload{
			Name:      cleanName,
			Type:      targetType,
			Secure:    secure,
			CreatedAt: now,
			UpdatedAt: now,
		}
		if !secure || s.Vault == nil || !s.Vault.IsAvailable() {
			newP.Value = value
		}
		list = append(list, newP)
		result = newP
		result.Value = value
	}

	if err := s.save(list); err != nil {
		return Payload{}, err
	}
	return result, nil
}

// Delete removes a payload by name.
func (s *Store) Delete(name string) ([]Payload, bool, error) {
	cleanName := strings.TrimSpace(name)
	if cleanName == "" {
		return nil, false, errors.New("payload name is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.Path)
	var list []Payload
	if err == nil && len(strings.TrimSpace(string(data))) > 0 {
		_ = json.Unmarshal(data, &list)
	}

	found := false
	filtered := make([]Payload, 0, len(list))
	for _, p := range list {
		if strings.EqualFold(p.Name, cleanName) {
			found = true
			if p.Secure && s.Vault != nil && s.Vault.IsAvailable() {
				_ = s.Vault.Delete(s.vaultKey(cleanName))
			}
		} else {
			filtered = append(filtered, p)
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
