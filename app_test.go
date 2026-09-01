package main

import (
	"os/exec"
	"strings"
	"testing"
)

// runGitIn runs git with args inside dir, failing the test on error.
func runGitIn(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

// initRepo creates a fresh git repo in a temp dir with a usable identity for
// commits. Returns the repo dir.
func initRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runGitIn(t, dir, "init", "-q")
	runGitIn(t, dir, "config", "user.email", "test@example.com")
	runGitIn(t, dir, "config", "user.name", "Test User")
	return dir
}

func TestExplain_NoSelection(t *testing.T) {
	initRepo(t)

	app := &App{}
	_, err := app.Explain("", []string{})
	if err == nil {
		t.Fatal("Explain: expected error when no files are selected, got nil")
	}
	if !strings.Contains(err.Error(), "nothing to explain: no files are selected") {
		t.Errorf("Explain error = %q, want it to mention %q", err.Error(), "nothing to explain: no files are selected")
	}
}

func TestRunCheck_NoSelection(t *testing.T) {
	initRepo(t)

	app := &App{}
	_, err := app.RunCheck("", "language-consistency", []string{})
	if err == nil {
		t.Fatal("RunCheck: expected error when no files are selected, got nil")
	}
	if !strings.Contains(err.Error(), "nothing to check: no files are selected") {
		t.Errorf("RunCheck error = %q, want it to mention %q", err.Error(), "nothing to check: no files are selected")
	}
}
