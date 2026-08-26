// Package config handles persistence of mdiff's application configuration:
// non-secret settings on disk as JSON, and the API key in the OS keyring.
package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// configDirName and configFileName make up the on-disk location:
// $XDG_CONFIG_HOME/mdiff/mdiff.json, falling back to ~/.config/mdiff/mdiff.json
// when XDG_CONFIG_HOME is unset. This deliberately does not use
// os.UserConfigDir() so the layout is identical, and XDG_CONFIG_HOME-aware,
// on Windows too.
const (
	configDirName  = "mdiff"
	configFileName = "mdiff.json"
)

// Config holds the non-secret application settings.
type Config struct {
	BaseURL       string   `json:"base_url"`
	Model         string   `json:"model"`
	EnabledChecks []string `json:"enabled_checks"`
}

// configDir returns the directory the config file lives in, creating no
// directories itself.
func configDir() (string, error) {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, configDirName), nil
}

func configPath() (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, configFileName), nil
}

// Load reads the config file. A missing file (or missing parent directory)
// is not an error: it returns a zero-value Config.
func Load() (Config, error) {
	path, err := configPath()
	if err != nil {
		return Config{}, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Config{}, nil
		}
		return Config{}, err
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// Save writes the config file, creating its parent directory if needed.
func Save(cfg Config) error {
	dir, err := configDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(filepath.Join(dir, configFileName), data, 0o644)
}
