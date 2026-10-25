package syncer

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/ghost-hid/ghid/internal/adb"
	"github.com/ghost-hid/ghid/internal/combos"
	"github.com/ghost-hid/ghid/internal/payloads"
)

// SyncReport summarizes the results of a sync operation.
type SyncReport struct {
	CombosPulled   int  `json:"combos_pulled"`
	CombosPushed   int  `json:"combos_pushed"`
	PayloadsPulled int  `json:"payloads_pulled"`
	PayloadsPushed int  `json:"payloads_pushed"`
	AppRestarted   bool `json:"app_restarted"`
	Reconnected    bool `json:"reconnected"`
}

// Pull pulls combos and non-secure payloads from Android and updates local stores.
// Secure payloads are never shared between CLI and Android app and remain local.
// Non-secure payloads keep their value and are not hidden.
func Pull(mgr *adb.Manager, cStore *combos.Store, pStore *payloads.Store) (*SyncReport, error) {
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

	// 2. Pull remote payloads - phone is SSOT for non-secure payloads.
	// Secure payloads are never shared and remain securely stored locally.
	// Non-secure payloads always keep their value when moving between CLI and phone.
	remotePayloadsRaw, err := mgr.Run("shell", "cat", adb.AppPayloadsFileRemote)
	if err == nil && strings.HasPrefix(strings.TrimSpace(remotePayloadsRaw), "[") {
		var remotePayloads []payloads.Payload
		if err := json.Unmarshal([]byte(strings.TrimSpace(remotePayloadsRaw)), &remotePayloads); err == nil {
			localPayloads, _ := pStore.Load()

			// Retain all local secure payloads
			merged := make([]payloads.Payload, 0, len(localPayloads)+len(remotePayloads))
			for _, p := range localPayloads {
				if p.Secure {
					merged = append(merged, p)
				}
			}

			// Add or update non-secure payloads from remote, keeping their full values
			for _, r := range remotePayloads {
				if !r.Secure {
					merged = append(merged, r)
					report.PayloadsPulled++
				}
			}
			_ = pStore.SaveAll(merged)
		}
	}

	return report, nil
}

// Push pushes local combos and non-secure payloads to Android,
// restarts the app and broadcasts reconnect to target device.
// Secure payloads are never saved by the CLI to the application.
// Non-secure payloads always keep their value when moving to the phone.
func Push(mgr *adb.Manager, cStore *combos.Store, pStore *payloads.Store) (*SyncReport, error) {
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

	// 2. Push payloads: secure payloads are NEVER saved by CLI to the application.
	// Only non-secure payloads are saved to the application, preserving their values.
	localPayloads, err := pStore.Load()
	if err != nil {
		return nil, fmt.Errorf("load payloads: %w", err)
	}
	toPush := make([]payloads.Payload, 0, len(localPayloads))
	for _, p := range localPayloads {
		if !p.Secure {
			toPush = append(toPush, p)
		}
	}
	payloadsData, err := json.MarshalIndent(toPush, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal payloads: %w", err)
	}
	tmpPayloads, err := os.CreateTemp("", "ghid_payloads_*.json")
	if err != nil {
		return nil, err
	}
	defer os.Remove(tmpPayloads.Name())
	if _, err := tmpPayloads.Write(payloadsData); err != nil {
		return nil, err
	}
	_ = tmpPayloads.Close()

	if _, err := mgr.Run("push", tmpPayloads.Name(), adb.AppPayloadsFileRemote); err != nil {
		return nil, fmt.Errorf("push payloads to android: %w", err)
	}
	report.PayloadsPushed = len(toPush)

	// 3. Restart app and reconnect Bluetooth
	if err := mgr.RestartAppAndReconnect(); err != nil {
		return report, fmt.Errorf("restart and reconnect: %w", err)
	}
	report.AppRestarted = true
	report.Reconnected = true

	return report, nil
}

// Sync performs a full two-way synchronization:
// 1. Pulls from Android (non-secure payloads and combos).
// 2. Pushes back to Android (excluding secure payloads).
// 3. Restarts app and reconnects Bluetooth.
func Sync(mgr *adb.Manager, cStore *combos.Store, pStore *payloads.Store) (*SyncReport, error) {
	pullRep, err := Pull(mgr, cStore, pStore)
	if err != nil {
		return nil, err
	}
	pushRep, err := Push(mgr, cStore, pStore)
	if err != nil {
		return nil, err
	}
	return &SyncReport{
		CombosPulled:   pullRep.CombosPulled,
		CombosPushed:   pushRep.CombosPushed,
		PayloadsPulled: pullRep.PayloadsPulled,
		PayloadsPushed: pushRep.PayloadsPushed,
		AppRestarted:   pushRep.AppRestarted,
		Reconnected:    pushRep.Reconnected,
	}, nil
}

// AutoPushIfConnected pushes combos & non-secure payloads if a device is connected via ADB.
// Silent no-op if no device is connected.
func AutoPushIfConnected(mgr *adb.Manager, cStore *combos.Store, pStore *payloads.Store) error {
	if mgr == nil || !mgr.IsAvailable() {
		return nil
	}
	_, err := Push(mgr, cStore, pStore)
	return err
}

// AutoPullIfConnected pulls combos and non-secure payloads if a device is connected.
// Returns true if sync succeeded.
func AutoPullIfConnected(mgr *adb.Manager, cStore *combos.Store, pStore *payloads.Store) bool {
	if mgr == nil || !mgr.IsAvailable() {
		return false
	}
	rep, err := Pull(mgr, cStore, pStore)
	return err == nil && (rep.CombosPulled > 0 || rep.PayloadsPulled > 0)
}
