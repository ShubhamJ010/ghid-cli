package cli

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ghost-hid/ghid/internal/combos"
)

func TestComboLibraryCommands(t *testing.T) {
	app, fakeRunner, _, _, stdout, stderr := setupTestApp(t)

	// Save a named Windows combo.
	if code := app.Run([]string{"combo", "save", "OpenRun", "win+r"}); code != 0 {
		t.Fatalf("combo save failed: %s", stderr.String())
	}
	if code := app.Run([]string{"combo", "save", "Click", "click-left"}); code != 0 {
		t.Fatalf("combo save mouse failed: %s", stderr.String())
	}

	// List as JSON.
	stdout.Reset()
	if code := app.Run([]string{"combo", "list", "--json"}); code != 0 {
		t.Fatalf("combo list failed: %s", stderr.String())
	}
	var list []combos.Combo
	if err := json.Unmarshal(stdout.Bytes(), &list); err != nil {
		t.Fatalf("invalid combo list json: %v\n%s", err, stdout.String())
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 combos, got %d (%+v)", len(list), list)
	}

	// Send by name, case-insensitive.
	stdout.Reset()
	if code := app.Run([]string{"combo", "send", "openrun", "--json"}); code != 0 {
		t.Fatalf("combo send failed: %s", stderr.String())
	}
	var sent map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &sent); err != nil || sent["name"] != "OpenRun" {
		t.Fatalf("unexpected send json: %s", stdout.String())
	}
	assertComboBroadcast(t, fakeRunner.Calls, "win+r", false)

	// Send a mouse gesture by name.
	stdout.Reset()
	if code := app.Run([]string{"combo", "send", "Click", "--json"}); code != 0 {
		t.Fatalf("combo send mouse failed: %s", stderr.String())
	}
	assertComboBroadcast(t, fakeRunner.Calls, "click-left", true)

	// One-shot mouse gesture.
	stdout.Reset()
	if code := app.Run([]string{"combo", "mouse", "scroll-up", "--json"}); code != 0 {
		t.Fatalf("combo mouse failed: %s", stderr.String())
	}
	assertComboBroadcast(t, fakeRunner.Calls, "scroll-up", true)

	// Delete without confirmation is rejected by the missing prompt input.
	if code := app.Run([]string{"combo", "delete", "OpenRun", "-f"}); code != 0 {
		t.Fatalf("combo delete failed: %s", stderr.String())
	}
	if list, _ := app.comboStore().Load(); len(list) != 1 {
		t.Fatalf("expected 1 combo after delete, got %+v", list)
	}
}

func TestComboRejectsMacOnlyKeys(t *testing.T) {
	app, _, _, _, _, stderr := setupTestApp(t)
	if code := app.Run([]string{"combo", "save", "Spotlight", "cmd+space"}); code == 0 {
		t.Fatal("expected macOS-only combo to be rejected")
	}
	if !strings.Contains(stderr.String(), "macOS") {
		t.Fatalf("error should explain macOS-only key: %s", stderr.String())
	}
}

func TestComboAllowsMacKeysWhenTargetIsMacOS(t *testing.T) {
	app, _, _, _, _, stderr := setupTestApp(t)
	if code := app.Run([]string{"combo", "save", "Spotlight", "cmd+space", "--os", "macos", "--json"}); code != 0 {
		t.Fatalf("saving macOS combo failed: %s", stderr.String())
	}
	c, found := app.comboStore().Find("Spotlight")
	if !found {
		t.Fatalf("expected Spotlight to be found in store")
	}
	if c.Keys != "cmd+space" || c.OS != "macos" {
		t.Fatalf("unexpected combo stored: %+v", c)
	}
}

func TestComboAllowsRawSequences(t *testing.T) {
	app, _, _, _, _, stderr := setupTestApp(t)
	keys := "raw:cd00 ->{0.06} raw:0000"
	if code := app.Run([]string{"combo", "save", "PlayPause", keys, "--json"}); code != 0 {
		t.Fatalf("saving raw combo failed: %s", stderr.String())
	}
	c, found := app.comboStore().Find("PlayPause")
	if !found {
		t.Fatalf("expected PlayPause to be found in store")
	}
	if c.Keys != keys {
		t.Fatalf("unexpected combo stored: %+v", c)
	}
}

func TestComboBareFallsBackToListingWithoutTTY(t *testing.T) {
	app, _, _, _, stdout, _ := setupTestApp(t)
	if code := app.Run([]string{"combo"}); code != 0 {
		t.Fatal("bare combo should succeed when stdin is not a terminal")
	}
	if !strings.Contains(stdout.String(), "COMBO STUDIO") {
		t.Fatalf("expected combo library listing:\n%s", stdout.String())
	}
}

func assertComboBroadcast(t *testing.T, calls [][]string, want string, mouse bool) {
	t.Helper()
	for _, call := range calls {
		joined := strings.Join(call, " ")
		if mouse && strings.Contains(joined, "cmd mouse") && strings.Contains(joined, "action "+want) {
			return
		}
		if !mouse && strings.Contains(joined, "cmd combo") && strings.Contains(joined, "keys "+want) {
			return
		}
	}
	t.Fatalf("no %s broadcast for %q found in %v", map[bool]string{true: "mouse", false: "combo"}[mouse], want, calls)
}

func TestComboAgentWorkflow(t *testing.T) {
	app, fakeRunner, _, _, stdout, stderr := setupTestApp(t)

	// Schema discovery.
	stdout.Reset()
	if code := app.Run([]string{"combo", "schema", "--json"}); code != 0 {
		t.Fatalf("combo schema failed: %s", stderr.String())
	}
	var schema map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &schema); err != nil {
		t.Fatalf("schema is not JSON: %v\n%s", err, stdout.String())
	}
	if schema["windows_only"] != true || schema["example"] == "" {
		t.Fatalf("incomplete schema: %s", stdout.String())
	}

	keys := "win+s ->{0.7} type:rustdesk ->{0.4} enter"

	// Validation returns parsed steps.
	stdout.Reset()
	if code := app.Run([]string{"combo", "validate", keys, "--json"}); code != 0 {
		t.Fatalf("validate failed: %s", stderr.String())
	}
	var validation struct {
		Valid bool             `json:"valid"`
		Steps []map[string]any `json:"steps"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &validation); err != nil {
		t.Fatalf("invalid validation json: %v", err)
	}
	if !validation.Valid || len(validation.Steps) != 3 {
		t.Fatalf("unexpected validation: %s", stdout.String())
	}
	if validation.Steps[1]["type"] != "text" || validation.Steps[1]["value"] != "rustdesk" {
		t.Fatalf("text step not described: %+v", validation.Steps[1])
	}

	// Invalid combos exit 1 with JSON, not stderr.
	stdout.Reset()
	if code := app.Run([]string{"combo", "validate", "cmd+space", "--json"}); code != 1 {
		t.Fatalf("expected exit 1 for invalid combo, got %d", code)
	}
	var invalid map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &invalid); err != nil || invalid["valid"] != false {
		t.Fatalf("invalid combo should return JSON: %s", stdout.String())
	}

	// Save is non-interactive and reports created vs updated.
	stdout.Reset()
	if code := app.Run([]string{"combo", "save", "Open RustDesk", keys, "--json"}); code != 0 {
		t.Fatalf("agent save failed: %s", stderr.String())
	}
	var saved struct {
		Status  string       `json:"status"`
		Updated bool         `json:"updated"`
		Combo   combos.Combo `json:"combo"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &saved); err != nil || saved.Status != "saved" || saved.Updated {
		t.Fatalf("unexpected save json: %s", stdout.String())
	}
	if saved.Combo.Keys != keys {
		t.Fatalf("saved keys = %q, want %q", saved.Combo.Keys, keys)
	}

	stdout.Reset()
	if code := app.Run([]string{"combo", "save", "Open RustDesk", "win+r", "--json"}); code != 0 {
		t.Fatalf("agent update failed: %s", stderr.String())
	}
	if err := json.Unmarshal(stdout.Bytes(), &saved); err != nil || !saved.Updated {
		t.Fatalf("expected updated=true on second save: %s", stdout.String())
	}
	// Restore the useful version.
	if code := app.Run([]string{"combo", "save", "Open RustDesk", keys, "--json"}); code != 0 {
		t.Fatalf("restore failed: %s", stderr.String())
	}

	// Send by name and confirm the text step reached the gateway.
	stdout.Reset()
	if code := app.Run([]string{"combo", "send", "Open RustDesk", "--json"}); code != 0 {
		t.Fatalf("agent send failed: %s", stderr.String())
	}
	assertCallContains(t, fakeRunner.Calls, "cmd type --es text rustdesk")

	// Delete works without a prompt in JSON mode.
	stdout.Reset()
	if code := app.Run([]string{"combo", "delete", "Open RustDesk", "--json"}); code != 0 {
		t.Fatalf("agent delete failed: %s", stderr.String())
	}
	if list, _ := app.comboStore().Load(); len(list) != 0 {
		t.Fatalf("expected empty library, got %+v", list)
	}
}

func assertCallContains(t *testing.T, calls [][]string, want string) {
	t.Helper()
	for _, call := range calls {
		if strings.Contains(strings.Join(call, " "), want) {
			return
		}
	}
	t.Fatalf("no call containing %q in %v", want, calls)
}

func TestComboSaveWithOSAndDescription(t *testing.T) {
	app, _, _, _, stdout, stderr := setupTestApp(t)

	// 1. Save with flags after positional
	stdout.Reset()
	stderr.Reset()
	if code := app.Run([]string{"combo", "save", "Login", "ctrl+alt+del", "--os", "macos", "--desc", "macOS unlock", "--fav"}); code != 0 {
		t.Fatalf("combo save with os/desc failed: %s", stderr.String())
	}
	saved, ok := app.comboStore().Find("Login")
	if !ok {
		t.Fatal("combo Login not found")
	}
	if saved.OS != combos.OSMacOS || saved.Description != "macOS unlock" || !saved.Favorite {
		t.Fatalf("unexpected saved combo: %+v", saved)
	}

	// 2. Add with flags before positional
	stdout.Reset()
	stderr.Reset()
	if code := app.Run([]string{"combo", "add", "--os", "linux", "--desc", "Lock screen", "Lock", "win+l"}); code != 0 {
		t.Fatalf("combo add with flags before positional failed: %s", stderr.String())
	}
	lockCombo, ok := app.comboStore().Find("Lock")
	if !ok {
		t.Fatal("combo Lock not found")
	}
	if lockCombo.OS != combos.OSLinux || lockCombo.Description != "Lock screen" {
		t.Fatalf("unexpected lock combo: %+v", lockCombo)
	}

	// 3. Shorthand flags with equal signs
	stdout.Reset()
	stderr.Reset()
	if code := app.Run([]string{"combo", "save", "-o=windows", "-d=Quick run", "Run", "win+r"}); code != 0 {
		t.Fatalf("combo save shorthand failed: %s", stderr.String())
	}
	runCombo, ok := app.comboStore().Find("Run")
	if !ok {
		t.Fatal("combo Run not found")
	}
	if runCombo.OS != combos.OSWindows || runCombo.Description != "Quick run" {
		t.Fatalf("unexpected run combo: %+v", runCombo)
	}

	// 4. Unknown OS is rejected
	stdout.Reset()
	stderr.Reset()
	if code := app.Run([]string{"combo", "save", "Bogus", "win+r", "--os", "beos", "--json"}); code != 1 {
		t.Fatalf("expected unknown os to fail, got exit %d", code)
	}
	if !strings.Contains(stdout.String(), "unknown os") {
		t.Fatalf("expected unknown os error, got %s", stdout.String())
	}

	// 5. Listing in non-TTY mode shows OS tags and descriptions aligned
	stdout.Reset()
	if code := app.Run([]string{"combo", "list"}); code != 0 {
		t.Fatalf("combo list failed: %s", stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "[mac]") || !strings.Contains(out, "macOS unlock") {
		t.Fatalf("listing should contain OS tag and description:\n%s", out)
	}

	// 6. OS filter narrows the library
	stdout.Reset()
	if code := app.Run([]string{"combo", "list", "--os", "linux", "--json"}); code != 0 {
		t.Fatalf("combo list --os failed: %s", stderr.String())
	}
	var listed []map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &listed); err != nil {
		t.Fatalf("parse filtered list: %v", err)
	}
	if len(listed) != 1 || listed[0]["name"] != "Lock" {
		t.Fatalf("expected only the linux combo, got %+v", listed)
	}
}
