package keychain

import (
	"github.com/ghost-hid/ghid/internal/vault"
)

const ServiceName = vault.DefaultServiceName

var defaultVault = vault.NewKeychainVault(ServiceName)

func IsAvailable() bool {
	return defaultVault.IsAvailable()
}

func SetPassword(username, password string) error {
	return defaultVault.Set(username, password)
}
