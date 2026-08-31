package watch

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
// commits. Returns the repo dir.
func initRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runGitIn(t, dir, "init", "-q")
	runGitIn(t, dir, "config", "user.email", "test@example.com")
	runGitIn(t, dir, "config", "user.name", "Test User")
	return dir
}

func writeFile(t *testing.T, dir, rel, content string) {
	t.Helper()
	full := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("mkdir for %s: %v", rel, err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", rel, err)
	}
}

func TestIgnoredDirs_SkipsGitignoredDirectory(t *testing.T) {
	dir := initRepo(t)
	writeFile(t, dir, ".gitignore", "node_modules/\n")
	writeFile(t, dir, "node_modules/pkg/index.js", "console.log(1)\n")
	writeFile(t, dir, "src/main.go", "package main\n")

	ignored, err := ignoredDirs(dir)
	if err != nil {
		t.Fatalf("ignoredDirs: %v", err)
	}

	if _, ok := ignored[filepath.FromSlash("node_modules")]; !ok {
		t.Errorf("ignoredDirs = %v, want it to contain %q", ignored, "node_modules")
	}
	if _, ok := ignored[filepath.FromSlash("src")]; ok {
		t.Errorf("ignoredDirs = %v, want it to not contain tracked %q", ignored, "src")
	}
}

func TestWatchTree_SkipsIgnoredAndGitDirectories(t *testing.T) {
	dir := initRepo(t)
	writeFile(t, dir, ".gitignore", "node_modules/\n")
	writeFile(t, dir, "node_modules/pkg/index.js", "console.log(1)\n")
	writeFile(t, dir, "src/main.go", "package main\n")
	runGitIn(t, dir, "add", ".")
	runGitIn(t, dir, "commit", "-q", "-m", "initial")

	w, err := New(dir, func() {})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer w.Close()

	watched := w.fsw.WatchList()
	want := map[string]bool{
		dir:                                    false,
		filepath.Join(dir, "src"):              false,
		filepath.Join(dir, ".git"):             false,
		filepath.Join(dir, "node_modules"):     true, // must NOT be present
		filepath.Join(dir, "node_modules/pkg"): true, // must NOT be present
	}
	for _, p := range watched {
		if mustAbsent, ok := want[p]; ok {
			if mustAbsent {
				t.Errorf("WatchList contains ignored path %q", p)
			}
			delete(want, p)
		}
	}
	for p, mustAbsent := range want {
		if !mustAbsent {
			t.Errorf("WatchList missing expected path %q, got %v", p, watched)
		}
	}
}

func TestStatus_ChangesWhenTrackedFileEdited(t *testing.T) {
	dir := initRepo(t)
	writeFile(t, dir, "file.txt", "line one\n")
	runGitIn(t, dir, "add", "file.txt")
	runGitIn(t, dir, "commit", "-q", "-m", "initial")

	sig1, err := status(dir)
	if err != nil {
		t.Fatalf("status: %v", err)
	}

	writeFile(t, dir, "file.txt", "line one changed\n")
	sig2, err := status(dir)
	if err != nil {
		t.Fatalf("status: %v", err)
	}

	if sig1 == sig2 {
		t.Errorf("status signature unchanged after editing a tracked file")
	}
}

func TestStatus_UnchangedWhenResavedIdentical(t *testing.T) {
	dir := initRepo(t)
	writeFile(t, dir, "file.txt", "line one\n")
	runGitIn(t, dir, "add", "file.txt")
	runGitIn(t, dir, "commit", "-q", "-m", "initial")

	sig1, err := status(dir)
	if err != nil {
		t.Fatalf("status: %v", err)
	}

	writeFile(t, dir, "file.txt", "line one\n") // identical content
	sig2, err := status(dir)
	if err != nil {
		t.Fatalf("status: %v", err)
	}

	if sig1 != sig2 {
		t.Errorf("status signature changed after resaving identical content")
	}
}

func TestCheckIgnored(t *testing.T) {
	dir := initRepo(t)
	writeFile(t, dir, ".gitignore", "ignored.txt\n")
	writeFile(t, dir, "ignored.txt", "x\n")
	writeFile(t, dir, "tracked.txt", "x\n")

	if !checkIgnored(dir, filepath.Join(dir, "ignored.txt")) {
		t.Errorf("checkIgnored(ignored.txt) = false, want true")
	}
	if checkIgnored(dir, filepath.Join(dir, "tracked.txt")) {
		t.Errorf("checkIgnored(tracked.txt) = true, want false")
	}
}
