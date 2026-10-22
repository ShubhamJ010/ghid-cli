package vault

import (
	"errors"
	"os/exec"
	"runtime"
	"strings"
	"sync"
)

const DefaultServiceName = "ghost-hid"

// Vault defines the interface for secure host credential storage.
type Vault interface {
	Get(username string) (string, error)
	Set(username, password string) error
	Delete(username string) error
	IsAvailable() bool
}

// KeychainVault implements Vault using the macOS Security CLI.
type KeychainVault struct {
	ServiceName string
}

func NewKeychainVault(serviceName string) *KeychainVault {
	if serviceName == "" {
		serviceName = DefaultServiceName
	}
	return &KeychainVault{ServiceName: serviceName}
}

func (k *KeychainVault) IsAvailable() bool {
	if runtime.GOOS != "darwin" {
		return false
	}
	_, err := exec.LookPath("security")
	return err == nil
}

func (k *KeychainVault) Get(username string) (string, error) {
	if username == "" {
		return "", errors.New("empty username")
	}
	if !k.IsAvailable() {
		return "", errors.New("keychain not available on this platform")
	}

	cmd := exec.Command("/usr/bin/security", "find-generic-password", "-a", username, "-s", k.ServiceName, "-w")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func (k *KeychainVault) Set(username, password string) error {
	if username == "" || password == "" {
		return errors.New("username or password cannot be empty")
	}
	if !k.IsAvailable() {
		return errors.New("keychain not available on this platform")
	}

	cmd := exec.Command("/usr/bin/security", "add-generic-password", "-a", username, "-s", k.ServiceName, "-w", password, "-U")
	return cmd.Run()
}

func (k *KeychainVault) Delete(username string) error {
	if username == "" || !k.IsAvailable() {
		return nil
	}
	cmd := exec.Command("/usr/bin/security", "delete-generic-password", "-a", username, "-s", k.ServiceName)
	_ = cmd.Run()
	return nil
}

// MemoryVault provides an in-memory test stand-in for Vault.
type MemoryVault struct {
	mu        sync.RWMutex
	store     map[string]string
	available bool
}

func NewMemoryVault() *MemoryVault {
	return &MemoryVault{
		store:     make(map[string]string),
		available: true,
	}
}

func (m *MemoryVault) SetAvailable(avail bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.available = avail
}

func (m *MemoryVault) IsAvailable() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.available
}

func (m *MemoryVault) Get(username string) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if !m.available {
		return "", errors.New("vault not available")
	}
	if username == "" {
		return "", errors.New("empty username")
	}
	pass, ok := m.store[username]
	if !ok {
		return "", errors.New("password not found")
	}
	return pass, nil
}

func (m *MemoryVault) Set(username, password string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.available {
		return errors.New("vault not available")
	}
	if username == "" || password == "" {
		return errors.New("username or password cannot be empty")
	}
	m.store[username] = password
	return nil
}

func (m *MemoryVault) Delete(username string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.store, username)
	return nil
}
