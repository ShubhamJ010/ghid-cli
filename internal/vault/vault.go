package vault

import (
	"errors"
	"os/exec"
	"runtime"
	"strings"
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
