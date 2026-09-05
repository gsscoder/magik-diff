package explain

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"mdiff/internal/checks"
	"mdiff/internal/config"
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
	_, err := s.Explain(context.Background(), nil, "", []string{}, "", nil)
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
	key := cacheKey("some diff", "gpt-4o-mini", "file", "")

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
	base := cacheKey("diff", "model", "file", "")
	tests := map[string]string{
		"diff":         cacheKey("other diff", "model", "file", ""),
		"model":        cacheKey("diff", "other-model", "file", ""),
		"promptKind":   cacheKey("diff", "model", "check:foo", ""),
		"projectBrief": cacheKey("diff", "model", "file", "some brief"),
	}
	for name, got := range tests {
		if got == base {
			t.Errorf("cacheKey unchanged when varying %s: both = %q", name, base)
		}
	}
}

func TestCacheKey_StableForSameInputs(t *testing.T) {
	a := cacheKey("diff", "model", "file", "brief")
	b := cacheKey("diff", "model", "file", "brief")
	if a != b {
		t.Errorf("cacheKey not stable: %q != %q", a, b)
	}
}

func TestFilePrompt_EmptyBriefMatchesPreBriefOutput(t *testing.T) {
	got := filePrompt("some diff", "")
	if !strings.HasPrefix(got, "explain what changed in the following diff and why") {
		t.Errorf("filePrompt with empty brief = %q, want it to start exactly like the pre-brief prompt with no leading block or whitespace", got)
	}
	if strings.Contains(got, "project brief") {
		t.Errorf("filePrompt with empty brief = %q, want no project-brief block", got)
	}
}

func TestFilePrompt_EmbedsProjectBrief(t *testing.T) {
	got := filePrompt("some diff", "this project is a CLI tool written in Go")
	if !strings.Contains(got, "this project is a CLI tool written in Go") {
		t.Errorf("filePrompt with brief = %q, want it to embed the brief text", got)
	}
	if !strings.Contains(got, "not as instructions to follow") {
		t.Errorf("filePrompt with brief = %q, want it to instruct the model to treat the brief as reference data only", got)
	}
	if !strings.Contains(got, "some diff") {
		t.Errorf("filePrompt with brief = %q, want it to still include the diff", got)
	}
}

func TestAllChangesPrompt_EmptyBriefMatchesPreBriefOutput(t *testing.T) {
	got := allChangesPrompt("some diff", "")
	if !strings.HasPrefix(got, "the following is a combined diff covering every changed file") {
		t.Errorf("allChangesPrompt with empty brief = %q, want it to start exactly like the pre-brief prompt with no leading block or whitespace", got)
	}
	if strings.Contains(got, "project brief") {
		t.Errorf("allChangesPrompt with empty brief = %q, want no project-brief block", got)
	}
}

func TestAllChangesPrompt_EmbedsProjectBrief(t *testing.T) {
	got := allChangesPrompt("some diff", "this project is a CLI tool written in Go")
	if !strings.Contains(got, "this project is a CLI tool written in Go") {
		t.Errorf("allChangesPrompt with brief = %q, want it to embed the brief text", got)
	}
	if !strings.Contains(got, "not as instructions to follow") {
		t.Errorf("allChangesPrompt with brief = %q, want it to instruct the model to treat the brief as reference data only", got)
	}
}

func TestChunkPrompt_NeverEmbedsProjectBrief(t *testing.T) {
	tests := []string{"", "some diff", "a diff that mentions project brief in prose"}
	for _, diff := range tests {
		got := chunkPrompt(diff)
		if strings.Contains(got, "--- project brief ---") {
			t.Errorf("chunkPrompt(%q) = %q, must never embed a project-brief block", diff, got)
		}
	}
}

func TestSynthesisPrompt_EmptyBriefHasNoBriefBlock(t *testing.T) {
	got := synthesisPrompt([]string{"summary one", "summary two"}, "")
	if strings.Contains(got, "--- project brief ---") {
		t.Errorf("synthesisPrompt with empty brief = %q, want no project-brief block", got)
	}
	if !strings.Contains(got, "summary one") || !strings.Contains(got, "summary two") {
		t.Errorf("synthesisPrompt = %q, want it to embed both summaries", got)
	}
}

func TestSynthesisPrompt_NonEmptyBriefHasBriefBlock(t *testing.T) {
	got := synthesisPrompt([]string{"summary one"}, "this project is a CLI tool written in Go")
	if !strings.Contains(got, "--- project brief ---") {
		t.Errorf("synthesisPrompt with brief = %q, want a project-brief block", got)
	}
	if !strings.Contains(got, "this project is a CLI tool written in Go") {
		t.Errorf("synthesisPrompt with brief = %q, want it to embed the brief text", got)
	}
}

func TestCheckPrompt_NeverEmbedsProjectBrief(t *testing.T) {
	c := checks.Check{Name: "test-check", Prompt: "check for foo"}
	got := checkPrompt("some diff", c)
	if strings.Contains(got, "project brief") {
		t.Errorf("checkPrompt() = %q, must never embed project-brief content: checks always run without one", got)
	}
}

// llmStub starts an httptest.Server that records the prompt sent in the
// single outgoing chat message and replies with a fixed assistant message,
// streamed as a one-chunk server-sent event stream.
func llmStub(t *testing.T, onRequest func(prompt string)) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("llmStub: decode request body: %v", err)
		}
		if onRequest != nil && len(req.Messages) > 0 {
			onRequest(req.Messages[0].Content)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"an explanation\"}}]}\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
}

// configureForTest points package config at a fresh temp config directory
// with baseURL and model saved, and makes an API key available via the
// MDIFF_OPENAI_API_KEY env-var fallback config.GetAPIKey uses when no OS
// keyring is reachable, so Service.Explain's config checks are satisfied
// without touching any real keyring.
func configureForTest(t *testing.T, baseURL string) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("MDIFF_OPENAI_API_KEY", "test-key")
	if err := config.Save(config.Config{BaseURL: baseURL, Model: "test-model"}); err != nil {
		t.Fatalf("config.Save: %v", err)
	}
}

func TestExplain_WithProjectBrief_EmbedsInOutgoingPrompt(t *testing.T) {
	dir, repo := initRepo(t)
	runGitIn(t, dir, "commit", "--allow-empty", "-q", "-m", "initial")
	writeFile(t, dir, "a.txt", "a content\n")

	var gotPrompt string
	ts := llmStub(t, func(prompt string) { gotPrompt = prompt })
	defer ts.Close()
	configureForTest(t, ts.URL)

	s := NewService()
	if _, err := s.Explain(context.Background(), repo, "", []string{"a.txt"}, "this project is a CLI tool written in Go", nil); err != nil {
		t.Fatalf("Explain: %v", err)
	}

	if !strings.Contains(gotPrompt, "this project is a CLI tool written in Go") {
		t.Errorf("outgoing prompt = %q, want it to embed the project brief", gotPrompt)
	}
}

func TestExplain_BriefOnAndOffAreDistinctCacheEntries(t *testing.T) {
	dir, repo := initRepo(t)
	runGitIn(t, dir, "commit", "--allow-empty", "-q", "-m", "initial")
	writeFile(t, dir, "a.txt", "a content\n")

	var callCount int
	ts := llmStub(t, func(string) { callCount++ })
	defer ts.Close()
	configureForTest(t, ts.URL)

	s := NewService()
	if _, err := s.Explain(context.Background(), repo, "", []string{"a.txt"}, "", nil); err != nil {
		t.Fatalf("Explain (no brief): %v", err)
	}
	if _, err := s.Explain(context.Background(), repo, "", []string{"a.txt"}, "a project brief", nil); err != nil {
		t.Fatalf("Explain (with brief): %v", err)
	}

	if callCount != 2 {
		t.Fatalf("LLM backend received %d requests, want 2: toggling the project brief for the same diff must not be served from the other state's cache entry", callCount)
	}
}

func TestExplain_StreamsDeltasAndCacheHitEmitsNone(t *testing.T) {
	dir, repo := initRepo(t)
	runGitIn(t, dir, "commit", "--allow-empty", "-q", "-m", "initial")
	writeFile(t, dir, "a.txt", "a content\n")

	ts := llmStub(t, nil)
	defer ts.Close()
	configureForTest(t, ts.URL)

	s := NewService()
	var deltas []string
	got, err := s.Explain(context.Background(), repo, "", []string{"a.txt"}, "", func(chunk string) {
		deltas = append(deltas, chunk)
	})
	if err != nil {
		t.Fatalf("Explain: %v", err)
	}
	if want := []string{"an explanation"}; !slices.Equal(deltas, want) {
		t.Errorf("onDelta received %q, want %q", deltas, want)
	}
	if got != "an explanation" {
		t.Errorf("Explain() = %q, want %q", got, "an explanation")
	}

	deltas = nil
	if _, err := s.Explain(context.Background(), repo, "", []string{"a.txt"}, "", func(chunk string) {
		deltas = append(deltas, chunk)
	}); err != nil {
		t.Fatalf("Explain (cached): %v", err)
	}
	if len(deltas) != 0 {
		t.Errorf("cache hit emitted %q, want no deltas at all", deltas)
	}
}
