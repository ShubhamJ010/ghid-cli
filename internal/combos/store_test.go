package combos

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/ghost-hid/ghid/internal/payloads"
)

func TestComboAutoProvisioning(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "ghid_combos_test_*")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	pPath := filepath.Join(tmpDir, "payloads.json")
	pStore := payloads.NewStore(pPath, nil)

	cPath := filepath.Join(tmpDir, "combos.json")
	cStore := NewStore(cPath, pStore)

	testCombo := Combo{
		Name:     "TestCombo",
		Sequence: "test@domain.com{Tab}{API_KEY}{Enter}{500ms}{mod+s}{100ms}{WIN_SLEEP}",
		Payloads: map[string]payloads.Payload{
			"API_KEY": {
				Name:   "API_KEY",
				Type:   "type",
				Value:  "secret_token_1234",
				Secure: false,
			},
			"WIN_SLEEP": {
				Name:   "WIN_SLEEP",
				Type:   "raw",
				Value:  "02",
				Secure: false,
			},
		},
	}

	_, err = cStore.Add(testCombo)
	if err != nil {
		t.Fatalf("failed to add combo: %v", err)
	}

	// Verify auto-provisioning
	apiKey, ok, err := pStore.Get("API_KEY")
	if err != nil || !ok {
		t.Fatalf("expected API_KEY to be auto-provisioned: %v, ok=%v", err, ok)
	}
	if apiKey.Value != "secret_token_1234" {
		t.Errorf("expected secret_token_1234, got %q", apiKey.Value)
	}

	winSleep, ok, err := pStore.Get("WIN_SLEEP")
	if err != nil || !ok {
		t.Fatalf("expected WIN_SLEEP to be auto-provisioned: %v, ok=%v", err, ok)
	}
	if winSleep.Value != "02" {
		t.Errorf("expected 02, got %q", winSleep.Value)
	}
}

func TestComboLegacyKeysUnmarshal(t *testing.T) {
	rawJSON := `{"name":"LegacyTest","keys":"{mod+l}","os":"windows"}`
	var c Combo
	if err := json.Unmarshal([]byte(rawJSON), &c); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if c.Sequence != "{mod+l}" {
		t.Errorf("expected Sequence '{mod+l}', got %q", c.Sequence)
	}
}
