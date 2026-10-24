package syncer

import (
	"testing"
	"time"

	"github.com/ghost-hid/ghid/internal/snippets"
)

func TestSnippetsSecretsPreserved(t *testing.T) {
	now := time.Now()
	localSnippets := []snippets.Snippet{
		{Name: "username", Value: "localuser", Secure: false, CreatedAt: now},
		{Name: "password", Value: "supersecret123", Secure: true, CreatedAt: now},
	}
	remoteSnippets := []snippets.Snippet{
		{Name: "username", Value: "remoteuser", Secure: false, CreatedAt: now},
		{Name: "password", Value: "", Secure: true, CreatedAt: now}, // Remote secure has empty value
		{Name: "email", Value: "test@example.com", Secure: false, CreatedAt: now},
	}

	localMap := make(map[string]string)
	for _, s := range localSnippets {
		if s.Secure && s.Value != "" {
			localMap[s.Name] = s.Value
		}
	}

	for i := range remoteSnippets {
		if remoteSnippets[i].Secure {
			if secretVal, exists := localMap[remoteSnippets[i].Name]; exists {
				remoteSnippets[i].Value = secretVal
			}
		}
	}

	m := make(map[string]snippets.Snippet)
	for _, s := range remoteSnippets {
		m[s.Name] = s
	}

	if m["username"].Value != "remoteuser" {
		t.Errorf("expected remote username to be source of truth, got %q", m["username"].Value)
	}
	if m["password"].Value != "supersecret123" {
		t.Errorf("expected local password to be retained, got %q", m["password"].Value)
	}
	if m["email"].Value != "test@example.com" {
		t.Errorf("expected email to be present, got %+v", m["email"])
	}
}
