package syncer

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/ghost-hid/ghid/internal/adb"
	"github.com/ghost-hid/ghid/internal/combos"
	"github.com/ghost-hid/ghid/internal/snippets"
)

// SyncReport summarizes the results of a sync operation.
type SyncReport struct {
	CombosPulled   int  `json:"combos_pulled"`
	CombosPushed   int  `json:"combos_pushed"`
	SnippetsPulled int  `json:"snippets_pulled"`
	SnippetsPushed int  `json:"snippets_pushed"`
	AppRestarted   bool `json:"app_restarted"`
	Reconnected    bool `json:"reconnected"`
}

// Pull pulls combos and snippets from Android and replaces local stores.
// Android is the single source of truth (SSOT).
func Pull(mgr *adb.Manager, cStore *combos.Store, sStore *snippets.Store) (*SyncReport, error) {
	report := &SyncReport{}

	// 1. Pull remote combos - phone is SSOT
	remoteCombosRaw, err := mgr.Run("shell", "cat", adb.AppCombosFileRemote)
	if err == nil && strings.HasPrefix(strings.TrimSpace(remoteCombosRaw), "[") {
		var remoteCombos []combos.Combo
		if err := json.Unmarshal([]byte(strings.TrimSpace(remoteCombosRaw)), &remoteCombos); err == nil {
			report.CombosPulled = len(remoteCombos)
			_ = cStore.SaveAll(remoteCombos)
		}
	}

	// 2. Pull remote snippets - phone is SSOT for entry list & plaintext values
	remoteSnippetsRaw, err := mgr.Run("shell", "cat", adb.AppSnippetsFileRemote)
	if err == nil && strings.HasPrefix(strings.TrimSpace(remoteSnippetsRaw), "[") {
		var remoteSnippets []snippets.Snippet
		if err := json.Unmarshal([]byte(strings.TrimSpace(remoteSnippetsRaw)), &remoteSnippets); err == nil {
			report.SnippetsPulled = len(remoteSnippets)
			localSnippets, _ := sStore.Load()
			// Map local secrets to preserve Keychain passwords
			localMap := make(map[string]string)
			for _, s := range localSnippets {
				if s.Secure && s.Value != "" {
					localMap[strings.ToLower(s.Name)] = s.Value
				}
			}

			for i := range remoteSnippets {
				nameKey := strings.ToLower(remoteSnippets[i].Name)
				if remoteSnippets[i].Secure {
					if secretVal, exists := localMap[nameKey]; exists {
						remoteSnippets[i].Value = secretVal
					}
				}
			}
			_ = sStore.SaveAll(remoteSnippets)
		}
	}

	return report, nil
}

// Push pushes local combos and sanitized snippets (secrets stripped) to Android,
// restarts the app and broadcasts reconnect to target device.
func Push(mgr *adb.Manager, cStore *combos.Store, sStore *snippets.Store) (*SyncReport, error) {
	report := &SyncReport{}

	// 1. Push combos
	localCombos, err := cStore.Load()
	if err != nil {
		return nil, fmt.Errorf("load combos: %w", err)
	}
	combosData, err := json.MarshalIndent(localCombos, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal combos: %w", err)
	}
	tmpCombos, err := os.CreateTemp("", "ghid_combos_*.json")
	if err != nil {
		return nil, err
	}
	defer os.Remove(tmpCombos.Name())
	if _, err := tmpCombos.Write(combosData); err != nil {
		return nil, err
	}
	_ = tmpCombos.Close()

	if _, err := mgr.Run("push", tmpCombos.Name(), adb.AppCombosFileRemote); err != nil {
		return nil, fmt.Errorf("push combos to android: %w", err)
	}
	report.CombosPushed = len(localCombos)

	// 2. Push snippets (strip secrets)
	localSnippets, err := sStore.Load()
	if err != nil {
		return nil, fmt.Errorf("load snippets: %w", err)
	}
	sanitized := make([]snippets.Snippet, len(localSnippets))
	for i, s := range localSnippets {
		sanitized[i] = s
		if s.Secure {
			sanitized[i].Value = ""
		}
	}
	snippetsData, err := json.MarshalIndent(sanitized, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal snippets: %w", err)
	}
	tmpSnippets, err := os.CreateTemp("", "ghid_snippets_*.json")
	if err != nil {
		return nil, err
	}
	defer os.Remove(tmpSnippets.Name())
	if _, err := tmpSnippets.Write(snippetsData); err != nil {
		return nil, err
	}
	_ = tmpSnippets.Close()

	if _, err := mgr.Run("push", tmpSnippets.Name(), adb.AppSnippetsFileRemote); err != nil {
		return nil, fmt.Errorf("push snippets to android: %w", err)
	}
	report.SnippetsPushed = len(sanitized)

	// 3. Restart app and reconnect Bluetooth
	if err := mgr.RestartAppAndReconnect(); err != nil {
		return report, fmt.Errorf("restart and reconnect: %w", err)
	}
	report.AppRestarted = true
	report.Reconnected = true

	return report, nil
}

// Sync performs a full two-way synchronization:
// 1. Pulls from Android (Android is single source of truth).
// 2. Pushes back to Android (sanitized secrets).
// 3. Restarts app and reconnects Bluetooth.
func Sync(mgr *adb.Manager, cStore *combos.Store, sStore *snippets.Store) (*SyncReport, error) {
	pullRep, err := Pull(mgr, cStore, sStore)
	if err != nil {
		return nil, err
	}
	pushRep, err := Push(mgr, cStore, sStore)
	if err != nil {
		return nil, err
	}
	return &SyncReport{
		CombosPulled:   pullRep.CombosPulled,
		CombosPushed:   pushRep.CombosPushed,
		SnippetsPulled: pullRep.SnippetsPulled,
		SnippetsPushed: pushRep.SnippetsPushed,
		AppRestarted:   pushRep.AppRestarted,
		Reconnected:    pushRep.Reconnected,
	}, nil
}

// AutoPullIfConnected quietly pulls from Android on CLI startup if a device is connected.
// Returns true if sync occurred, false if no device or error.
func AutoPullIfConnected(mgr *adb.Manager, cStore *combos.Store, sStore *snippets.Store) bool {
	if mgr == nil || cStore == nil || sStore == nil {
		return false
	}
	devices, err := mgr.GetDevices()
	if err != nil || len(devices) == 0 {
		return false
	}
	hasDevice := false
	for _, d := range devices {
		if d.State == "device" {
			hasDevice = true
			break
		}
	}
	if !hasDevice {
		return false
	}

	_, err = Pull(mgr, cStore, sStore)
	return err == nil
}

// AutoPushIfConnected pushes local changes back to Android and restarts app if device is connected.
func AutoPushIfConnected(mgr *adb.Manager, cStore *combos.Store, sStore *snippets.Store) bool {
	if mgr == nil || cStore == nil || sStore == nil {
		return false
	}
	devices, err := mgr.GetDevices()
	if err != nil || len(devices) == 0 {
		return false
	}
	for _, d := range devices {
		if d.State == "device" {
			_, pushErr := Push(mgr, cStore, sStore)
			return pushErr == nil
		}
	}
	return false
}
