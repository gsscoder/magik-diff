// Package config handles persistence of mdiff's application configuration:
// non-secret settings on disk as JSON, and the API key in the OS keyring.
package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/zalando/go-keyring"
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

// Provider names the kind of OpenAI-compatible endpoint the LLM client talks to.
type Provider string

const (
	ProviderOpenAI Provider = "openai"
	ProviderAzure  Provider = "azure"
)

// Config holds the non-secret application settings. Model is the deployment
// name when Provider is ProviderAzure.
type Config struct {
	Provider Provider `json:"provider"`
	BaseURL  string   `json:"base_url"`
	Model    string   `json:"model"`
}

// LLM is the complete connection data for one LLM call: the non-secret
// settings plus the API key.
type LLM struct {
	Provider Provider
	BaseURL  string
	Model    string
	APIKey   string
}

// clean trims surrounding spaces from BaseURL and Model, and the trailing
// slashes from BaseURL so endpoint paths can be appended directly.
func (c Config) clean() Config {
	c.BaseURL = strings.TrimRight(strings.TrimSpace(c.BaseURL), "/")
	c.Model = strings.TrimSpace(c.Model)
	return c
}

// NewLLM cleans cfg, combines it with apiKey and checks the result is complete
// enough to make a call, so every caller applies the same rules.
func NewLLM(cfg Config, apiKey string) (LLM, error) {
	cfg = cfg.clean()
	ep := LLM{Provider: cfg.Provider, BaseURL: cfg.BaseURL, Model: cfg.Model, APIKey: apiKey}
	if err := requireComplete(ep); err != nil {
		return LLM{}, err
	}
	return ep, nil
}

// requireComplete reports the first missing setting. An OpenAI-compatible
// endpoint may run without a key (e.g. a local server); Azure always needs one.
func requireComplete(ep LLM) error {
	azure := ep.Provider == ProviderAzure
	switch {
	case ep.BaseURL == "":
		return errors.New("set the base URL")
	case ep.Model == "" && azure:
		return errors.New("set the deployment name")
	case ep.Model == "":
		return errors.New("set the model")
	case azure && ep.APIKey == "":
		return errors.New("set the API key")
	}
	return nil
}

// LoadLLM reads the saved settings and the API key and returns them as one
// complete LLM, or an error naming the first missing setting.
func LoadLLM() (LLM, error) {
	return loadLLM(keyring.Get)
}

func loadLLM(get keyringGetFunc) (LLM, error) {
	cfg, err := Load()
	if err != nil {
		return LLM{}, err
	}
	apiKey, _ := getAPIKey(get)
	return NewLLM(cfg, apiKey)
}

// BaseDir returns the directory the config file lives in, creating no
// directories itself.
func BaseDir() (string, error) {
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
	dir, err := BaseDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, configFileName), nil
}

// Load reads the config file. A missing file (or missing parent directory)
// is not an error: it returns a Config with only the default provider set.
// A file saved before providers existed also loads as ProviderOpenAI.
func Load() (Config, error) {
	path, err := configPath()
	if err != nil {
		return Config{}, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Config{Provider: ProviderOpenAI}, nil
		}
		return Config{}, err
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, err
	}
	if cfg.Provider == "" {
		cfg.Provider = ProviderOpenAI
	}
	return cfg, nil
}

// Save writes the cleaned config file, creating its parent directory if needed.
func Save(cfg Config) error {
	dir, err := BaseDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(cfg.clean(), "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(filepath.Join(dir, configFileName), data, 0o644)
}
