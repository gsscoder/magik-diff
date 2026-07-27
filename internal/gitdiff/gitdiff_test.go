package gitdiff

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// runGitIn runs git with args inside dir, failing the test on error.
func runGitIn(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, out)
	}
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

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

func findChange(t *testing.T, changes []FileChange, path string) FileChange {
	t.Helper()
	for _, c := range changes {
		if c.Path == path {
			return c
		}
	}
	t.Fatalf("no change found for path %q in %+v", path, changes)
	return FileChange{}
}

func TestChangedFiles_Modified(t *testing.T) {
	dir := initRepo(t)
	writeFile(t, dir, "file.txt", "line one\nline two\n")
	runGitIn(t, dir, "add", "file.txt")
	runGitIn(t, dir, "commit", "-q", "-m", "initial")

	writeFile(t, dir, "file.txt", "line one\nline two changed\n")

	changes, err := ChangedFiles()
	if err != nil {
		t.Fatalf("ChangedFiles: %v", err)
	}
	got := findChange(t, changes, "file.txt")
	if got.Type != Modified {
		t.Errorf("Type = %q, want %q", got.Type, Modified)
	}
	if got.OrigPath != "" {
		t.Errorf("OrigPath = %q, want empty", got.OrigPath)
	}
}

func TestChangedFiles_Added(t *testing.T) {
	dir := initRepo(t)
	runGitIn(t, dir, "commit", "--allow-empty", "-q", "-m", "initial")

	writeFile(t, dir, "new.txt", "brand new content\n")

	// Untracked, not yet added.
	changes, err := ChangedFiles()
	if err != nil {
		t.Fatalf("ChangedFiles: %v", err)
	}
	got := findChange(t, changes, "new.txt")
	if got.Type != Added {
		t.Errorf("untracked: Type = %q, want %q", got.Type, Added)
	}

	// Staged via `git add`.
	runGitIn(t, dir, "add", "new.txt")
	changes, err = ChangedFiles()
	if err != nil {
		t.Fatalf("ChangedFiles: %v", err)
	}
	got = findChange(t, changes, "new.txt")
	if got.Type != Added {
		t.Errorf("staged: Type = %q, want %q", got.Type, Added)
	}
}

func TestChangedFiles_Deleted(t *testing.T) {
	dir := initRepo(t)
	writeFile(t, dir, "gone.txt", "will be deleted\n")
	runGitIn(t, dir, "add", "gone.txt")
	runGitIn(t, dir, "commit", "-q", "-m", "initial")

	if err := os.Remove(filepath.Join(dir, "gone.txt")); err != nil {
		t.Fatalf("remove gone.txt: %v", err)
	}

	changes, err := ChangedFiles()
	if err != nil {
		t.Fatalf("ChangedFiles: %v", err)
	}
	got := findChange(t, changes, "gone.txt")
	if got.Type != Deleted {
		t.Errorf("Type = %q, want %q", got.Type, Deleted)
	}
}

func TestChangedFiles_Renamed(t *testing.T) {
	dir := initRepo(t)
	writeFile(t, dir, "old.txt", "same content\n")
	runGitIn(t, dir, "add", "old.txt")
	runGitIn(t, dir, "commit", "-q", "-m", "initial")

	runGitIn(t, dir, "mv", "old.txt", "renamed.txt")

	changes, err := ChangedFiles()
	if err != nil {
		t.Fatalf("ChangedFiles: %v", err)
	}
	got := findChange(t, changes, "renamed.txt")
	if got.Type != Renamed {
		t.Errorf("Type = %q, want %q", got.Type, Renamed)
	}
	if got.OrigPath != "old.txt" {
		t.Errorf("OrigPath = %q, want %q", got.OrigPath, "old.txt")
	}
}

func TestFileDiff_ModifiedContainsAddedAndRemovedLines(t *testing.T) {
	dir := initRepo(t)
	writeFile(t, dir, "file.txt", "unchanged line\nold line\n")
	runGitIn(t, dir, "add", "file.txt")
	runGitIn(t, dir, "commit", "-q", "-m", "initial")

	writeFile(t, dir, "file.txt", "unchanged line\nnew line\n")

	diff, err := FileDiff("file.txt")
	if err != nil {
		t.Fatalf("FileDiff: %v", err)
	}
	if !strings.Contains(diff, "-old line") {
		t.Errorf("diff missing removed line, got:\n%s", diff)
	}
	if !strings.Contains(diff, "+new line") {
		t.Errorf("diff missing added line, got:\n%s", diff)
	}
}
