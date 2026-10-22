package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStoreLoadDefault(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	store := NewStore(configPath)
	cfg := store.Load()

	if len(cfg.Devices) == 0 {
		t.Fatal("expected default devices to be initialized")
	}
	sel := cfg.GetSelectedDevice()
	if sel == nil {
		t.Fatal("expected an active selected device")
	}
	if !sel.Selected {
		t.Error("expected selected flag to be true")
	}
}

func TestStoreSaveAndReload(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "sub", "config.json")

	store := NewStore(configPath)
	cfg := store.Load()
	cfg.DefaultUser = "testuser"
	cfg.Devices[0].Address = "AA:BB:CC:DD:EE:FF"
	cfg.TargetAddress = "AA:BB:CC:DD:EE:FF"

	if err := store.Save(cfg); err != nil {
		t.Fatalf("failed to save config: %v", err)
	}

	// Verify file was written
	if _, err := os.Stat(configPath); err != nil {
		t.Fatalf("config file was not created: %v", err)
	}

	reloaded := store.Load()
	if reloaded.DefaultUser != "testuser" {
		t.Errorf("got DefaultUser = %q, want %q", reloaded.DefaultUser, "testuser")
	}
	if reloaded.TargetAddress != "AA:BB:CC:DD:EE:FF" {
		t.Errorf("got TargetAddress = %q, want %q", reloaded.TargetAddress, "AA:BB:CC:DD:EE:FF")
	}
}

func TestDeviceManagement(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	store := NewStore(configPath)
	cfg := store.Load()

	// Add device
	newDev := cfg.AddDevice("MacBook Air", "11:22:33:44:55:66", "macos", true)
	if newDev.Platform != "macos" {
		t.Errorf("expected platform macos, got %s", newDev.Platform)
	}
	if !newDev.Selected {
		t.Error("expected new device to be selected")
	}
	if cfg.TargetAddress != "11:22:33:44:55:66" {
		t.Errorf("expected TargetAddress to be updated, got %s", cfg.TargetAddress)
	}

	// Select previous device
	if !cfg.SelectDevice("LP-TZD-0X114650") {
		t.Error("failed to select default device by name")
	}
	sel := cfg.GetSelectedDevice()
	if sel == nil || sel.Name != "LP-TZD-0X114650" {
		t.Errorf("selected device mismatch: %v", sel)
	}

	// Delete device
	deleted, ok := cfg.DeleteDevice(newDev.ID)
	if !ok || deleted == nil {
		t.Fatal("failed to delete added device")
	}
	if deleted.Name != "MacBook Air" {
		t.Errorf("deleted device name = %q, want 'MacBook Air'", deleted.Name)
	}

	// Ensure deleted device is no longer found
	if cfg.SelectDevice(newDev.ID) {
		t.Error("expected deleted device to not be selectable")
	}
}
