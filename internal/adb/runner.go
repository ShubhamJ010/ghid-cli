package adb

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Runner abstracts process execution for ADB commands so callers can swap the transport.
type Runner interface {
	AdbPath() string
	Run(serial string, args ...string) (string, error)
	RunCommand(dir string, name string, args ...string) error
}

// ExecRunner is the production Runner that invokes system executables.
type ExecRunner struct {
	adbPath string
}

// NewExecRunner creates an ExecRunner, locating ADB on the system.
func NewExecRunner() (*ExecRunner, error) {
	adbPath, err := LocateAdb()
	if err != nil {
		return nil, err
	}
	return &ExecRunner{adbPath: adbPath}, nil
}

func (e *ExecRunner) AdbPath() string {
	return e.adbPath
}

func (e *ExecRunner) Run(serial string, args ...string) (string, error) {
	var fullArgs []string
	if serial != "" {
		fullArgs = append(fullArgs, "-s", serial)
	}
	fullArgs = append(fullArgs, args...)

	cmd := exec.Command(e.adbPath, fullArgs...)
	out, err := cmd.CombinedOutput()
	outputStr := strings.TrimSpace(string(out))
	if err != nil {
		return outputStr, fmt.Errorf("adb %s failed: %w (output: %s)", strings.Join(args, " "), err, outputStr)
	}
	return outputStr, nil
}

func (e *ExecRunner) RunCommand(dir string, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// LocateAdb searches for the adb binary in PATH and standard platform directories.
func LocateAdb() (string, error) {
	if env := os.Getenv("ADB_PATH"); env != "" {
		if fi, err := os.Stat(env); err == nil && !fi.IsDir() {
			return env, nil
		}
	}

	if path, err := exec.LookPath("adb"); err == nil {
		return path, nil
	}

	home, _ := os.UserHomeDir()
	commonPaths := []string{
		"/opt/homebrew/bin/adb",
		filepath.Join(home, "Library/Android/sdk/platform-tools/adb"),
		"/usr/local/bin/adb",
		"/usr/bin/adb",
	}

	for _, p := range commonPaths {
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			return p, nil
		}
	}

	return "", errors.New("Android Debug Bridge ('adb') was not found in PATH or standard directories")
}
