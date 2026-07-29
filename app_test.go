package main

import (
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"

	"mdiff/internal/gitdiff"
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
	files := []gitdiff.FileChange{
		{Path: "a.txt", Type: gitdiff.Modified},
		{Path: "b.txt", Type: gitdiff.Added},
	}
	diffs := map[string]string{
		"a.txt": "diff a content\n",
		"b.txt": "diff b content\n",
	}

	got, err := combineDiffs(files, func(path string) (string, error) {
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
	files := []gitdiff.FileChange{{Path: "broken.txt", Type: gitdiff.Modified}}
	wantErr := errors.New("boom")

	_, err := combineDiffs(files, func(path string) (string, error) {
		return "", wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Errorf("combineDiffs error = %v, want %v", err, wantErr)
	}
}

func TestExplainAllChanges_NoChanges(t *testing.T) {
	initRepo(t)

	app := &App{}
	_, err := app.ExplainAllChanges()
	if err == nil {
		t.Fatal("ExplainAllChanges: expected error for a clean working tree, got nil")
	}
	if !strings.Contains(err.Error(), "nothing to explain") {
		t.Errorf("ExplainAllChanges error = %q, want it to mention %q", err.Error(), "nothing to explain")
	}
}

func TestExplainAllCommitChanges_NoFiles(t *testing.T) {
	dir := initRepo(t)
	runGitIn(t, dir, "commit", "--allow-empty", "-q", "-m", "empty commit")
	hash := runGitIn(t, dir, "rev-parse", "HEAD")

	app := &App{}
	_, err := app.ExplainAllCommitChanges(hash)
	if err == nil {
		t.Fatal("ExplainAllCommitChanges: expected error for a commit with no changed files, got nil")
	}
	if !strings.Contains(err.Error(), "nothing to explain") {
		t.Errorf("ExplainAllCommitChanges error = %q, want it to mention %q", err.Error(), "nothing to explain")
	}
}
