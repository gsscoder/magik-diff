package main

import (
	"errors"
	"os"
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
// commits, and chdirs the test process into it, restoring the previous
// working directory when the test ends. Returns the repo dir.
func initRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runGitIn(t, dir, "init", "-q")
	runGitIn(t, dir, "config", "user.email", "test@example.com")
	runGitIn(t, dir, "config", "user.name", "Test User")

	orig, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("os.Chdir(%s): %v", dir, err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(orig); err != nil {
			t.Fatalf("os.Chdir(%s): %v", orig, err)
		}
	})
	return dir
}

func TestCombineDiffs(t *testing.T) {
	paths := []string{"a.txt", "b.txt"}
	diffs := map[string]string{
		"a.txt": "diff a content\n",
		"b.txt": "diff b content\n",
	}

	got, err := combineDiffs(paths, func(path string) (string, error) {
		return diffs[path], nil
	})
	if err != nil {
		t.Fatalf("combineDiffs: %v", err)
	}

	want := "--- a.txt ---\ndiff a content\n\n--- b.txt ---\ndiff b content\n"
	if got != want {
		t.Errorf("combineDiffs = %q, want %q", got, want)
	}
}

func TestCombineDiffs_PropagatesError(t *testing.T) {
	paths := []string{"broken.txt"}
	wantErr := errors.New("boom")

	_, err := combineDiffs(paths, func(path string) (string, error) {
		return "", wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Errorf("combineDiffs error = %v, want %v", err, wantErr)
	}
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
