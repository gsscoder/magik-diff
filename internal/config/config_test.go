package config

import (
	"reflect"
	"testing"
)

func TestLoadMissingFileReturnsZeroValue(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}
	if !reflect.DeepEqual(cfg, Config{}) {
		t.Fatalf("Load() = %+v, want zero-value Config", cfg)
	}
}

func TestSaveThenLoadRoundTrip(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	want := Config{BaseURL: "https://api.openai.com/v1", Model: "gpt-4o"}
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
