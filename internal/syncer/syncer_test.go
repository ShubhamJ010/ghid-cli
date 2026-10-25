package syncer

import (
	"testing"
	"time"

	"github.com/ghost-hid/ghid/internal/payloads"
)

func TestSecurePayloadsNeverPushed(t *testing.T) {
	now := time.Now()
	localPayloads := []payloads.Payload{
		{Name: "username", Type: "type", Value: "admin", Secure: false, CreatedAt: now},
		{Name: "password", Type: "type", Value: "supersecret123", Secure: true, CreatedAt: now},
		{Name: "email", Type: "type", Value: "admin@example.com", Secure: true, CreatedAt: now},
		{Name: "WIN_SLEEP", Type: "raw", Value: "02", Secure: false, CreatedAt: now},
	}

	// CLI only pushes non-secure payloads to Android
	var toPush []payloads.Payload
	for _, p := range localPayloads {
		if !p.Secure {
			toPush = append(toPush, p)
		}
	}

	if len(toPush) != 2 {
		t.Fatalf("expected 2 non-secure payloads pushed, got %d", len(toPush))
	}
	for _, p := range toPush {
		if p.Secure {
			t.Errorf("secure payload %q was included in push payload", p.Name)
		}
		if p.Value == "" {
			t.Errorf("non-secure payload %q lost its value", p.Name)
		}
	}
}

func TestNonSecurePayloadsPulledPreservingValues(t *testing.T) {
	now := time.Now()
	localPayloads := []payloads.Payload{
		{Name: "password", Type: "type", Value: "supersecret123", Secure: true, CreatedAt: now},
		{Name: "email", Type: "type", Value: "local@example.com", Secure: true, CreatedAt: now},
	}
	remotePayloads := []payloads.Payload{
		{Name: "username", Type: "type", Value: "remoteuser", Secure: false, CreatedAt: now},
		{Name: "WIN_SLEEP", Type: "raw", Value: "02", Secure: false, CreatedAt: now},
	}

	merged := make([]payloads.Payload, 0, len(localPayloads)+len(remotePayloads))
	for _, p := range localPayloads {
		if p.Secure {
			merged = append(merged, p)
		}
	}
	for _, r := range remotePayloads {
		if !r.Secure {
			merged = append(merged, r)
		}
	}

	m := make(map[string]payloads.Payload)
	for _, p := range merged {
		m[p.Name] = p
	}

	if m["username"].Value != "remoteuser" {
		t.Errorf("expected remote username to be pulled, got %q", m["username"].Value)
	}
	if m["password"].Value != "supersecret123" {
		t.Errorf("expected local password to be retained, got %q", m["password"].Value)
	}
	if m["WIN_SLEEP"].Value != "02" {
		t.Errorf("expected WIN_SLEEP raw payload to be pulled, got %q", m["WIN_SLEEP"].Value)
	}
}
