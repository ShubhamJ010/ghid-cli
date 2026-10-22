package keychain

import (
	"errors"
	"os/exec"
	"runtime"
	"strings"
)

const ServiceName = "ghost-hid"

func IsAvailable() bool {
	if runtime.GOOS != "darwin" {
		return false
	}
	_, err := exec.LookPath("security")
	return err == nil
}

func GetPassword(username string) (string, error) {
	if username == "" {
		return "", errors.New("empty username")
	}
	if !IsAvailable() {
		return "", errors.New("keychain not available on this platform")
	}

	cmd := exec.Command("/usr/bin/security", "find-generic-password", "-a", username, "-s", ServiceName, "-w")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func SetPassword(username, password string) error {
	if username == "" || password == "" {
		return errors.New("username or password cannot be empty")
	}
	if !IsAvailable() {
		return errors.New("keychain not available on this platform")
	}

	cmd := exec.Command("/usr/bin/security", "add-generic-password", "-a", username, "-s", ServiceName, "-w", password, "-U")
	return cmd.Run()
}

func DeletePassword(username string) error {
	if username == "" || !IsAvailable() {
		return nil
	}
	cmd := exec.Command("/usr/bin/security", "delete-generic-password", "-a", username, "-s", ServiceName)
	_ = cmd.Run()
	return nil
}
