package vault

import (
	"testing"
)

func TestMemoryVault(t *testing.T) {
	v := NewMemoryVault()

	if !v.IsAvailable() {
		t.Fatal("expected memory vault to be available")
	}

	// Empty username
	if err := v.Set("", "secret"); err == nil {
		t.Error("expected error for empty username")
	}
	if err := v.Set("alice", ""); err == nil {
		t.Error("expected error for empty password")
	}

	// Not found
	if _, err := v.Get("alice"); err == nil {
		t.Error("expected error for nonexistent user")
	}

	// Set & Get
	if err := v.Set("alice", "hunter2"); err != nil {
		t.Fatalf("unexpected error setting password: %v", err)
	}

	pass, err := v.Get("alice")
	if err != nil {
		t.Fatalf("unexpected error getting password: %v", err)
	}
	if pass != "hunter2" {
		t.Errorf("got %q, want %q", pass, "hunter2")
	}

	// Delete
	if err := v.Delete("alice"); err != nil {
		t.Fatalf("unexpected error deleting password: %v", err)
	}
	if _, err := v.Get("alice"); err == nil {
		t.Error("expected error getting deleted password")
	}

	// Availability toggle
	v.SetAvailable(false)
	if v.IsAvailable() {
		t.Error("expected vault to be unavailable")
	}
	if err := v.Set("bob", "secret"); err == nil {
		t.Error("expected error when vault unavailable")
	}
	if _, err := v.Get("bob"); err == nil {
		t.Error("expected error when vault unavailable")
	}
}
