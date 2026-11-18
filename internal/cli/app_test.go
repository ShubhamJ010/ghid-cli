package cli

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/ShubhamJ010/ghid-cli/internal/config"
	"github.com/ShubhamJ010/ghid-cli/internal/payloads"
)

type testVault struct {
	data map[string]string
}

func (m *testVault) Get(u string) (string, error) {
	if v, ok := m.data[u]; ok {
		return v, nil
	}
	return "", errors.New("not found")
}
func (m *testVault) Set(u, p string) error {
	m.data[u] = p
	return nil
}
func (m *testVault) Delete(u string) error {
	delete(m.data, u)
	return nil
}
func (m *testVault) IsAvailable() bool { return true }

func TestPayloadCheck(t *testing.T) {
	vlt := &testVault{data: make(map[string]string)}
	pStore := payloads.NewStore(t.TempDir()+"/payloads.json", vlt)
	_, err := pStore.Set("api_key", "secret123", true, "type")
	if err != nil {
		t.Fatalf("failed to set secure payload: %v", err)
	}
	_, err = pStore.Set("username", "admin", false, "type")
	if err != nil {
		t.Fatalf("failed to set plain payload: %v", err)
	}

	var stdout, stderr bytes.Buffer
	app := &App{
		ConfigStore:  config.NewStore(t.TempDir() + "/config.json"),
		PayloadStore: pStore,
		Stdout:       &stdout,
		Stderr:       &stderr,
	}

	// Test payload check with all valid payloads
	code := app.handlePayloadCheck(pStore, "", false)
	if code != 0 {
		t.Fatalf("expected code 0, got %d: %s", code, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "{API_KEY}") || !strings.Contains(out, "{USERNAME}") {
		t.Fatalf("output missing expected tokens: %s", out)
	}
	if !strings.Contains(out, "passed validation") {
		t.Fatalf("expected pass summary, got: %s", out)
	}

	// Test check json output
	stdout.Reset()
	code = app.handlePayloadCheck(pStore, "api_key", true)
	if code != 0 {
		t.Fatalf("expected code 0 for json check, got %d", code)
	}
	if !strings.Contains(stdout.String(), `"valid": true`) {
		t.Fatalf("expected valid: true in json, got: %s", stdout.String())
	}
}

func TestPrintHelpIncludesNewCommands(t *testing.T) {
	var stdout bytes.Buffer
	app := &App{
		Stdout: &stdout,
	}
	app.printHelp()
	out := stdout.String()
	if !strings.Contains(out, "setup [target]") {
		t.Fatalf("help missing 'setup [target]' command")
	}
	if !strings.Contains(out, "payload check") {
		t.Fatalf("help missing 'payload check' command")
	}
	if !strings.Contains(out, "install-apk") {
		t.Fatalf("help missing 'install-apk' command")
	}
}
