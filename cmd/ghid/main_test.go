package main

import (
	"os/exec"
	"strings"
	"testing"
)

func TestMainBinaryBuildAndVersion(t *testing.T) {
	cmd := exec.Command("go", "run", ".", "version")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go run . version failed: %v, output: %s", err, string(out))
	}
	if !strings.Contains(string(out), Version) {
		t.Errorf("expected version %s in output, got: %s", Version, string(out))
	}
}
