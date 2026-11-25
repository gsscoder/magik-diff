package brief

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
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

// initRepo creates a fresh git repo in a temp dir with a usable identity
// for commits, and returns its dir.
func initRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runGitIn(t, dir, "init", "-q")
	runGitIn(t, dir, "config", "user.email", "test@example.com")
	runGitIn(t, dir, "config", "user.name", "Test User")
	return dir
}

// writeFiles writes each of names under dir with some content.
func writeFiles(t *testing.T, dir string, names ...string) {
	t.Helper()
	for _, name := range names {
		writeFile(t, dir, name, "content of "+name)
	}
}

// docPaths returns the paths ListDocs finds in dir.
func docPaths(t *testing.T, dir string) []string {
	t.Helper()
	docs, err := ListDocs(context.Background(), dir)
	if err != nil {
		t.Fatalf("ListDocs() error = %v", err)
	}
	paths := make([]string, len(docs))
	for i, d := range docs {
		paths[i] = d.Path
	}
	return paths
}

func TestIsDoc_ExtensionsDecide(t *testing.T) {
	for _, name := range []string{"a.md", "a.MDX", "a.markdown", "a.rst", "a.adoc"} {
		if !isDoc(name) {
			t.Errorf("isDoc(%q) = false, want true", name)
		}
	}
	for _, name := range []string{"a.py", "a.html", "README", "md", "notes.md.bak"} {
		if isDoc(name) {
			t.Errorf("isDoc(%q) = true, want false", name)
		}
	}
}

func TestIsDoc_TxtOnlyWithADocLikeName(t *testing.T) {
	for _, name := range []string{"NOTES.txt", "notes.txt", "README.txt", "LICENSE.txt"} {
		if !isDoc(name) {
			t.Errorf("isDoc(%q) = false, want true", name)
		}
	}
	for _, name := range []string{"requirements.txt", "CMakeLists.txt", "robots.txt"} {
		if isDoc(name) {
			t.Errorf("isDoc(%q) = true, want false", name)
		}
	}
}

func TestListDocs_AllowedHiddenFoldersOnly(t *testing.T) {
	dir := initRepo(t)
	writeFiles(t, dir,
		".github/CONTRIBUTING.md",
		".github/ISSUE_TEMPLATE/bug.md",
		".github/.secret.md",
		".github/workflows/ci.py",
		".agent-files/spec.md",
		".cursor/rules/style.md",
		".hidden/a.md",
		".venv/lib/a.md",
		".cursor-old/a.md",
		"src/.notes/a.md",
		".dotfile.md",
		"ignored.md",
	)
	writeFile(t, dir, ".gitignore", "ignored.md\n")

	want := []string{
		".agent-files/spec.md",
		".cursor/rules/style.md",
		".github/CONTRIBUTING.md",
		".github/ISSUE_TEMPLATE/bug.md",
	}
	if got := docPaths(t, dir); !slices.Equal(got, want) {
		t.Errorf("ListDocs() paths = %q, want %q", got, want)
	}
}

func TestListDocs_GitignoreAppliesToHiddenDocFolders(t *testing.T) {
	dir := initRepo(t)
	writeFiles(t, dir, ".cursor/a.md", ".github/a.md")
	writeFile(t, dir, ".gitignore", ".cursor/\n")

	want := []string{".github/a.md"}
	if got := docPaths(t, dir); !slices.Equal(got, want) {
		t.Errorf("ListDocs() paths = %q, want %q", got, want)
	}
}

func TestListDocs_SuggestedFirstThenDepthThenPath(t *testing.T) {
	dir := initRepo(t)
	writeFiles(t, dir,
		"main.go",
		"requirements.txt",
		"README.md",
		"ARCHITECTURE.md",
		"claude.md",
		"AGENTS.md",
		"docs/guide.md",
		"web/README.md",
		"web/DESIGN.md",
		".agent-files/intent.md",
		".cursor/rules/style.md",
		"CHANGELOG.md",
		"LICENSE.txt",
		"notes.txt",
		"CODE_OF_CONDUCT.md",
		"SECURITY.md",
		"docs/api/routes.md",
		"web/docs/DESIGN.md",
		"other/README.md",
		"other/deep/README.md",
		"web/CHANGELOG.md",
		".github/PULL_REQUEST_TEMPLATE.md",
		".github/ISSUE_TEMPLATE/bug.md",
	)

	docs, err := ListDocs(context.Background(), dir)
	if err != nil {
		t.Fatalf("ListDocs() error = %v", err)
	}

	var paths, suggested []string
	for _, d := range docs {
		paths = append(paths, d.Path)
		if d.Suggested {
			suggested = append(suggested, d.Path)
		}
	}
	wantSuggested := []string{
		"AGENTS.md",
		"ARCHITECTURE.md",
		"README.md",
		"claude.md",
		".agent-files/intent.md",
		"docs/guide.md",
		".cursor/rules/style.md",
	}
	wantOthers := []string{
		"CHANGELOG.md",
		"CODE_OF_CONDUCT.md",
		"LICENSE.txt",
		"SECURITY.md",
		"notes.txt",
		".github/PULL_REQUEST_TEMPLATE.md",
		"other/README.md",
		"web/CHANGELOG.md",
		"web/DESIGN.md",
		"web/README.md",
		".github/ISSUE_TEMPLATE/bug.md",
		"docs/api/routes.md",
		"other/deep/README.md",
		"web/docs/DESIGN.md",
	}
	if want := slices.Concat(wantSuggested, wantOthers); !slices.Equal(paths, want) {
		t.Errorf("ListDocs() paths = %q, want %q", paths, want)
	}
	if !slices.Equal(suggested, wantSuggested) {
		t.Errorf("suggested = %q, want %q", suggested, wantSuggested)
	}
}

func TestListDocs_SizeAndMissingTrackedFile(t *testing.T) {
	dir := initRepo(t)
	writeFile(t, dir, "README.md", "12345")
	writeFile(t, dir, "GONE.md", "tracked, then deleted")
	runGitIn(t, dir, "add", ".")
	runGitIn(t, dir, "commit", "-q", "-m", "initial")
	if err := os.Remove(filepath.Join(dir, "GONE.md")); err != nil {
		t.Fatalf("remove GONE.md: %v", err)
	}

	docs, err := ListDocs(context.Background(), dir)
	if err != nil {
		t.Fatalf("ListDocs() error = %v", err)
	}
	want := []DocFile{{Path: "README.md", Size: 5, Suggested: true}}
	if !slices.Equal(docs, want) {
		t.Errorf("ListDocs() = %+v, want %+v", docs, want)
	}
}

func TestListDocs_UnmergedPathListedOnce(t *testing.T) {
	dir := initRepo(t)
	writeFile(t, dir, "README.md", "base\n")
	runGitIn(t, dir, "add", ".")
	runGitIn(t, dir, "commit", "-q", "-m", "base")
	runGitIn(t, dir, "checkout", "-q", "-b", "other")
	writeFile(t, dir, "README.md", "other\n")
	runGitIn(t, dir, "commit", "-q", "-am", "other")
	runGitIn(t, dir, "checkout", "-q", "-")
	writeFile(t, dir, "README.md", "main\n")
	runGitIn(t, dir, "commit", "-q", "-am", "main")
	merge := exec.Command("git", "merge", "-q", "other")
	merge.Dir = dir
	if err := merge.Run(); err == nil {
		t.Fatal("git merge succeeded, want a conflict")
	}

	want := []string{"README.md"}
	if got := docPaths(t, dir); !slices.Equal(got, want) {
		t.Errorf("ListDocs() paths = %q, want %q", got, want)
	}
}

func TestListDocs_NoDocsIsEmptyNotNil(t *testing.T) {
	dir := initRepo(t)
	writeFiles(t, dir, "main.go")

	docs, err := ListDocs(context.Background(), dir)
	if err != nil {
		t.Fatalf("ListDocs() error = %v", err)
	}
	if docs == nil || len(docs) != 0 {
		t.Errorf("ListDocs() = %#v, want an empty non-nil slice", docs)
	}
}
