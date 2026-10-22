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
	DefaultUser        string         `json:"default_user"`
	TargetAddress      string         `json:"target_address"`
	WirelessADBAddress string         `json:"wireless_adb_address"`
	DeviceSerial       string         `json:"device_serial,omitempty"`
	Delays             Delays         `json:"delays"`
	Devices            []TargetDevice `json:"devices"`
}

func DefaultConfig() Config {
	return Config{
		DefaultUser:        "",
		TargetAddress:      "38:FC:98:E8:E3:87",
		WirelessADBAddress: "192.168.1.80:41839",
		Delays: Delays{
			Wake:    1.0,
			Overlay: 1.0,
			User:    1.2,
			Tab:     0.8,
			Pass:    1.2,
			Submit:  1.0,
		},
		Devices: []TargetDevice{
			{
				ID:       "dev-1",
				Name:     "LP-TZD-0X114650",
				Address:  "38:FC:98:E8:E3:87",
				Platform: "windows",
				Selected: true,
			},
		},
	}
}

func GetConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".config/ghid/config.json"
	}
	return filepath.Join(home, ".config", "ghid", "config.json")
}

func LoadConfig() Config {
	cfg := DefaultConfig()
	path := GetConfigPath()

	data, err := os.ReadFile(path)
	if err == nil {
		_ = json.Unmarshal(data, &cfg)
	}

	// Ensure default device exists and is selected
	if len(cfg.Devices) == 0 {
		name := "LP-TZD-0X114650"
		addr := "38:FC:98:E8:E3:87"
		if cfg.TargetAddress != "" {
			addr = cfg.TargetAddress
		}
		cfg.Devices = []TargetDevice{
			{
				ID:       "dev-1",
				Name:     name,
				Address:  addr,
				Platform: "windows",
				Selected: true,
			},
		}
		cfg.TargetAddress = addr
		_ = SaveConfig(cfg)
	} else {
		// Ensure exactly one device is selected
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

	return cfg
}

func SaveConfig(cfg Config) error {
	path := GetConfigPath()
	dir := filepath.Dir(path)

	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0600)
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
