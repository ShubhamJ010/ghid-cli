package syncer

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/ShubhamJ010/ghid-cli/internal/adb"
	"github.com/ShubhamJ010/ghid-cli/internal/combos"
	"github.com/ShubhamJ010/ghid-cli/internal/payloads"
)

// SyncReport summarizes the results of a sync operation.
type SyncReport struct {
	CombosPulled          int  `json:"combos_pulled"`
	CombosPushed          int  `json:"combos_pushed"`
	PayloadsPulled        int  `json:"payloads_pulled"`
	PayloadsPushed        int  `json:"payloads_pushed"`
	PayloadsSkippedSecure int  `json:"payloads_skipped_secure"`
	AppRestarted          bool `json:"app_restarted"`
	Reconnected           bool `json:"reconnected"`
}

// Pull merges combos and non-secure payloads from Android into the local stores.
// The phone wins name conflicts (Android is SSOT), but local-only entries are
// preserved: a CLI edit made while the phone was away must not disappear on the
// next startup pull. Secure payload values never cross: remote entries arrive
// Value-scrubbed and never overwrite the local Keychain-backed originals.
func Pull(mgr *adb.Manager, cStore *combos.Store, pStore *payloads.Store) (*SyncReport, error) {
	report := &SyncReport{}

	// 1. Pull remote combos - phone wins conflicts, local-only entries survive.
	remoteCombosRaw, err := mgr.Run("shell", "cat", adb.AppCombosFileRemote)
	if err == nil && strings.HasPrefix(strings.TrimSpace(remoteCombosRaw), "[") {
		var remoteCombos []combos.Combo
		if err := json.Unmarshal([]byte(strings.TrimSpace(remoteCombosRaw)), &remoteCombos); err == nil {
			report.CombosPulled = len(remoteCombos)
			localCombos, _ := cStore.Load()
			_ = cStore.SaveAll(mergeCombos(localCombos, remoteCombos))
		}
	}

	// 2. Pull remote payloads - phone wins conflicts for non-secure payloads.
	// Remote secure entries arrive Value-scrubbed (ADR-009) and are ignored:
	// the local Keychain-backed originals stay untouched, and local-only
	// non-secure payloads are preserved for the next push.
	remotePayloadsRaw, err := mgr.Run("shell", "cat", adb.AppPayloadsFileRemote)
	if err == nil && strings.HasPrefix(strings.TrimSpace(remotePayloadsRaw), "[") {
		var remotePayloads []payloads.Payload
		if err := json.Unmarshal([]byte(strings.TrimSpace(remotePayloadsRaw)), &remotePayloads); err == nil {
			localPayloads, _ := pStore.Load()
			_ = pStore.SaveAll(mergePayloads(localPayloads, remotePayloads))
			for _, p := range remotePayloads {
				if !p.Secure {
					report.PayloadsPulled++
				}
			}
		}
	}

	return report, nil
}

// mergeCombos returns remote combos in order, then local-only combos. Names are
// compared case-insensitively, matching the rest of the combo store.
func mergeCombos(local, remote []combos.Combo) []combos.Combo {
	seen := make(map[string]bool, len(remote))
	merged := make([]combos.Combo, 0, len(local)+len(remote))
	for _, c := range remote {
		merged = append(merged, c)
		seen[strings.ToLower(strings.TrimSpace(c.Name))] = true
	}
	for _, c := range local {
		if !seen[strings.ToLower(strings.TrimSpace(c.Name))] {
			merged = append(merged, c)
		}
	}
	return merged
}

// mergePayloads keeps secure payloads local, takes remote non-secure payloads,
// and preserves non-secure payloads that only exist locally.
func mergePayloads(local, remote []payloads.Payload) []payloads.Payload {
	seen := make(map[string]bool, len(remote))
	merged := make([]payloads.Payload, 0, len(local)+len(remote))
	for _, p := range local {
		if p.Secure {
			merged = append(merged, p)
		}
	}
	for _, p := range remote {
		if p.Secure {
			continue
		}
		merged = append(merged, p)
		seen[strings.ToLower(strings.TrimSpace(p.Name))] = true
	}
	for _, p := range local {
		if !p.Secure && !seen[strings.ToLower(strings.TrimSpace(p.Name))] {
			merged = append(merged, p)
		}
	}
	return merged
}

// Push pushes local combos and all payloads to Android, restarts the app and
// broadcasts reconnect to the target device. Secure payloads are pushed with
// Value scrubbed per ADR-009: the entry (name, type, secure flag) lands on the
// phone so referencing combos resolve, but the secret stays in the Keychain
// and never crosses the transport. Non-secure payloads keep their full value.
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

	// 2. Push payloads: every payload travels so phone-side combos resolve,
	// but secure payloads are scrubbed first (Value = "", ADR-009). The secret
	// never leaves the macOS Keychain; PayloadsSkippedSecure counts the scrub.
	localPayloads, err := pStore.Load()
	if err != nil {
		return nil, fmt.Errorf("load payloads: %w", err)
	}
	toPush := make([]payloads.Payload, 0, len(localPayloads))
	for _, p := range localPayloads {
		if p.Secure {
			p.Value = ""
			report.PayloadsSkippedSecure++
		}
		toPush = append(toPush, p)
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
// 1. Pulls from Android (combos and non-secure payload values).
// 2. Pushes back to Android (secure payloads included with Value scrubbed).
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
		CombosPulled:          pullRep.CombosPulled,
		CombosPushed:          pushRep.CombosPushed,
		PayloadsPulled:        pullRep.PayloadsPulled,
		PayloadsPushed:        pushRep.PayloadsPushed,
		PayloadsSkippedSecure: pushRep.PayloadsSkippedSecure,
		AppRestarted:          pushRep.AppRestarted,
		Reconnected:           pushRep.Reconnected,
	}, nil
}

// PushIfConnected pushes local state only when an Android device is actually
// attached. The boolean reports whether a push was attempted, so callers can
// print the truth instead of assuming a sync happened.
func PushIfConnected(mgr *adb.Manager, cStore *combos.Store, pStore *payloads.Store) (*SyncReport, bool, error) {
	if mgr == nil {
		return nil, false, nil
	}
	if _, err := mgr.SelectDevice(); err != nil {
		return nil, false, nil
	}
	report, err := Push(mgr, cStore, pStore)
	return report, true, err
}

// AutoPushIfConnected pushes combos & non-secure payloads if a device is
// connected via ADB. Silent no-op if no device is connected.
func AutoPushIfConnected(mgr *adb.Manager, cStore *combos.Store, pStore *payloads.Store) error {
	_, _, err := PushIfConnected(mgr, cStore, pStore)
	return err
}

// AutoPullIfConnected pulls combos and non-secure payloads if a device is
// connected. Discovery happens here too: a manager with no serial still finds
// a freshly attached phone instead of silently skipping the pull.
func AutoPullIfConnected(mgr *adb.Manager, cStore *combos.Store, pStore *payloads.Store) bool {
	if mgr == nil {
		return false
	}
	if _, err := mgr.SelectDevice(); err != nil {
		return false
	}
	rep, err := Pull(mgr, cStore, pStore)
	return err == nil && (rep.CombosPulled > 0 || rep.PayloadsPulled > 0)
}
