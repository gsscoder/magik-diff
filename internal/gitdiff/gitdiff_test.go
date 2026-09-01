package gitdiff

import (
	"context"
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
	runGitInOut(t, dir, args...)
}

// runGitInOut runs git with args inside dir, failing the test on error, and
// returns its combined output.
func runGitInOut(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, out)
	}
	return string(out)
}

// initRepo creates a fresh git repo in a temp dir with a usable identity for
// commits. Returns the repo dir and a Repo rooted at it.
func initRepo(t *testing.T) (string, *Repo) {
	t.Helper()
	dir := t.TempDir()
	runGitIn(t, dir, "init", "-q")
	runGitIn(t, dir, "config", "user.email", "test@example.com")
	runGitIn(t, dir, "config", "user.name", "Test User")
	return dir, New(dir)
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
	dir, repo := initRepo(t)
	writeFile(t, dir, "file.txt", "line one\nline two\n")
	runGitIn(t, dir, "add", "file.txt")
	runGitIn(t, dir, "commit", "-q", "-m", "initial")

	writeFile(t, dir, "file.txt", "line one\nline two changed\n")

	changes, err := repo.ChangedFiles(context.Background())
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
	dir, repo := initRepo(t)
	runGitIn(t, dir, "commit", "--allow-empty", "-q", "-m", "initial")

	writeFile(t, dir, "new.txt", "brand new content\n")

	// Untracked, not yet added.
	changes, err := repo.ChangedFiles(context.Background())
	if err != nil {
		t.Fatalf("ChangedFiles: %v", err)
	}
	got := findChange(t, changes, "new.txt")
	if got.Type != Added {
		t.Errorf("untracked: Type = %q, want %q", got.Type, Added)
	}

	// Staged via `git add`.
	runGitIn(t, dir, "add", "new.txt")
	changes, err = repo.ChangedFiles(context.Background())
	if err != nil {
		t.Fatalf("ChangedFiles: %v", err)
	}
	got = findChange(t, changes, "new.txt")
	if got.Type != Added {
		t.Errorf("staged: Type = %q, want %q", got.Type, Added)
	}
}

func TestChangedFiles_Deleted(t *testing.T) {
	dir, repo := initRepo(t)
	writeFile(t, dir, "gone.txt", "will be deleted\n")
	runGitIn(t, dir, "add", "gone.txt")
	runGitIn(t, dir, "commit", "-q", "-m", "initial")

	if err := os.Remove(filepath.Join(dir, "gone.txt")); err != nil {
		t.Fatalf("remove gone.txt: %v", err)
	}

	changes, err := repo.ChangedFiles(context.Background())
	if err != nil {
		t.Fatalf("ChangedFiles: %v", err)
	}
	got := findChange(t, changes, "gone.txt")
	if got.Type != Deleted {
		t.Errorf("Type = %q, want %q", got.Type, Deleted)
	}
}

func TestChangedFiles_Renamed(t *testing.T) {
	dir, repo := initRepo(t)
	writeFile(t, dir, "old.txt", "same content\n")
	runGitIn(t, dir, "add", "old.txt")
	runGitIn(t, dir, "commit", "-q", "-m", "initial")

	runGitIn(t, dir, "mv", "old.txt", "renamed.txt")

	changes, err := repo.ChangedFiles(context.Background())
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

func TestChangedFiles_UntrackedDirectoryListsFilesIndividually(t *testing.T) {
	dir, repo := initRepo(t)
	runGitIn(t, dir, "commit", "--allow-empty", "-q", "-m", "initial")

	if err := os.MkdirAll(filepath.Join(dir, "newdir"), 0o755); err != nil {
		t.Fatalf("mkdir newdir: %v", err)
	}
	writeFile(t, dir, filepath.Join("newdir", "one.txt"), "one\n")
	writeFile(t, dir, filepath.Join("newdir", "two.txt"), "two\n")

	changes, err := repo.ChangedFiles(context.Background())
	if err != nil {
		t.Fatalf("ChangedFiles: %v", err)
	}
	for _, path := range []string{
		filepath.ToSlash(filepath.Join("newdir", "one.txt")),
		filepath.ToSlash(filepath.Join("newdir", "two.txt")),
	} {
		findChange(t, changes, path)
	}
	if got := findChange(t, changes, filepath.ToSlash(filepath.Join("newdir", "one.txt"))); got.Type != Added {
		t.Errorf("newdir/one.txt: Type = %q, want %q", got.Type, Added)
	}
}

// A clean repo must marshal to a JSON array ("[]"), not "null" — the Wails
// binding layer round-trips this through encoding/json to the frontend,
// where `null.map(...)` crashes the whole React tree. A nil Go slice
// marshals to "null", so ChangedFiles must never return a nil slice.
func TestChangedFiles_CleanRepoMarshalsToEmptyArrayNotNull(t *testing.T) {
	dir, repo := initRepo(t)
	runGitIn(t, dir, "commit", "--allow-empty", "-q", "-m", "initial")

	changes, err := repo.ChangedFiles(context.Background())
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
	dir, repo := initRepo(t)
	runGitIn(t, dir, "commit", "--allow-empty", "-q", "-m", "initial")

	writeFile(t, dir, "new.txt", "brand new content\n")

	diff, err := repo.FileDiff(context.Background(), "new.txt")
	if err != nil {
		t.Fatalf("FileDiff: %v", err)
	}
	if !strings.Contains(diff, "+brand new content") {
		t.Errorf("diff missing added content for untracked file, got:\n%s", diff)
	}
}

func TestFileDiff_UnbornHeadStagedFileIsVisible(t *testing.T) {
	dir, repo := initRepo(t)
	writeFile(t, dir, "new.txt", "brand new content\n")
	runGitIn(t, dir, "add", "new.txt")

	diff, err := repo.FileDiff(context.Background(), "new.txt")
	if err != nil {
		t.Fatalf("FileDiff: %v", err)
	}
	if diff == "" {
		t.Fatal("FileDiff returned an empty diff for a staged file in a repo with no commits")
	}
	if !strings.Contains(diff, "+brand new content") {
		t.Errorf("diff missing added content, got:\n%s", diff)
	}
}

func TestFileDiff_StagedModificationIsVisible(t *testing.T) {
	dir, repo := initRepo(t)
	writeFile(t, dir, "file.txt", "unchanged line\nold line\n")
	runGitIn(t, dir, "add", "file.txt")
	runGitIn(t, dir, "commit", "-q", "-m", "initial")

	writeFile(t, dir, "file.txt", "unchanged line\nnew line\n")
	runGitIn(t, dir, "add", "file.txt")

	diff, err := repo.FileDiff(context.Background(), "file.txt")
	if err != nil {
		t.Fatalf("FileDiff: %v", err)
	}
	if !strings.Contains(diff, "-old line") || !strings.Contains(diff, "+new line") {
		t.Errorf("diff missing staged change, got:\n%s", diff)
	}
}

func TestFileDiff_ModifiedContainsAddedAndRemovedLines(t *testing.T) {
	dir, repo := initRepo(t)
	writeFile(t, dir, "file.txt", "unchanged line\nold line\n")
	runGitIn(t, dir, "add", "file.txt")
	runGitIn(t, dir, "commit", "-q", "-m", "initial")

	writeFile(t, dir, "file.txt", "unchanged line\nnew line\n")

	diff, err := repo.FileDiff(context.Background(), "file.txt")
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
	dir, repo := initRepo(t)
	runGitIn(t, dir, "commit", "--allow-empty", "-q", "-m", "first")
	runGitIn(t, dir, "commit", "--allow-empty", "-q", "-m", "second")
	runGitIn(t, dir, "commit", "--allow-empty", "-q", "-m", "third")

	commits, err := repo.RecentCommits(context.Background(), 0, 200)
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
	dir, repo := initRepo(t)
	runGitIn(t, dir, "commit", "--allow-empty", "-q", "-m", "first")
	runGitIn(t, dir, "commit", "--allow-empty", "-q", "-m", "second")
	runGitIn(t, dir, "commit", "--allow-empty", "-q", "-m", "third")

	page, err := repo.RecentCommits(context.Background(), 2, 2)
	if err != nil {
		t.Fatalf("RecentCommits: %v", err)
	}
	if len(page) != 1 || page[0].Subject != "first" {
		t.Errorf("RecentCommits(2, 2) = %+v, want just the oldest commit", page)
	}
}

func TestRecentCommits_EmptyRepoReturnsEmptyNotError(t *testing.T) {
	_, repo := initRepo(t)

	commits, err := repo.RecentCommits(context.Background(), 0, 200)
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
	dir, repo := initRepo(t)
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

	head, err := repo.RecentCommits(context.Background(), 0, 1)
	if err != nil {
		t.Fatalf("RecentCommits: %v", err)
	}
	changes, err := repo.CommitFiles(context.Background(), head[0].Hash)
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

func TestCommitFiles_MergeCommitDiffsAgainstFirstParent(t *testing.T) {
	dir, repo := initRepo(t)
	writeFile(t, dir, "base.txt", "base\n")
	runGitIn(t, dir, "add", ".")
	runGitIn(t, dir, "commit", "-q", "-m", "initial")
	base := strings.TrimSpace(runGitInOut(t, dir, "branch", "--show-current"))

	runGitIn(t, dir, "checkout", "-q", "-b", "feature")
	writeFile(t, dir, "feature.txt", "feature content\n")
	runGitIn(t, dir, "add", ".")
	runGitIn(t, dir, "commit", "-q", "-m", "add feature file")

	runGitIn(t, dir, "checkout", "-q", base)
	runGitIn(t, dir, "merge", "-q", "--no-ff", "-m", "merge feature", "feature")

	head, err := repo.RecentCommits(context.Background(), 0, 1)
	if err != nil {
		t.Fatalf("RecentCommits: %v", err)
	}

	changes, err := repo.CommitFiles(context.Background(), head[0].Hash)
	if err != nil {
		t.Fatalf("CommitFiles: %v", err)
	}
	if len(changes) == 0 {
		t.Fatal("CommitFiles returned no changes for a merge commit")
	}
	findChange(t, changes, "feature.txt")

	diff, err := repo.CommitFileDiff(context.Background(), head[0].Hash, "feature.txt")
	if err != nil {
		t.Fatalf("CommitFileDiff: %v", err)
	}
	if !strings.HasPrefix(diff, "diff --git") {
		t.Errorf("CommitFileDiff output = %q, want it to start with %q", diff, "diff --git")
	}
}

func TestCommitFiles_RootCommitListsAllFilesAsAdded(t *testing.T) {
	dir, repo := initRepo(t)
	writeFile(t, dir, "one.txt", "one\n")
	writeFile(t, dir, "two.txt", "two\n")
	runGitIn(t, dir, "add", ".")
	runGitIn(t, dir, "commit", "-q", "-m", "initial")

	commits, err := repo.RecentCommits(context.Background(), 0, 1)
	if err != nil {
		t.Fatalf("RecentCommits: %v", err)
	}
	changes, err := repo.CommitFiles(context.Background(), commits[0].Hash)
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

func TestValidateHash_RejectsFlagLikeInput(t *testing.T) {
	_, repo := initRepo(t)

	tests := []struct {
		name string
		call func() error
	}{
		{
			name: "CommitFiles",
			call: func() error {
				_, err := repo.CommitFiles(context.Background(), "--output=x")
				return err
			},
		},
		{
			name: "CommitFileDiff",
			call: func() error {
				_, err := repo.CommitFileDiff(context.Background(), "--output=x", "f")
				return err
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.call(); err == nil {
				t.Fatalf("%s(%q): expected error, got nil", tt.name, "--output=x")
			}
		})
	}
}

func TestCommitFileDiff_ContainsAddedAndRemovedLines(t *testing.T) {
	dir, repo := initRepo(t)
	writeFile(t, dir, "file.txt", "unchanged line\nold line\n")
	runGitIn(t, dir, "add", "file.txt")
	runGitIn(t, dir, "commit", "-q", "-m", "initial")

	writeFile(t, dir, "file.txt", "unchanged line\nnew line\n")
	runGitIn(t, dir, "add", "file.txt")
	runGitIn(t, dir, "commit", "-q", "-m", "change")

	head, err := repo.RecentCommits(context.Background(), 0, 1)
	if err != nil {
		t.Fatalf("RecentCommits: %v", err)
	}
	diff, err := repo.CommitFileDiff(context.Background(), head[0].Hash, "file.txt")
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
