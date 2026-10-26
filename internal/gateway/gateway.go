package gateway

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ShubhamJ010/ghid-cli/internal/adb"
	"github.com/ShubhamJ010/ghid-cli/internal/config"
	"github.com/ShubhamJ010/ghid-cli/internal/hid"
	"github.com/ShubhamJ010/ghid-cli/internal/vault"
)

// LoginRequest holds parameters for the multi-step automated unlock sequence.
type LoginRequest struct {
	Username string
	Password string
	Delays   config.Delays
}

// DoctorCheck represents an individual component health check.
type DoctorCheck struct {
	Name   string `json:"name"`
	Pass   bool   `json:"pass"`
	Detail string `json:"detail"`
}

// DoctorReport contains the full system diagnostic status.
type DoctorReport struct {
	Version           string               `json:"version"`
	AdbPath           string               `json:"adb_path"`
	AdbAvailable      bool                 `json:"adb_available"`
	DeviceConnected   bool                 `json:"device_connected"`
	DeviceModel       string               `json:"device_model"`
	DeviceName        string               `json:"device_name,omitempty"`
	DeviceSerial      string               `json:"device_serial"`
	ApkInstalled      bool                 `json:"apk_installed"`
	BluetoothOn       bool                 `json:"bluetooth_on"`
	GatewayRegistered bool                 `json:"gateway_registered"`
	GatewayConnected  bool                 `json:"gateway_connected"`
	TargetLaptop      string               `json:"target_laptop"`
	TargetAddress     string               `json:"target_address"`
	SelectedDevice    *config.TargetDevice `json:"selected_device,omitempty"`
	RegisteredDevices int                  `json:"registered_devices"`
	KeychainAvailable bool                 `json:"keychain_available"`
	CachedUser        string               `json:"cached_user"`
	HasCachedPassword bool                 `json:"has_cached_password"`
	Checks            []DoctorCheck        `json:"checks,omitempty"`
}

// Controller defines the deep module interface for all GhostHID gateway operations.
type Controller interface {
	Status() (adb.GatewayStatus, error)
	Doctor(version string, cfg config.Config, vlt vault.Vault) (DoctorReport, error)
	EnsureDevice() (*adb.Device, error)
	Connect(targetAddr string) error
	Disconnect() error
	Unpair(targetAddr string) error
	StartService() error
	StopService() error
	RestartService() error
	Lock() error
	SleepSequence() error
	Type(text string) error
	Combo(keys string) error
	ComboWithResolver(keys string, resolver hid.PayloadResolver) error
	Click(button string, count int) error
	Mouse(action string) error
	Raw(hexReport string) error
	RawWithHold(hexReport string, hold time.Duration) error
	Login(req LoginRequest, progress func(step int, desc string)) error
	GetLogs(lines int) (string, error)
	GetFileLog() (string, error)
	BuildApk() error
	InstallApk(path string) error
	GetBondedDevices() ([]adb.BondedDevice, error)
	Manager() *adb.Manager
}

// Gateway implements Controller using an underlying adb.Manager.
type Gateway struct {
	mgr *adb.Manager
}

// New creates a new Gateway controller using the provided adb.Manager.
func New(mgr *adb.Manager) *Gateway {
	return &Gateway{mgr: mgr}
}

func (g *Gateway) Manager() *adb.Manager {
	return g.mgr
}

func (g *Gateway) Status() (adb.GatewayStatus, error) {
	if g.mgr == nil {
		return adb.GatewayStatus{}, errors.New("adb manager not initialized")
	}
	return g.mgr.GetStatus(), nil
}

func (g *Gateway) EnsureDevice() (*adb.Device, error) {
	if g.mgr == nil {
		return nil, errors.New("adb manager not initialized")
	}
	return g.mgr.SelectDevice()
}

func (g *Gateway) Connect(targetAddr string) error {
	if g.mgr == nil {
		return errors.New("adb manager not initialized")
	}
	return g.mgr.ConnectTarget(targetAddr)
}

func (g *Gateway) Disconnect() error {
	if g.mgr == nil {
		return errors.New("adb manager not initialized")
	}
	return g.mgr.DisconnectTarget()
}

func (g *Gateway) Unpair(targetAddr string) error {
	if g.mgr == nil {
		return errors.New("adb manager not initialized")
	}
	return g.mgr.UnpairTarget()
}

func (g *Gateway) StartService() error {
	if g.mgr == nil {
		return errors.New("adb manager not initialized")
	}
	return g.mgr.StartGateway()
}

func (g *Gateway) StopService() error {
	if g.mgr == nil {
		return errors.New("adb manager not initialized")
	}
	return g.mgr.StopGateway()
}

func (g *Gateway) RestartService() error {
	if g.mgr == nil {
		return errors.New("adb manager not initialized")
	}
	_ = g.mgr.StopGateway()
	time.Sleep(500 * time.Millisecond)
	return g.mgr.StartGateway()
}

func (g *Gateway) Lock() error {
	if g.mgr == nil {
		return errors.New("adb manager not initialized")
	}
	return g.mgr.SendLock()
}

func (g *Gateway) SleepSequence() error {
	if g.mgr == nil {
		return errors.New("adb manager not initialized")
	}
	return g.mgr.SendSleepSequence()
}

func (g *Gateway) Type(text string) error {
	if g.mgr == nil {
		return errors.New("adb manager not initialized")
	}
	return g.mgr.TypeText(text)
}

func (g *Gateway) Combo(keys string) error {
	return g.ComboWithResolver(keys, nil)
}

func (g *Gateway) ComboWithResolver(keys string, resolver hid.PayloadResolver) error {
	if g.mgr == nil {
		return errors.New("adb manager not initialized")
	}
	// Validate sequence using the domain HID parser
	if _, err := hid.ParseSequence(keys); err != nil {
		return fmt.Errorf("invalid combo sequence: %w", err)
	}
	return g.mgr.SendComboWithResolver(keys, resolver)
}

func (g *Gateway) Click(button string, count int) error {
	if g.mgr == nil {
		return errors.New("adb manager not initialized")
	}
	return g.mgr.SendClick(button, count)
}

func (g *Gateway) Mouse(action string) error {
	if g.mgr == nil {
		return errors.New("adb manager not initialized")
	}
	return g.mgr.SendMouse(action)
}

func (g *Gateway) Raw(hexReport string) error {
	return g.RawWithHold(hexReport, 0)
}

func (g *Gateway) RawWithHold(hexReport string, hold time.Duration) error {
	if g.mgr == nil {
		return errors.New("adb manager not initialized")
	}
	if _, err := hid.ValidateRaw(hexReport); err != nil {
		return err
	}
	return g.mgr.SendRawWithHold(hexReport, hold)
}

func (g *Gateway) Login(req LoginRequest, progress func(step int, desc string)) error {
	if g.mgr == nil {
		return errors.New("adb manager not initialized")
	}
	if req.Username == "" || req.Password == "" {
		return errors.New("username and password cannot be empty")
	}

	delays := map[string]float64{
		"wake":    req.Delays.Wake,
		"overlay": req.Delays.Overlay,
		"user":    req.Delays.User,
		"tab":     req.Delays.Tab,
		"pass":    req.Delays.Pass,
		"submit":  req.Delays.Submit,
	}

	return g.mgr.ExecuteLogin(req.Username, req.Password, delays, progress)
}

func (g *Gateway) GetLogs(lines int) (string, error) {
	if g.mgr == nil {
		return "", errors.New("adb manager not initialized")
	}
	return g.mgr.GetLogs(lines)
}

func (g *Gateway) GetFileLog() (string, error) {
	if g.mgr == nil {
		return "", errors.New("adb manager not initialized")
	}
	return g.mgr.GetFileLog()
}

func (g *Gateway) BuildApk() error {
	if g.mgr == nil {
		return errors.New("adb manager not initialized")
	}
	return g.mgr.BuildApk()
}

func (g *Gateway) InstallApk(path string) error {
	if g.mgr == nil {
		return errors.New("adb manager not initialized")
	}
	return g.mgr.InstallApk(path)
}

func (g *Gateway) GetBondedDevices() ([]adb.BondedDevice, error) {
	if g.mgr == nil {
		return nil, errors.New("adb manager not initialized")
	}
	if _, err := g.EnsureDevice(); err != nil {
		return nil, err
	}
	return g.mgr.GetBondedBluetoothDevices()
}

func (g *Gateway) Doctor(version string, cfg config.Config, vlt vault.Vault) (DoctorReport, error) {
	if g.mgr == nil {
		return DoctorReport{}, errors.New("adb manager not initialized")
	}

	dev, devErr := g.mgr.SelectDevice()
	pkgInstalled := g.mgr.IsPackageInstalled()
	btOn := g.mgr.GetBluetoothState()
	status := g.mgr.GetStatus()
	selDev := cfg.GetSelectedDevice()

	keychainAvail := false
	hasCachedPass := false
	if vlt != nil {
		keychainAvail = vlt.IsAvailable()
		if cfg.DefaultUser != "" && keychainAvail {
			if _, err := vlt.Get(cfg.DefaultUser); err == nil {
				hasCachedPass = true
			}
		}
	}

	targetDesc := "None"
	if selDev != nil {
		targetDesc = fmt.Sprintf("%s [%s] (%s)", selDev.Name, selDev.Address, selDev.Platform)
	}

	userDesc := "None"
	if cfg.DefaultUser != "" {
		userDesc = cfg.DefaultUser
	}

	phoneName := g.mgr.GetDeviceName()
	if cfg.PhoneName != "" {
		phoneName = cfg.PhoneName
	}

	checks := []DoctorCheck{
		{Name: "ADB Executable", Pass: true, Detail: g.mgr.AdbPath},
		{Name: "Android Device", Pass: devErr == nil, Detail: fmt.Sprintf("%s [%s]", phoneName, g.mgr.Serial)},
		{Name: "GhostHID APK", Pass: pkgInstalled, Detail: adb.PackageName},
		{Name: "Phone Bluetooth State", Pass: btOn, Detail: "Enabled"},
		{Name: "Bluetooth HID Profile (SDP)", Pass: status.Registered, Detail: "Registered"},
		{Name: "Target Laptop Connection", Pass: status.Connected, Detail: fmt.Sprintf("%s [%s]", status.Target, status.Address)},
		{Name: "Configured Active Target", Pass: selDev != nil, Detail: targetDesc},
		{Name: "macOS Keychain Vault", Pass: keychainAvail, Detail: "Service 'ghost-hid'"},
		{Name: "Cached Windows Credentials", Pass: hasCachedPass, Detail: userDesc},
	}

	return DoctorReport{
		Version:           version,
		AdbPath:           g.mgr.AdbPath,
		AdbAvailable:      true,
		DeviceConnected:   dev != nil && devErr == nil,
		DeviceModel:       g.mgr.GetDeviceModel(),
		DeviceName:        phoneName,
		DeviceSerial:      g.mgr.Serial,
		ApkInstalled:      pkgInstalled,
		BluetoothOn:       btOn,
		GatewayRegistered: status.Registered,
		GatewayConnected:  status.Connected,
		TargetLaptop:      status.Target,
		TargetAddress:     status.Address,
		SelectedDevice:    selDev,
		RegisteredDevices: len(cfg.Devices),
		KeychainAvailable: keychainAvail,
		CachedUser:        strings.TrimSpace(cfg.DefaultUser),
		HasCachedPassword: hasCachedPass,
		Checks:            checks,
	}, nil
}
