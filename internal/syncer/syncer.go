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

// Pull pulls combos and non-secure snippets from Android and updates local stores.
// Secure snippets are never shared between CLI and Android app and remain local.
// Non-secure snippets keep their value and are not hidden.
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

	// 2. Pull remote snippets - phone is SSOT for non-secure snippets.
	// Secure snippets are never shared and remain securely stored locally.
	// Non-secure snippets always keep their value when moving between CLI and phone.
	remoteSnippetsRaw, err := mgr.Run("shell", "cat", adb.AppSnippetsFileRemote)
	if err == nil && strings.HasPrefix(strings.TrimSpace(remoteSnippetsRaw), "[") {
		var remoteSnippets []snippets.Snippet
		if err := json.Unmarshal([]byte(strings.TrimSpace(remoteSnippetsRaw)), &remoteSnippets); err == nil {
			localSnippets, _ := sStore.Load()

			// Retain all local secure snippets
			merged := make([]snippets.Snippet, 0, len(localSnippets)+len(remoteSnippets))
			for _, s := range localSnippets {
				if s.Secure {
					merged = append(merged, s)
				}
			}

			// Add or update non-secure snippets from remote, keeping their full values
			for _, r := range remoteSnippets {
				if !r.Secure {
					merged = append(merged, r)
					report.SnippetsPulled++
				}
			}
			_ = sStore.SaveAll(merged)
		}
	}

	return report, nil
}

// Push pushes local combos and non-secure snippets to Android,
// restarts the app and broadcasts reconnect to target device.
// Secure snippets are never saved by the CLI to the application.
// Non-secure snippets always keep their value when moving to the phone.
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

	// 2. Push snippets: secure snippets are NEVER saved by CLI to the application.
	// Only non-secure snippets are saved to the application, preserving their values.
	localSnippets, err := sStore.Load()
	if err != nil {
		return nil, fmt.Errorf("load snippets: %w", err)
	}
	toPush := make([]snippets.Snippet, 0, len(localSnippets))
	for _, s := range localSnippets {
		if !s.Secure {
			toPush = append(toPush, s)
		}
	}
	snippetsData, err := json.MarshalIndent(toPush, "", "  ")
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
	report.SnippetsPushed = len(toPush)

	// 3. Restart app and reconnect Bluetooth
	if err := mgr.RestartAppAndReconnect(); err != nil {
		return report, fmt.Errorf("restart and reconnect: %w", err)
	}
	report.AppRestarted = true
	report.Reconnected = true

	return report, nil
}

// Sync performs a full two-way synchronization:
// 1. Pulls from Android (non-secure snippets and combos).
// 2. Pushes back to Android (excluding secure snippets).
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

// AutoPushIfConnected pushes combos & non-secure snippets if a device is connected via ADB.
// Silent no-op if no device is connected.
func AutoPushIfConnected(mgr *adb.Manager, cStore *combos.Store, sStore *snippets.Store) error {
	if mgr == nil || !mgr.IsAvailable() {
		return nil
	}
	_, err := Push(mgr, cStore, sStore)
	return err
}

// AutoPullIfConnected pulls combos and non-secure snippets if a device is connected.
// Returns true if sync succeeded.
func AutoPullIfConnected(mgr *adb.Manager, cStore *combos.Store, sStore *snippets.Store) bool {
	if mgr == nil || !mgr.IsAvailable() {
		return false
	}
	rep, err := Pull(mgr, cStore, sStore)
	return err == nil && (rep.CombosPulled > 0 || rep.SnippetsPulled > 0)
}
