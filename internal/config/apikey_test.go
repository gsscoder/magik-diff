package config

import (
	"errors"
	"testing"

	"github.com/zalando/go-keyring"
)

func TestGetAPIKeyFromKeyring(t *testing.T) {
	get := func(service, user string) (string, error) {
		return "sk-from-keyring", nil
	}

	key, usedFallback := getAPIKey(get)
	if key != "sk-from-keyring" || usedFallback {
		t.Fatalf("getAPIKey() = (%q, %v), want (%q, false)", key, usedFallback, "sk-from-keyring")
	}
}

func TestGetAPIKeyNotFoundIsEmptyNotFallback(t *testing.T) {
	get := func(service, user string) (string, error) {
		return "", keyring.ErrNotFound
	}

	// A key set in the env var must NOT be used when the keyring is simply
	// empty (not-found is a normal empty state, not a reason to fall back).
	t.Setenv("MDIFF_OPENAI_API_KEY", "sk-from-env")

	key, usedFallback := getAPIKey(get)
	if key != "" || usedFallback {
		t.Fatalf("getAPIKey() = (%q, %v), want (\"\", false)", key, usedFallback)
	}
}

func TestGetAPIKeyBackendUnavailableFallsBackToEnv(t *testing.T) {
	get := func(service, user string) (string, error) {
		return "", errors.New("no keyring backend reachable")
	}
	t.Setenv("MDIFF_OPENAI_API_KEY", "sk-from-env")

	key, usedFallback := getAPIKey(get)
	if key != "sk-from-env" || !usedFallback {
		t.Fatalf("getAPIKey() = (%q, %v), want (%q, true)", key, usedFallback, "sk-from-env")
	}
}

func TestGetAPIKeyBackendUnavailableAndNoEnvVar(t *testing.T) {
	get := func(service, user string) (string, error) {
		return "", errors.New("no keyring backend reachable")
	}
	t.Setenv("MDIFF_OPENAI_API_KEY", "")

	key, usedFallback := getAPIKey(get)
	if key != "" || !usedFallback {
		t.Fatalf("getAPIKey() = (%q, %v), want (\"\", true)", key, usedFallback)
	}
}
