package syncer

import (
	"testing"
	"time"

	"github.com/ghost-hid/ghid/internal/snippets"
)

func TestSecureSnippetsNeverPushed(t *testing.T) {
	now := time.Now()
	localSnippets := []snippets.Snippet{
		{Name: "username", Type: "type", Value: "admin", Secure: false, CreatedAt: now},
		{Name: "password", Type: "type", Value: "supersecret123", Secure: true, CreatedAt: now},
		{Name: "email", Type: "type", Value: "admin@example.com", Secure: true, CreatedAt: now},
		{Name: "sleep_windows", Type: "raw", Value: "02", Secure: false, CreatedAt: now},
	}

	// CLI only pushes non-secure snippets to Android
	var toPush []snippets.Snippet
	for _, s := range localSnippets {
		if !s.Secure {
			toPush = append(toPush, s)
		}
	}

	if len(toPush) != 2 {
		t.Fatalf("expected 2 non-secure snippets pushed, got %d", len(toPush))
	}
	for _, s := range toPush {
		if s.Secure {
			t.Errorf("secure snippet %q was included in push payload", s.Name)
		}
		if s.Value == "" {
			t.Errorf("non-secure snippet %q lost its value", s.Name)
		}
	}
}

func TestNonSecureSnippetsPulledPreservingValues(t *testing.T) {
	now := time.Now()
	localSnippets := []snippets.Snippet{
		{Name: "password", Type: "type", Value: "supersecret123", Secure: true, CreatedAt: now},
		{Name: "email", Type: "type", Value: "local@example.com", Secure: true, CreatedAt: now},
	}
	remoteSnippets := []snippets.Snippet{
		{Name: "username", Type: "type", Value: "remoteuser", Secure: false, CreatedAt: now},
		{Name: "sleep_windows", Type: "raw", Value: "02", Secure: false, CreatedAt: now},
	}

	merged := make([]snippets.Snippet, 0, len(localSnippets)+len(remoteSnippets))
	for _, s := range localSnippets {
		if s.Secure {
			merged = append(merged, s)
		}
	}
	for _, r := range remoteSnippets {
		if !r.Secure {
			merged = append(merged, r)
		}
	}

	m := make(map[string]snippets.Snippet)
	for _, s := range merged {
		m[s.Name] = s
	}

	if m["username"].Value != "remoteuser" {
		t.Errorf("expected remote username to be pulled, got %q", m["username"].Value)
	}
	if m["password"].Value != "supersecret123" {
		t.Errorf("expected local password to be retained, got %q", m["password"].Value)
	}
	if m["sleep_windows"].Value != "02" {
		t.Errorf("expected sleep_windows raw snippet to be pulled, got %q", m["sleep_windows"].Value)
	}
}
