package cli

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ghost-hid/ghid/internal/snippets"
)

func TestSnippetsCLI(t *testing.T) {
	app, _, _, _, stdout, stderr := setupTestApp(t)

	// Set a snippet
	if code := app.Run([]string{"snippets", "set", "api_key", "test-secret-123", "--secure"}); code != 0 {
		t.Fatalf("snippets set failed: %s", stderr.String())
	}

	// Get the snippet
	stdout.Reset()
	if code := app.Run([]string{"snippets", "get", "api_key"}); code != 0 {
		t.Fatalf("snippets get failed: %s", stderr.String())
	}
	if strings.TrimSpace(stdout.String()) != "test-secret-123" {
		t.Fatalf("expected 'test-secret-123', got %q", stdout.String())
	}

	// List snippets as JSON
	stdout.Reset()
	if code := app.Run([]string{"snippets", "list", "--json"}); code != 0 {
		t.Fatalf("snippets list failed: %s", stderr.String())
	}
	var list []snippets.Snippet
	if err := json.Unmarshal(stdout.Bytes(), &list); err != nil {
		t.Fatalf("invalid json: %v (%s)", err, stdout.String())
	}
	found := false
	for _, s := range list {
		if s.Name == "api_key" {
			found = true
			if !s.Secure {
				t.Fatalf("expected api_key to be marked secure")
			}
			break
		}
	}
	if !found {
		t.Fatalf("api_key not found in snippets list")
	}

	// Delete snippet
	if code := app.Run([]string{"snippets", "delete", "api_key"}); code != 0 {
		t.Fatalf("snippets delete failed: %s", stderr.String())
	}

	// Verify deletion
	stdout.Reset()
	if code := app.Run([]string{"snippets", "get", "api_key"}); code == 0 {
		t.Fatalf("expected deleted snippet get to fail")
	}

	// Deleting default snippets must fail
	stderr.Reset()
	if code := app.Run([]string{"snippets", "delete", "username"}); code == 0 {
		t.Fatalf("expected delete username to fail")
	}
	if !strings.Contains(stderr.String(), "cannot be deleted") {
		t.Fatalf("expected protected snippet error message: %s", stderr.String())
	}
}

func TestComboFavoriteAndVariableCLI(t *testing.T) {
	app, fakeRunner, _, _, stdout, stderr := setupTestApp(t)

	// Save snippet values
	if code := app.Run([]string{"snippets", "set", "username", "alice"}); code != 0 {
		t.Fatalf("failed to set username snippet: %s", stderr.String())
	}
	if code := app.Run([]string{"snippets", "set", "password", "supersecret", "--secure"}); code != 0 {
		t.Fatalf("failed to set password snippet: %s", stderr.String())
	}

	// Save combo with variables
	if code := app.Run([]string{"combo", "save", "AutoAuth", "$USERNAME -> tab -> $PASSWORD -> enter", "--fav"}); code != 0 {
		t.Fatalf("failed to save combo: %s", stderr.String())
	}

	// Toggle favorite
	stdout.Reset()
	if code := app.Run([]string{"combo", "fav", "AutoAuth", "--json"}); code != 0 {
		t.Fatalf("failed to toggle favorite: %s", stderr.String())
	}
	var favRes map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &favRes); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if favRes["name"] != "AutoAuth" || favRes["favorite"] != false {
		t.Fatalf("unexpected favorite result: %+v", favRes)
	}

	// Dispatch combo with variable resolution
	stdout.Reset()
	if code := app.Run([]string{"combo", "send", "AutoAuth", "--json"}); code != 0 {
		t.Fatalf("failed to send combo: %s", stderr.String())
	}

	// Assert variables were resolved into literal text broadcasts
	assertTypeBroadcast(t, fakeRunner.Calls, "alice")
	assertComboBroadcast(t, fakeRunner.Calls, "tab", false)
	assertTypeBroadcast(t, fakeRunner.Calls, "supersecret")
	assertComboBroadcast(t, fakeRunner.Calls, "enter", false)
}

func assertTypeBroadcast(t *testing.T, calls [][]string, want string) {
	t.Helper()
	for _, call := range calls {
		joined := strings.Join(call, " ")
		if strings.Contains(joined, "cmd type") && strings.Contains(joined, "text "+want) {
			return
		}
	}
	t.Fatalf("no type broadcast for %q found in %v", want, calls)
}
