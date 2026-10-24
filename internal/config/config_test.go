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

	if len(cfg.Devices) != 0 {
		t.Fatalf("expected 0 default devices, got %d", len(cfg.Devices))
	}
	sel := cfg.GetSelectedDevice()
	if sel != nil {
		t.Fatalf("expected no active selected device when empty, got %+v", sel)
	}
}

func TestStoreSaveAndReload(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "sub", "config.json")

	store := NewStore(configPath)
	cfg := store.Load()
	cfg.DefaultUser = "testuser"
	cfg.AddDevice("TestPC", "AA:BB:CC:DD:EE:FF", "windows", true)

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
	if len(reloaded.Devices) != 1 {
		t.Errorf("expected 1 device, got %d", len(reloaded.Devices))
	}
}

func TestDeviceManagement(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	store := NewStore(configPath)
	cfg := store.Load()

	// Add first device
	dev1 := cfg.AddDevice("PC1", "11:22:33:44:55:66", "windows", true)

	// Add second device
	newDev := cfg.AddDevice("MacBook Air", "AA:BB:CC:DD:EE:FF", "macos", true)
	if newDev.Platform != "macos" {
		t.Errorf("expected platform macos, got %s", newDev.Platform)
	}
	if !newDev.Selected {
		t.Error("expected new device to be selected")
	}
	if cfg.TargetAddress != "AA:BB:CC:DD:EE:FF" {
		t.Errorf("expected TargetAddress to be updated, got %s", cfg.TargetAddress)
	}

	// Select previous device
	if !cfg.SelectDevice(dev1.ID) {
		t.Error("failed to select device 1 by ID")
	}
	sel := cfg.GetSelectedDevice()
	if sel == nil || sel.Name != "PC1" {
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

	// Delete last remaining device
	deleted2, ok2 := cfg.DeleteDevice(dev1.ID)
	if !ok2 || deleted2 == nil {
		t.Fatal("failed to delete dev1")
	}
	if len(cfg.Devices) != 0 {
		t.Errorf("expected 0 devices after deleting all, got %d", len(cfg.Devices))
	}
	if cfg.TargetAddress != "" {
		t.Errorf("expected empty TargetAddress after deleting all, got %s", cfg.TargetAddress)
	}

	// Reload from store to verify normalizeDevices does NOT resurrect deleted devices
	_ = store.Save(cfg)
	reloaded := store.Load()
	if len(reloaded.Devices) != 0 {
		t.Errorf("expected 0 devices on reload, got %d", len(reloaded.Devices))
	}
}
