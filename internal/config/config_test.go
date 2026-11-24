package config

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestLoadMissingFileReturnsDefaultProvider(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}
	if want := (Config{Provider: ProviderOpenAI}); !reflect.DeepEqual(cfg, want) {
		t.Fatalf("Load() = %+v, want %+v", cfg, want)
	}
}

func TestSaveThenLoadRoundTrip(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	want := Config{Provider: ProviderAzure, BaseURL: "https://api.openai.com/v1", Model: "gpt-4o"}
	if err := Save(want); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	got, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Load() = %+v, want %+v", got, want)
	}
}

func TestSaveCreatesMissingDirectory(t *testing.T) {
	// XDG_CONFIG_HOME itself points at a path that does not exist yet;
	// Save must create the full mdiff subdirectory path.
	t.Setenv("XDG_CONFIG_HOME", t.TempDir()+"/does/not/exist/yet")

	if err := Save(Config{BaseURL: "http://localhost:11434/v1", Model: "llama3"}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	got, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got.BaseURL != "http://localhost:11434/v1" || got.Model != "llama3" {
		t.Fatalf("Load() = %+v, want restored config", got)
	}
}

func TestLoadFileWithoutProviderDefaultsToOpenAI(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	if err := os.MkdirAll(filepath.Join(dir, configDirName), 0o755); err != nil {
		t.Fatal(err)
	}
	old := `{"base_url": "https://api.openai.com/v1", "model": "gpt-4o"}`
	if err := os.WriteFile(filepath.Join(dir, configDirName, configFileName), []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	want := Config{Provider: ProviderOpenAI, BaseURL: "https://api.openai.com/v1", Model: "gpt-4o"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Load() = %+v, want %+v", got, want)
	}
}

func TestSaveCleansBaseURLAndModel(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	in := Config{Provider: ProviderOpenAI, BaseURL: "  https://api.openai.com/v1//  ", Model: "  gpt-4o "}
	if err := Save(in); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	got, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	want := Config{Provider: ProviderOpenAI, BaseURL: "https://api.openai.com/v1", Model: "gpt-4o"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Load() = %+v, want %+v", got, want)
	}
}

func TestNewLLM(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		apiKey  string
		want    LLM
		wantErr string
	}{
		{
			name:    "missing base URL",
			cfg:     Config{Provider: ProviderOpenAI, Model: "gpt-4o"},
			wantErr: "set the base URL",
		},
		{
			name:    "blank base URL after cleaning",
			cfg:     Config{Provider: ProviderOpenAI, BaseURL: "  /", Model: "gpt-4o"},
			wantErr: "set the base URL",
		},
		{
			name:    "openai missing model",
			cfg:     Config{Provider: ProviderOpenAI, BaseURL: "http://localhost:11434/v1"},
			wantErr: "set the model",
		},
		{
			name:    "azure missing deployment name",
			cfg:     Config{Provider: ProviderAzure, BaseURL: "https://x.openai.azure.com", Model: " "},
			apiKey:  "k",
			wantErr: "set the deployment name",
		},
		{
			name:    "azure missing API key",
			cfg:     Config{Provider: ProviderAzure, BaseURL: "https://x.openai.azure.com", Model: "dep"},
			wantErr: "set the API key",
		},
		{
			name:    "azure base URL is checked before deployment name and key",
			cfg:     Config{Provider: ProviderAzure},
			wantErr: "set the base URL",
		},
		{
			name: "openai with empty key is valid",
			cfg:  Config{Provider: ProviderOpenAI, BaseURL: " http://localhost:11434/v1/ ", Model: " llama3 "},
			want: LLM{Provider: ProviderOpenAI, BaseURL: "http://localhost:11434/v1", Model: "llama3"},
		},
		{
			name:   "azure complete",
			cfg:    Config{Provider: ProviderAzure, BaseURL: "https://x.openai.azure.com", Model: "dep"},
			apiKey: "k",
			want:   LLM{Provider: ProviderAzure, BaseURL: "https://x.openai.azure.com", Model: "dep", APIKey: "k"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewLLM(tt.cfg, tt.apiKey)
			if tt.wantErr != "" {
				if err == nil || err.Error() != tt.wantErr {
					t.Fatalf("NewLLM() error = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("NewLLM() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("NewLLM() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestLoadLLMCombinesSavedConfigAndKey(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if err := Save(Config{Provider: ProviderAzure, BaseURL: "https://x.openai.azure.com/", Model: "dep"}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	get := func(service, user string) (string, error) { return "k", nil }

	got, err := loadLLM(get)
	if err != nil {
		t.Fatalf("loadLLM() error = %v", err)
	}
	want := LLM{Provider: ProviderAzure, BaseURL: "https://x.openai.azure.com", Model: "dep", APIKey: "k"}
	if got != want {
		t.Fatalf("loadLLM() = %+v, want %+v", got, want)
	}
}

func TestLoadLLMWithoutSavedConfigAsksForBaseURL(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	get := func(service, user string) (string, error) { return "", errors.New("unused") }

	if _, err := loadLLM(get); err == nil || err.Error() != "set the base URL" {
		t.Fatalf("loadLLM() error = %v, want %q", err, "set the base URL")
	}
}
