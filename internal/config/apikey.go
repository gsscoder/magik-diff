package config

import (
	"errors"
	"os"

	"github.com/zalando/go-keyring"
)

// keyringService and keyringAccount identify the secret in the OS keyring.
const (
	keyringService = "mdiff"
	keyringAccount = "openai-api-key"
)

// envAPIKey is the environment variable consulted when the OS keyring
// backend itself is unavailable.
const envAPIKey = "MDIFF_OPENAI_API_KEY"

// keyringGetFunc matches keyring.Get's signature, so the keyring backend
// can be swapped out in tests without touching the real OS keyring.
type keyringGetFunc func(service, user string) (string, error)

// GetAPIKey returns the configured API key. It checks the OS keyring first.
//
//   - If the keyring has no entry (keyring.ErrNotFound), that is a normal
//     empty state (no key was ever set) and is not an error, nor a reason
//     to fall back to the environment variable.
//   - If the keyring read fails for any other reason (e.g. no keyring
//     backend is reachable on this OS), GetAPIKey falls back to the
//     MDIFF_OPENAI_API_KEY environment variable and reports usedFallback
//     as true, so a caller can surface a visible warning about the source.
func GetAPIKey() (key string, usedFallback bool) {
	return getAPIKey(keyring.Get)
}

func getAPIKey(get keyringGetFunc) (key string, usedFallback bool) {
	key, err := get(keyringService, keyringAccount)
	switch {
	case err == nil:
		return key, false
	case errors.Is(err, keyring.ErrNotFound):
		return "", false
	default:
		return os.Getenv(envAPIKey), true
	}
}

// SetAPIKey stores the API key in the OS keyring.
func SetAPIKey(key string) error {
	return keyring.Set(keyringService, keyringAccount, key)
}
