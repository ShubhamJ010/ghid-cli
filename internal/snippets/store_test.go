package snippets

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/ghost-hid/ghid/internal/vault"
)

func TestSnippetEnsureDefaults(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "snippets.json")
	vlt := vault.NewMemoryVault()
	store := NewStore(storePath, vlt)

	list, err := store.EnsureDefaults()
	if err != nil {
		t.Fatalf("EnsureDefaults failed: %v", err)
	}

	if len(list) != 2 {
		t.Fatalf("expected 2 default snippets, got %d", len(list))
	}

	user, ok, err := store.Get("username")
	if err != nil || !ok {
		t.Fatalf("expected username snippet: ok=%v err=%v", ok, err)
	}
	if user.Secure {
		t.Errorf("expected username snippet to be non-secure")
	}
	if user.Value != "" {
		t.Errorf("expected default username to be empty, got %q", user.Value)
	}

	pass, ok, err := store.Get("password")
	if err != nil || !ok {
		t.Fatalf("expected password snippet: ok=%v err=%v", ok, err)
	}
	if !pass.Secure {
		t.Errorf("expected password snippet to be secure")
	}
	if pass.Value != "" {
		t.Errorf("expected default password to be empty, got %q", pass.Value)
	}
}

func TestSnippetCRUD(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "snippets.json")
	vlt := vault.NewMemoryVault()
	store := NewStore(storePath, vlt)

	// Set text snippet
	sn, err := store.Set("username", "admin", false)
	if err != nil {
		t.Fatalf("Set username failed: %v", err)
	}
	if sn.Value != "admin" {
		t.Errorf("expected value 'admin', got %q", sn.Value)
	}

	// Set secure snippet
	sn, err = store.Set("password", "Secret123!", true)
	if err != nil {
		t.Fatalf("Set password failed: %v", err)
	}
	if sn.Value != "Secret123!" {
		t.Errorf("expected value 'Secret123!', got %q", sn.Value)
	}

	// Verify secure snippet is NOT stored plaintext in JSON file on disk
	fileBytes, err := os.ReadFile(storePath)
	if err != nil {
		t.Fatalf("read store file: %v", err)
	}
	var saved []Snippet
	if err := json.Unmarshal(fileBytes, &saved); err != nil {
		t.Fatalf("unmarshal store file: %v", err)
	}
	for _, s := range saved {
		if s.Name == "password" && s.Value != "" {
			t.Fatalf("secure password must NOT be persisted plaintext on disk, got %q", s.Value)
		}
	}

	// Get and resolve secure snippet from store
	val, ok, err := store.Resolve("password")
	if err != nil || !ok || val != "Secret123!" {
		t.Errorf("Resolve password failed: val=%q ok=%v err=%v", val, ok, err)
	}

	// Case-insensitive resolution
	val, ok, err = store.Resolve("PASSWORD")
	if err != nil || !ok || val != "Secret123!" {
		t.Errorf("Case-insensitive Resolve failed: val=%q ok=%v err=%v", val, ok, err)
	}

	// Create custom snippet
	_, err = store.Set("PIN", "9944", true)
	if err != nil {
		t.Fatalf("Set custom snippet failed: %v", err)
	}

	val, ok, _ = store.Resolve("pin")
	if !ok || val != "9944" {
		t.Errorf("Resolve PIN failed: val=%q ok=%v", val, ok)
	}

	// Delete custom snippet
	_, deleted, err := store.Delete("PIN")
	if err != nil || !deleted {
		t.Fatalf("Delete PIN failed: deleted=%v err=%v", deleted, err)
	}
	_, ok, _ = store.Get("PIN")
	if ok {
		t.Errorf("expected PIN to be deleted")
	}
}

func TestSnippetMaskedValue(t *testing.T) {
	s1 := Snippet{Name: "user", Value: "alice", Secure: false}
	if s1.MaskedValue() != "alice" {
		t.Errorf("unexpected masked value: %q", s1.MaskedValue())
	}

	s2 := Snippet{Name: "pass", Value: "secret", Secure: true}
	if s2.MaskedValue() != "••••••••" {
		t.Errorf("unexpected masked value: %q", s2.MaskedValue())
	}

	s3 := Snippet{Name: "pass", Value: "", Secure: true}
	if s3.MaskedValue() != "(empty)" {
		t.Errorf("unexpected masked value: %q", s3.MaskedValue())
	}
}

func TestSnippetValidateName(t *testing.T) {
	valid := []string{"username", "PASSWORD", "$user", "${token}", "my_api_key-1"}
	for _, n := range valid {
		if _, err := ValidateName(n); err != nil {
			t.Errorf("expected valid name %q, got error: %v", n, err)
		}
	}

	invalid := []string{"", "   ", "bad name with spaces", "hello@world", "bad*key"}
	for _, n := range invalid {
		if _, err := ValidateName(n); err == nil {
			t.Errorf("expected invalid name %q to fail", n)
		}
	}
}
