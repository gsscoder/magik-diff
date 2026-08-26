package gitdiff

import (
	"encoding/json"
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

// A clean repo must marshal to a JSON array ("[]"), not "null" — the Wails
// binding layer round-trips this through encoding/json to the frontend,
// where `null.map(...)` crashes the whole React tree. A nil Go slice
// marshals to "null", so ChangedFiles must never return a nil slice.
func TestChangedFiles_CleanRepoMarshalsToEmptyArrayNotNull(t *testing.T) {
	dir := initRepo(t)
	runGitIn(t, dir, "commit", "--allow-empty", "-q", "-m", "initial")

	changes, err := ChangedFiles()
	if err != nil {
		t.Fatalf("ChangedFiles: %v", err)
	}
	if changes == nil {
		t.Fatal("ChangedFiles returned a nil slice for a clean repo, want non-nil empty slice")
	}
	if len(changes) != 0 {
		t.Fatalf("changes = %+v, want empty", changes)
	}

	b, err := json.Marshal(changes)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	if string(b) != "[]" {
		t.Errorf("json.Marshal(changes) = %s, want []", b)
	}
}

func TestFileDiff_UntrackedFileShowsContentAsAdded(t *testing.T) {
	dir := initRepo(t)
	runGitIn(t, dir, "commit", "--allow-empty", "-q", "-m", "initial")

	writeFile(t, dir, "new.txt", "brand new content\n")

	diff, err := FileDiff("new.txt")
	if err != nil {
		t.Fatalf("FileDiff: %v", err)
	}
	if !strings.Contains(diff, "+brand new content") {
		t.Errorf("diff missing added content for untracked file, got:\n%s", diff)
	}
}

func TestFileDiff_StagedModificationIsVisible(t *testing.T) {
	dir := initRepo(t)
	writeFile(t, dir, "file.txt", "unchanged line\nold line\n")
	runGitIn(t, dir, "add", "file.txt")
	runGitIn(t, dir, "commit", "-q", "-m", "initial")

	writeFile(t, dir, "file.txt", "unchanged line\nnew line\n")
	runGitIn(t, dir, "add", "file.txt")

	diff, err := FileDiff("file.txt")
	if err != nil {
		t.Fatalf("FileDiff: %v", err)
	}
	if !strings.Contains(diff, "-old line") || !strings.Contains(diff, "+new line") {
		t.Errorf("diff missing staged change, got:\n%s", diff)
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

func TestRecentCommits_NewestFirstWithFields(t *testing.T) {
	dir := initRepo(t)
	runGitIn(t, dir, "commit", "--allow-empty", "-q", "-m", "first")
	runGitIn(t, dir, "commit", "--allow-empty", "-q", "-m", "second")
	runGitIn(t, dir, "commit", "--allow-empty", "-q", "-m", "third")

	commits, err := RecentCommits(0, 200)
	if err != nil {
		t.Fatalf("RecentCommits: %v", err)
	}
	if len(commits) != 3 {
		t.Fatalf("len(commits) = %d, want 3: %+v", len(commits), commits)
	}
	for i, want := range []string{"third", "second", "first"} {
		if commits[i].Subject != want {
			t.Errorf("commits[%d].Subject = %q, want %q", i, commits[i].Subject, want)
		}
	}
	head := commits[0]
	if head.Hash == "" || head.Author != "Test User" || head.Date == "" {
		t.Errorf("head commit fields incomplete: %+v", head)
	}
}

func TestRecentCommits_Paging(t *testing.T) {
	dir := initRepo(t)
	runGitIn(t, dir, "commit", "--allow-empty", "-q", "-m", "first")
	runGitIn(t, dir, "commit", "--allow-empty", "-q", "-m", "second")
	runGitIn(t, dir, "commit", "--allow-empty", "-q", "-m", "third")

	page, err := RecentCommits(2, 2)
	if err != nil {
		t.Fatalf("RecentCommits: %v", err)
	}
	if len(page) != 1 || page[0].Subject != "first" {
		t.Errorf("RecentCommits(2, 2) = %+v, want just the oldest commit", page)
	}
}

func TestRecentCommits_EmptyRepoReturnsEmptyNotError(t *testing.T) {
	initRepo(t)

	commits, err := RecentCommits(0, 200)
	if err != nil {
		t.Fatalf("RecentCommits on empty repo: %v", err)
	}
	if commits == nil {
		t.Fatal("RecentCommits returned a nil slice for an empty repo, want non-nil empty slice")
	}
	if len(commits) != 0 {
		t.Fatalf("commits = %+v, want empty", commits)
	}
}

func TestCommitFiles_AddModifyDeleteRename(t *testing.T) {
	dir := initRepo(t)
	writeFile(t, dir, "keep.txt", "old content\n")
	writeFile(t, dir, "gone.txt", "to be deleted\n")
	writeFile(t, dir, "old.txt", "same content\n")
	runGitIn(t, dir, "add", ".")
	runGitIn(t, dir, "commit", "-q", "-m", "initial")

	writeFile(t, dir, "keep.txt", "new content\n")
	writeFile(t, dir, "new.txt", "brand new\n")
	if err := os.Remove(filepath.Join(dir, "gone.txt")); err != nil {
		t.Fatalf("remove gone.txt: %v", err)
	}
	runGitIn(t, dir, "mv", "old.txt", "renamed.txt")
	runGitIn(t, dir, "add", ".")
	runGitIn(t, dir, "commit", "-q", "-m", "changes")

	head, err := RecentCommits(0, 1)
	if err != nil {
		t.Fatalf("RecentCommits: %v", err)
	}
	changes, err := CommitFiles(head[0].Hash)
	if err != nil {
		t.Fatalf("CommitFiles: %v", err)
	}
	for path, want := range map[string]ChangeType{
		"keep.txt":    Modified,
		"new.txt":     Added,
		"gone.txt":    Deleted,
		"renamed.txt": Renamed,
	} {
		if got := findChange(t, changes, path); got.Type != want {
			t.Errorf("%s: Type = %q, want %q", path, got.Type, want)
		}
	}
	if got := findChange(t, changes, "renamed.txt"); got.OrigPath != "old.txt" {
		t.Errorf("renamed.txt: OrigPath = %q, want %q", got.OrigPath, "old.txt")
	}
}

func TestCommitFiles_RootCommitListsAllFilesAsAdded(t *testing.T) {
	dir := initRepo(t)
	writeFile(t, dir, "one.txt", "one\n")
	writeFile(t, dir, "two.txt", "two\n")
	runGitIn(t, dir, "add", ".")
	runGitIn(t, dir, "commit", "-q", "-m", "initial")

	commits, err := RecentCommits(0, 1)
	if err != nil {
		t.Fatalf("RecentCommits: %v", err)
	}
	changes, err := CommitFiles(commits[0].Hash)
	if err != nil {
		t.Fatalf("CommitFiles: %v", err)
	}
	if len(changes) != 2 {
		t.Fatalf("len(changes) = %d, want 2: %+v", len(changes), changes)
	}
	for _, c := range changes {
		if c.Type != Added {
			t.Errorf("%s: Type = %q, want %q", c.Path, c.Type, Added)
		}
	}
}

func TestIsCodeFile(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"main.go", true},
		{"App.tsx", true},
		{"Program.cs", true},
		{"script.py", true},
		{"main.csproj", false},
		{"package.json", false},
		{"README.md", false},
		{"Makefile", false},
		{"noext", false},
	}
	for _, tt := range tests {
		if got := isCodeFile(tt.path); got != tt.want {
			t.Errorf("isCodeFile(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

func TestCommitFileDiff_ContainsAddedAndRemovedLines(t *testing.T) {
	dir := initRepo(t)
	writeFile(t, dir, "file.txt", "unchanged line\nold line\n")
	runGitIn(t, dir, "add", "file.txt")
	runGitIn(t, dir, "commit", "-q", "-m", "initial")

	writeFile(t, dir, "file.txt", "unchanged line\nnew line\n")
	runGitIn(t, dir, "add", "file.txt")
	runGitIn(t, dir, "commit", "-q", "-m", "change")

	head, err := RecentCommits(0, 1)
	if err != nil {
		t.Fatalf("RecentCommits: %v", err)
	}
	diff, err := CommitFileDiff(head[0].Hash, "file.txt")
	if err != nil {
		t.Fatalf("CommitFileDiff: %v", err)
	}
	if !strings.Contains(diff, "-old line") {
		t.Errorf("diff missing removed line, got:\n%s", diff)
	}
	if !strings.Contains(diff, "+new line") {
		t.Errorf("diff missing added line, got:\n%s", diff)
	}
}
