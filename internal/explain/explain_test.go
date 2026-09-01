package explain

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"mdiff/internal/gitdiff"
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
// commits. Returns the repo dir and a Repo rooted at it.
func initRepo(t *testing.T) (string, *gitdiff.Repo) {
	t.Helper()
	dir := t.TempDir()
	runGitIn(t, dir, "init", "-q")
	runGitIn(t, dir, "config", "user.email", "test@example.com")
	runGitIn(t, dir, "config", "user.name", "Test User")
	return dir, gitdiff.New(dir)
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

func TestCombineDiffs(t *testing.T) {
	dir, repo := initRepo(t)
	runGitIn(t, dir, "commit", "--allow-empty", "-q", "-m", "initial")
	writeFile(t, dir, "a.txt", "a content\n")
	writeFile(t, dir, "b.txt", "b content\n")

	got, err := combineDiffs(context.Background(), repo, "", []string{"a.txt", "b.txt"})
	if err != nil {
		t.Fatalf("combineDiffs: %v", err)
	}
	if !strings.Contains(got, "--- a.txt ---") || !strings.Contains(got, "+a content") {
		t.Errorf("combineDiffs missing a.txt section, got:\n%s", got)
	}
	if !strings.Contains(got, "--- b.txt ---") || !strings.Contains(got, "+b content") {
		t.Errorf("combineDiffs missing b.txt section, got:\n%s", got)
	}
}

func TestCombineDiffs_PropagatesError(t *testing.T) {
	_, repo := initRepo(t)

	_, err := combineDiffs(context.Background(), repo, "not-a-valid-hash!!", []string{"a.txt"})
	if err == nil {
		t.Fatal("combineDiffs: expected an error for an invalid commit hash, got nil")
	}
}

func TestExplain_NoSelection(t *testing.T) {
	s := NewService()
	_, err := s.Explain(context.Background(), nil, "", []string{})
	if err == nil {
		t.Fatal("Explain: expected error when no files are selected, got nil")
	}
	if !strings.Contains(err.Error(), "nothing to explain: no files are selected") {
		t.Errorf("Explain error = %q, want it to mention %q", err.Error(), "nothing to explain: no files are selected")
	}
}

func TestRunCheck_NoSelection(t *testing.T) {
	s := NewService()
	_, err := s.RunCheck(context.Background(), nil, "", "language-consistency", []string{})
	if err == nil {
		t.Fatal("RunCheck: expected error when no files are selected, got nil")
	}
	if !strings.Contains(err.Error(), "nothing to check: no files are selected") {
		t.Errorf("RunCheck error = %q, want it to mention %q", err.Error(), "nothing to check: no files are selected")
	}
}

func TestService_CacheHitReturnsStoredResult(t *testing.T) {
	s := NewService()
	key := cacheKey("some diff", "gpt-4o-mini", "file")

	if _, ok := s.cached(key); ok {
		t.Fatal("cached() = ok before anything was stored, want not ok")
	}

	s.setCached(key, "cached explanation")

	got, ok := s.cached(key)
	if !ok || got != "cached explanation" {
		t.Fatalf("cached() = (%q, %v), want (%q, true)", got, ok, "cached explanation")
	}
}

func TestCacheKey_DiffersByDiffModelAndPromptKind(t *testing.T) {
	base := cacheKey("diff", "model", "file")
	tests := map[string]string{
		"diff":       cacheKey("other diff", "model", "file"),
		"model":      cacheKey("diff", "other-model", "file"),
		"promptKind": cacheKey("diff", "model", "check:foo"),
	}
	for name, got := range tests {
		if got == base {
			t.Errorf("cacheKey unchanged when varying %s: both = %q", name, base)
		}
	}
}

func TestCacheKey_StableForSameInputs(t *testing.T) {
	a := cacheKey("diff", "model", "file")
	b := cacheKey("diff", "model", "file")
	if a != b {
		t.Errorf("cacheKey not stable: %q != %q", a, b)
	}
}
