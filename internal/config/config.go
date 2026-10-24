package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Delays struct {
	Wake    float64 `json:"wake"`
	Overlay float64 `json:"overlay"`
	User    float64 `json:"user"`
	Tab     float64 `json:"tab"`
	Pass    float64 `json:"pass"`
	Submit  float64 `json:"submit"`
}

type TargetDevice struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Address  string `json:"address"`
	Platform string `json:"platform"` // "windows" or "macos"
	Selected bool   `json:"selected"`
}

type Config struct {
	PhoneName          string         `json:"phone_name,omitempty"`
	DefaultUser        string         `json:"default_user"`
	TargetAddress      string         `json:"target_address"`
	WirelessADBAddress string         `json:"wireless_adb_address"`
	DeviceSerial       string         `json:"device_serial,omitempty"`
	Delays             Delays         `json:"delays"`
	Devices            []TargetDevice `json:"devices"`
	storePath          string         `json:"-"`
}

func DefaultConfig() Config {
	return Config{
		PhoneName:          "",
		DefaultUser:        "",
		TargetAddress:      "",
		WirelessADBAddress: "",
		Delays: Delays{
			Wake:    1.0,
			Overlay: 1.0,
			User:    1.2,
			Tab:     0.8,
			Pass:    1.2,
			Submit:  1.0,
		},
		Devices: []TargetDevice{},
	}
}

// Store encapsulates persistent configuration loading and saving.
type Store struct {
	Path string
}

func NewStore(path string) *Store {
	if path == "" {
		path = GetConfigPath()
	}
	return &Store{Path: path}
}

func GetConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".config/ghid/config.json"
	}
	return filepath.Join(home, ".config", "ghid", "config.json")
}

func normalizeDevices(cfg *Config) {
	if cfg.Devices == nil {
		cfg.Devices = []TargetDevice{}
	}
	if len(cfg.Devices) == 0 {
		cfg.TargetAddress = ""
		return
	}
	hasSelected := false
	for i := range cfg.Devices {
		if cfg.Devices[i].Selected {
			hasSelected = true
			cfg.TargetAddress = cfg.Devices[i].Address
			break
		}
	}
	if !hasSelected && len(cfg.Devices) > 0 {
		cfg.Devices[0].Selected = true
		cfg.TargetAddress = cfg.Devices[0].Address
	}
}

func (s *Store) Load() Config {
	cfg := DefaultConfig()
	cfg.storePath = s.Path

	data, err := os.ReadFile(s.Path)
	if err == nil {
		_ = json.Unmarshal(data, &cfg)
		cfg.storePath = s.Path
	}

	normalizeDevices(&cfg)
	return cfg
}

func (s *Store) Save(cfg Config) error {
	dir := filepath.Dir(s.Path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(s.Path, data, 0600)
}

func LoadConfig() Config {
	return NewStore("").Load()
}

func SaveConfig(cfg Config) error {
	path := cfg.storePath
	if path == "" {
		path = GetConfigPath()
	}
	return NewStore(path).Save(cfg)
}

func (c *Config) SetStorePath(p string) {
	c.storePath = p
}

func (c *Config) GetSelectedDevice() *TargetDevice {
	for i := range c.Devices {
		if c.Devices[i].Selected {
			return &c.Devices[i]
		}
	}
	if len(c.Devices) > 0 {
		c.Devices[0].Selected = true
		c.TargetAddress = c.Devices[0].Address
		return &c.Devices[0]
	}
	return nil
}

func (c *Config) SelectDevice(idOrAddr string) bool {
	found := false
	for i := range c.Devices {
		if c.Devices[i].ID == idOrAddr || strings.EqualFold(c.Devices[i].Address, idOrAddr) || strings.EqualFold(c.Devices[i].Name, idOrAddr) {
			c.Devices[i].Selected = true
			c.TargetAddress = c.Devices[i].Address
			found = true
		} else {
			c.Devices[i].Selected = false
		}
	}
	if found {
		_ = SaveConfig(*c)
	}
	return found
}

func (c *Config) AddDevice(name, addr, platform string, selectNow bool) *TargetDevice {
	id := fmt.Sprintf("dev-%d", time.Now().UnixNano()%100000)
	platform = strings.ToLower(strings.TrimSpace(platform))
	if platform != "macos" && platform != "mac" {
		platform = "windows"
	} else {
		platform = "macos"
	}

	newDev := TargetDevice{
		ID:       id,
		Name:     name,
		Address:  strings.ToUpper(strings.TrimSpace(addr)),
		Platform: platform,
		Selected: selectNow,
	}

	if selectNow {
		for i := range c.Devices {
			c.Devices[i].Selected = false
		}
		c.TargetAddress = newDev.Address
	}

	c.Devices = append(c.Devices, newDev)
	_ = SaveConfig(*c)
	return &newDev
}

func (c *Config) DeleteDevice(idOrAddr string) (*TargetDevice, bool) {
	var deleted *TargetDevice
	var remaining []TargetDevice
	for i := range c.Devices {
		dev := c.Devices[i]
		if deleted == nil && (dev.ID == idOrAddr || strings.EqualFold(dev.Address, idOrAddr) || strings.EqualFold(dev.Name, idOrAddr)) {
			deleted = &dev
			continue
		}
		remaining = append(remaining, dev)
	}

	if deleted == nil {
		return nil, false
	}

	c.Devices = remaining
	if deleted.Selected && len(c.Devices) > 0 {
		c.Devices[0].Selected = true
		c.TargetAddress = c.Devices[0].Address
	} else if len(c.Devices) == 0 {
		c.TargetAddress = ""
	}

	_ = SaveConfig(*c)
	return deleted, true
}
