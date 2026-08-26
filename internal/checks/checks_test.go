package checks

import (
	"os"
	"path/filepath"
	"testing"
)

const validCheckFile = `---
name: "language-consistency"
description: |
  Flags identifiers, comments, or literals that are not consistently English.
color: red
---

Check for symbols that are not consistently English.
`

const malformedCheckFile = `no frontmatter here, just a prompt body`

func writeCheckFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", path, err)
	}
	return path
}

func TestParseFileValidCheck(t *testing.T) {
	dir := t.TempDir()
	path := writeCheckFile(t, dir, "language-consistency.md", validCheckFile)

	got, err := ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile() error = %v, want nil", err)
	}
	if got.Name != "language-consistency" {
		t.Errorf("Name = %q, want %q", got.Name, "language-consistency")
	}
	if got.Color != "red" {
		t.Errorf("Color = %q, want %q", got.Color, "red")
	}
	wantPrompt := "Check for symbols that are not consistently English."
	if got.Prompt != wantPrompt {
		t.Errorf("Prompt = %q, want %q", got.Prompt, wantPrompt)
	}
}

func TestParseFileMalformedFrontmatter(t *testing.T) {
	dir := t.TempDir()
	path := writeCheckFile(t, dir, "bad.md", malformedCheckFile)

	if _, err := ParseFile(path); err == nil {
		t.Fatal("ParseFile() error = nil, want error for malformed frontmatter")
	}
}

func TestListMissingDirectoryReturnsEmptySlice(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	got, err := List()
	if err != nil {
		t.Fatalf("List() error = %v, want nil", err)
	}
	if len(got) != 0 {
		t.Fatalf("List() = %+v, want empty slice", got)
	}
}

func TestListValidAndMalformedFiles(t *testing.T) {
	base := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", base)

	dir := filepath.Join(base, "mdiff", "checks")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	writeCheckFile(t, dir, "language-consistency.md", validCheckFile)
	writeCheckFile(t, dir, "bad.md", malformedCheckFile)

	got, err := List()
	if err != nil {
		t.Fatalf("List() error = %v, want nil", err)
	}
	if len(got) != 1 {
		t.Fatalf("List() returned %d checks, want 1 (malformed file should be skipped): %+v", len(got), got)
	}
	if got[0].Name != "language-consistency" {
		t.Errorf("Name = %q, want %q", got[0].Name, "language-consistency")
	}
}
