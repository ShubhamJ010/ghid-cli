package adb

import (
	"fmt"
	"strings"
	"testing"
)

func TestGetDevices(t *testing.T) {
	fake := NewFakeRunner()
	fake.Register("devices -l", `List of devices attached
988e1039484b39414e     device usb:337641472X product:OnePlus7Pro model:GM1917 device:OnePlus7Pro transport_id:1
192.168.1.100:5555     device product:Pixel6 model:Pixel_6 device:oriole transport_id:2
`, nil)

	mgr := NewManagerWithRunner(fake, "", "")
	devices, err := mgr.GetDevices()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(devices) != 2 {
		t.Fatalf("expected 2 devices, got %d", len(devices))
	}

	if devices[0].Serial != "988e1039484b39414e" || devices[0].Model != "GM1917" {
		t.Errorf("device 0 mismatch: %+v", devices[0])
	}
	if devices[1].Serial != "192.168.1.100:5555" || devices[1].Model != "Pixel_6" {
		t.Errorf("device 1 mismatch: %+v", devices[1])
	}
}

func TestSelectDevice(t *testing.T) {
	fake := NewFakeRunner()
	fake.Register("devices -l", `List of devices attached
988e1039484b39414e     unauthorized usb:337641472X transport_id:1
`, nil)

	mgr := NewManagerWithRunner(fake, "", "")
	_, err := mgr.SelectDevice()
	if err == nil || !strings.Contains(err.Error(), "unauthorized") {
		t.Errorf("expected unauthorized error, got %v", err)
	}

	// Now with an authorized device
	fake.Register("devices -l", `List of devices attached
988e1039484b39414e     device usb:337641472X product:OnePlus7Pro model:GM1917 transport_id:1
`, nil)
	dev, err := mgr.SelectDevice()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dev.Serial != "988e1039484b39414e" {
		t.Errorf("selected device mismatch: %+v", dev)
	}
}

func TestGetStatus(t *testing.T) {
	fake := NewFakeRunner()
	fake.Register("cat /sdcard/Android/data/com.sj010.ghid_headless/files/status.json", `{
		"registered": true,
		"connected": true,
		"state": "CONNECTED",
		"target": "ThinkPad",
		"address": "38:FC:98:E8:E3:87",
		"updated": "2023-10-27T10:00:00Z"
	}`, nil)

	mgr := NewManagerWithRunner(fake, "test-serial", "")
	st := mgr.GetStatus()

	if !st.Registered {
		t.Error("expected registered=true")
	}
	if !st.Connected {
		t.Error("expected connected=true")
	}
	if st.State != "CONNECTED" || st.Target != "ThinkPad" || st.Address != "38:FC:98:E8:E3:87" {
		t.Errorf("status fields mismatch: %+v", st)
	}

	// Malformed JSON fallback
	fakeBroken := NewFakeRunner()
	fakeBroken.Register("cat /sdcard/Android/data/com.sj010.ghid_headless/files/status.json", "file not found", nil)
	mgrBroken := NewManagerWithRunner(fakeBroken, "test-serial", "")
	stBroken := mgrBroken.GetStatus()
	if stBroken.State != "OFFLINE" || stBroken.Connected != false {
		t.Errorf("expected fallback offline status, got %+v", stBroken)
	}
}

func TestSendRawValidation(t *testing.T) {
	fake := NewFakeRunner()
	mgr := NewManagerWithRunner(fake, "test-serial", "")

	// Invalid length
	if err := mgr.SendRaw("12345"); err == nil {
		t.Error("expected error for invalid hex length")
	}

	// Valid length
	if err := mgr.SendRaw("08000f0000000000"); err != nil {
		t.Errorf("unexpected error for valid raw report: %v", err)
	}
}

func TestExecuteLogin(t *testing.T) {
	fake := NewFakeRunner()
	mgr := NewManagerWithRunner(fake, "test-serial", "")

	delays := map[string]float64{
		"wake":    0.001,
		"overlay": 0.001,
		"user":    0.001,
		"tab":     0.001,
		"pass":    0.001,
		"submit":  0.001,
	}

	var steps []int
	err := mgr.ExecuteLogin("Administrator", "SecretPass123", delays, func(step int, desc string) {
		steps = append(steps, step)
	})

	if err != nil {
		t.Fatalf("unexpected error executing login: %v", err)
	}
	if len(steps) != 6 {
		t.Errorf("expected 6 steps called back, got %d (%v)", len(steps), steps)
	}
}

func TestSendSleepSequence(t *testing.T) {
	fake := NewFakeRunner()
	mgr := NewManagerWithRunner(fake, "test-serial", "")

	err := mgr.SendSleepSequence()
	if err != nil {
		t.Fatalf("unexpected error during sleep sequence: %v", err)
	}

	// Verify the calls: esc -> esc -> win+x -> u -> s
	var comboKeys []string
	for _, call := range fake.Calls {
		for i, arg := range call {
			if arg == "combo" && i+2 < len(call) && call[i+1] == "--es" && call[i+2] == "keys" && i+3 < len(call) {
				comboKeys = append(comboKeys, call[i+3])
			}
		}
	}

	expected := []string{"esc", "esc", "win+x", "u", "s"}
	if len(comboKeys) != len(expected) {
		t.Fatalf("got %d combo keys sent (%v), want %d (%v)", len(comboKeys), comboKeys, len(expected), expected)
	}
	for i := range expected {
		if comboKeys[i] != expected[i] {
			t.Errorf("comboKey %d = %q, want %q", i, comboKeys[i], expected[i])
		}
	}
}

func TestSendComboSequence(t *testing.T) {
	fake := NewFakeRunner()
	mgr := NewManagerWithRunner(fake, "test-serial", "")

	// Sequence: esc ->{0.001} super+l
	err := mgr.SendCombo("esc ->{0.001} super+l")
	if err != nil {
		t.Fatalf("unexpected error during combo sequence: %v", err)
	}

	var comboKeys []string
	for _, call := range fake.Calls {
		for i, arg := range call {
			if arg == "combo" && i+2 < len(call) && call[i+1] == "--es" && call[i+2] == "keys" && i+3 < len(call) {
				comboKeys = append(comboKeys, call[i+3])
			}
		}
	}

	// super should normalize to win for Android broadcast
	expected := []string{"esc", "win+l"}
	if len(comboKeys) != len(expected) {
		t.Fatalf("got %d combo keys sent (%v), want %d (%v)", len(comboKeys), comboKeys, len(expected), expected)
	}
	for i := range expected {
		if comboKeys[i] != expected[i] {
			t.Errorf("comboKey %d = %q, want %q", i, comboKeys[i], expected[i])
		}
	}
}

func TestSendComboTypedText(t *testing.T) {
	fake := NewFakeRunner()
	mgr := NewManagerWithRunner(fake, "", "")

	if err := mgr.SendCombo("win+s ->{0.001} type:rustdesk"); err != nil {
		t.Fatalf("SendCombo failed: %v", err)
	}

	var typed string
	for _, call := range fake.Calls {
		for i := 0; i+3 < len(call); i++ {
			if call[i] == "type" && call[i+1] == "--es" && call[i+2] == "text" && call[i+3] == "rustdesk" {
				typed = call[i+3]
			}
		}
	}
	if typed != "rustdesk" {
		t.Fatalf("typed text broadcast not found in %v", fake.Calls)
	}
}

func TestSendComboWithResolver(t *testing.T) {
	fake := NewFakeRunner()
	mgr := NewManagerWithRunner(fake, "", "")

	resolver := func(name string) (string, error) {
		switch strings.ToLower(name) {
		case "username":
			return "alice", nil
		case "password":
			return "Secret99!", nil
		default:
			return "", fmt.Errorf("unknown variable %s", name)
		}
	}

	seq := "ctrl+alt+del ->{0.001} esc ->{0.001} $USERNAME ->{0.001} tab ->{0.001} $PASSWORD ->{0.001} enter"
	if err := mgr.SendComboWithResolver(seq, resolver); err != nil {
		t.Fatalf("SendComboWithResolver failed: %v", err)
	}

	var typed []string
	for _, call := range fake.Calls {
		for i := 0; i+3 < len(call); i++ {
			if call[i] == "type" && call[i+1] == "--es" && call[i+2] == "text" {
				typed = append(typed, call[i+3])
			}
		}
	}

	if len(typed) != 2 || typed[0] != "alice" || typed[1] != "Secret99!" {
		t.Errorf("unexpected typed values: %v", typed)
	}
}

func TestGetDeviceName(t *testing.T) {
	cmd := `for k in "secure bluetooth_name" "global bluetooth_name" "global device_name" "system bluetooth_name"; do v=$(settings get $k 2>/dev/null); if [ -n "$v" ] && [ "$v" != "null" ]; then echo "$v"; exit 0; fi; done; m=$(getprop ro.product.marketname); if [ -n "$m" ]; then echo "$m"; exit 0; fi; getprop ro.product.model`

	// Case 1: Dynamic Bluetooth / device name found
	fake1 := NewFakeRunner()
	fake1.Register("shell "+cmd, "SJ's OnePlus 8T\n", nil)
	mgr1 := NewManagerWithRunner(fake1, "test-serial", "")
	if name := mgr1.GetDeviceName(); name != "SJ's OnePlus 8T" {
		t.Errorf("expected dynamic name, got %q", name)
	}

	// Case 2: Settings return empty/null, fall back to model
	fake2 := NewFakeRunner()
	fake2.Register("shell "+cmd, "", nil)
	fake2.Register("shell getprop ro.product.marketname", "", nil)
	fake2.Register("shell getprop ro.product.model", "KB2001\n", nil)
	mgr2 := NewManagerWithRunner(fake2, "test-serial", "")
	if name := mgr2.GetDeviceName(); name != "KB2001" {
		t.Errorf("expected fallback to model KB2001, got %q", name)
	}

	// Case 3: Completely empty, fall back to Android Device
	fake3 := NewFakeRunner()
	mgr3 := NewManagerWithRunner(fake3, "test-serial", "")
	if name := mgr3.GetDeviceName(); name != "Android Device" {
		t.Errorf("expected default Android Device, got %q", name)
	}
}

func TestGetBondedBluetoothDevices(t *testing.T) {
	sampleOutput := `
  Discovering: false
  DiscoveryEndMs: 1792846141507
  Bonded devices:
    F8:55:CD:5C:2E:C4 [BR/EDR] MAHINDRA4JK231024
    84:0F:2A:4B:6F:ED [ DUAL ] OnePlus Buds Z2
    38:FC:98:E8:E3:87 [BR/EDR] LP-TZD-0X114650
    A0:9A:8E:57:36:DA [ DUAL ] Shubham’s MacBook Air
mSnoopLogSettingAtEnable = empty

AdapterState:
`
	fake := NewFakeRunner()
	fake.Register("shell dumpsys bluetooth_manager", sampleOutput, nil)
	mgr := NewManagerWithRunner(fake, "test-serial", "")

	devices, err := mgr.GetBondedBluetoothDevices()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(devices) != 4 {
		t.Fatalf("expected 4 devices, got %d", len(devices))
	}

	if devices[2].Address != "38:FC:98:E8:E3:87" || devices[2].Name != "LP-TZD-0X114650" || devices[2].Type != "BR/EDR" {
		t.Errorf("device 2 mismatch: %+v", devices[2])
	}
	if devices[3].Address != "A0:9A:8E:57:36:DA" || devices[3].Name != "Shubham’s MacBook Air" || devices[3].Type != "DUAL" {
		t.Errorf("device 3 mismatch: %+v", devices[3])
	}
}
