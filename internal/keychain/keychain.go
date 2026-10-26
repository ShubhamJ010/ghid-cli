package keychain

import (
	"github.com/ShubhamJ010/ghid-cli/internal/vault"
)

const ServiceName = vault.DefaultServiceName

var defaultVault = vault.NewKeychainVault(ServiceName)

func IsAvailable() bool {
	return defaultVault.IsAvailable()
}

func SetPassword(username, password string) error {
	return defaultVault.Set(username, password)
}
