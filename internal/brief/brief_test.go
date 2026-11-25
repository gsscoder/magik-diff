package brief

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"

	"mdiff/internal/config"
)

// writeFile writes content to name under dir, creating parent directories
// as needed.
func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

func TestStorePath_DoesNotCollideAcrossSeparatorBoundaries(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	base := t.TempDir()
	root1 := filepath.Join(base, "a", "bc")
	root2 := filepath.Join(base, "ab", "c")

	p1, err := storePath(root1)
	if err != nil {
		t.Fatalf("storePath(root1) error = %v", err)
	}
	p2, err := storePath(root2)
	if err != nil {
		t.Fatalf("storePath(root2) error = %v", err)
	}

	if p1 == p2 {
		t.Fatalf("storePath collided for distinct roots %q and %q: both = %q", root1, root2, p1)
	}
}

func TestStorePath_CaseInsensitiveOnWindows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("case-insensitive path collision only applies on Windows")
	}
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	root := t.TempDir()

	p1, err := storePath(root)
	if err != nil {
		t.Fatalf("storePath(root) error = %v", err)
	}
	p2, err := storePath(strings.ToUpper(root))
	if err != nil {
		t.Fatalf("storePath(upper-cased root) error = %v", err)
	}

	if p1 != p2 {
		t.Fatalf("storePath differed for differently-cased spellings of the same repo: %q vs %q", p1, p2)
	}
}

func TestSaveThenLoadRoundTrip(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	root := t.TempDir()

	want := Brief{
		RepoPath: root,
		Files:    []string{"README.md", "docs/guide.md"},
		Text:     "- a factual project brief",
	}
	if err := Save(want); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	got, ok, err := Load(root)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !ok {
		t.Fatal("Load() ok = false, want true")
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Load() = %+v, want %+v", got, want)
	}
}

func TestLoad_MissingFileReturnsNotStored(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	root := t.TempDir()

	got, ok, err := Load(root)
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}
	if ok {
		t.Fatal("Load() ok = true, want false for a repo with no stored brief")
	}
	if !reflect.DeepEqual(got, Brief{}) {
		t.Fatalf("Load() = %+v, want zero-value Brief", got)
	}
}

func TestGetState(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	root := t.TempDir()

	state, err := GetState(root)
	if err != nil {
		t.Fatalf("GetState() error = %v", err)
	}
	if !reflect.DeepEqual(state, State{}) {
		t.Errorf("GetState() = %+v, want zero-value State before any Save", state)
	}

	stored := Brief{RepoPath: root, Files: []string{"README.md"}, Text: "- brief text"}
	if err := Save(stored); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	state, err = GetState(root)
	if err != nil {
		t.Fatalf("GetState() error = %v", err)
	}
	if want := (State{Stored: true, Brief: stored}); !reflect.DeepEqual(state, want) {
		t.Errorf("GetState() = %+v, want %+v", state, want)
	}
}

func TestCheckSelection(t *testing.T) {
	docs := []DocFile{{Path: "README.md"}, {Path: "docs/a.md"}}
	for i := range maxFiles {
		docs = append(docs, DocFile{Path: fmt.Sprintf("doc%d.md", i)})
	}

	tests := map[string]struct {
		files []string
		want  string
	}{
		"listed docs":          {files: []string{"README.md", "docs/a.md"}, want: ""},
		"at most the file cap": {files: []string{"doc0.md", "doc1.md", "doc2.md", "doc3.md", "doc4.md"}, want: ""},
		"empty":                {files: nil, want: "Select at least one file."},
		"over the file cap":    {files: []string{"README.md", "doc0.md", "doc1.md", "doc2.md", "doc3.md", "doc4.md"}, want: "Select at most 5 files."},
		"not a doc":            {files: []string{"README.md", "src/main.go"}, want: "Not a documentation file of this repo: src/main.go"},
		"outside the repo":     {files: []string{"../README.md"}, want: "Not a documentation file of this repo: ../README.md"},
		"missing doc":          {files: []string{"docs/missing.md"}, want: "Not a documentation file of this repo: docs/missing.md"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			err := checkSelection(docs, tt.files)
			got := ""
			if err != nil {
				got = err.Error()
			}
			if got != tt.want {
				t.Errorf("checkSelection(%q) error = %q, want %q", tt.files, got, tt.want)
			}
		})
	}
}

func TestCheckBudget_ErrorNamesTheLimit(t *testing.T) {
	if err := checkBudget(strings.Repeat("a", maxPromptTokens*4+3)); err != nil {
		t.Errorf("checkBudget(at the limit) error = %v, want nil", err)
	}
	err := checkBudget(strings.Repeat("a", maxPromptTokens*4+4))
	want := "The selected files are too large for one request (about 100001 tokens, the limit is 100000). Select fewer files."
	if err == nil || err.Error() != want {
		t.Errorf("checkBudget(over the limit) error = %v, want %q", err, want)
	}
}

// llmStub is an httptest.Server that records the prompt of every request
// and replies with the next of its replies, streamed as a one-chunk
// server-sent event stream.
type llmStub struct {
	*httptest.Server
	mu      sync.Mutex
	prompts []string
}

func newLLMStub(t *testing.T, replies ...string) *llmStub {
	t.Helper()
	stub := &llmStub{}
	stub.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.Messages) != 1 {
			t.Errorf("llmStub: want one message, got %+v (decode error %v)", req, err)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		stub.mu.Lock()
		call := len(stub.prompts)
		stub.prompts = append(stub.prompts, req.Messages[0].Content)
		stub.mu.Unlock()
		if call >= len(replies) {
			t.Errorf("llmStub: unexpected request %d", call+1)
			http.Error(w, "no reply left", http.StatusInternalServerError)
			return
		}
		content, err := json.Marshal(replies[call])
		if err != nil {
			t.Errorf("llmStub: marshal reply: %v", err)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":%s}}]}\n\n", content)
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	t.Cleanup(stub.Close)
	return stub
}

// requests returns the prompts received so far.
func (s *llmStub) requests() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.prompts...)
}

// configureForTest points package config at a fresh temp config directory
// with baseURL and a model saved, and makes an API key available via the
// MDIFF_OPENAI_API_KEY env-var fallback config.GetAPIKey uses when no OS
// keyring is reachable.
func configureForTest(t *testing.T, baseURL string) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("MDIFF_OPENAI_API_KEY", "test-key")
	if err := config.Save(config.Config{BaseURL: baseURL, Model: "test-model"}); err != nil {
		t.Fatalf("config.Save: %v", err)
	}
}

func TestGenerate_InvalidRequestFailsBeforeAnyLLMCall(t *testing.T) {
	dir := initRepo(t)
	writeFile(t, dir, "README.md", "About the app")
	writeFile(t, dir, "main.go", "package main")
	writeFile(t, dir, "docs/big.md", strings.Repeat("a", maxPromptTokens*4))
	bigPrompt := prompt(overviewPrompt, docBlock("docs/big.md", strings.Repeat("a", maxPromptTokens*4)))

	tests := map[string]struct {
		files []string
		want  string
	}{
		"empty selection": {files: nil, want: "Select at least one file."},
		"too many files":  {files: []string{"a.md", "b.md", "c.md", "d.md", "e.md", "f.md"}, want: "Select at most 5 files."},
		"not a doc":       {files: []string{"README.md", "main.go"}, want: "Not a documentation file of this repo: main.go"},
		"too large": {
			files: []string{"docs/big.md"},
			want:  fmt.Sprintf("The selected files are too large for one request (about %d tokens, the limit is 100000). Select fewer files.", len(bigPrompt)/4),
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			stub := newLLMStub(t)
			configureForTest(t, stub.URL)

			_, err := Generate(context.Background(), dir, tt.files)
			if err == nil || err.Error() != tt.want {
				t.Errorf("Generate() error = %v, want %q", err, tt.want)
			}
			if got := stub.requests(); len(got) != 0 {
				t.Errorf("LLM received %d requests, want 0", len(got))
			}
			if _, ok, _ := Load(dir); ok {
				t.Error("a brief was stored after a failed Generate")
			}
		})
	}
}

func TestGenerate_RetriesAnUnusableOverviewOnceThenWritesTheBrief(t *testing.T) {
	dir := initRepo(t)
	writeFile(t, dir, "README.md", "About the app")
	writeFile(t, dir, "docs/guide.md", "How it works")
	overview := strings.TrimSpace(validOverview())
	stub := newLLMStub(t, "A plain paragraph.", overview, "- one.\n- two...")
	configureForTest(t, stub.URL)
	files := []string{"README.md", "docs/guide.md"}

	got, err := Generate(context.Background(), dir, files)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	want := Brief{RepoPath: dir, Files: files, Text: "- one\n- two..."}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Generate() = %+v, want %+v", got, want)
	}
	if stored, ok, err := Load(dir); err != nil || !ok || !reflect.DeepEqual(stored, want) {
		t.Errorf("Load() = %+v, %v, %v; want %+v, true, nil", stored, ok, err, want)
	}

	requests := stub.requests()
	if len(requests) != 3 {
		t.Fatalf("LLM received %d requests, want 3 (overview, retry, brief)", len(requests))
	}
	wantOverviewPrompt := prompt(overviewPrompt,
		"<<<FILE README.md>>>\nAbout the app\n<<<END FILE>>>\n\n<<<FILE docs/guide.md>>>\nHow it works\n<<<END FILE>>>\n")
	for i, p := range requests[:2] {
		if p != wantOverviewPrompt {
			t.Errorf("request %d = %q, want %q", i+1, p, wantOverviewPrompt)
		}
	}
	if want := prompt(briefPrompt, "<<<OVERVIEW>>>\n"+overview+"\n<<<END OVERVIEW>>>\n"); requests[2] != want {
		t.Errorf("brief request = %q, want %q", requests[2], want)
	}
}

func TestGenerate_NoUsableOverviewFails(t *testing.T) {
	tests := map[string]struct {
		replies      []string
		wantRequests int
	}{
		"sentinel is not retried":    {replies: []string{"INSUFFICIENT_SOURCES"}, wantRequests: 1},
		"second unusable answer":     {replies: []string{"A plain paragraph.", "Another one."}, wantRequests: 2},
		"sentinel after an unusable": {replies: []string{"A plain paragraph.", "```\nINSUFFICIENT_SOURCES\n```"}, wantRequests: 2},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			dir := initRepo(t)
			writeFile(t, dir, "README.md", "Contribution rules only")
			stub := newLLMStub(t, tt.replies...)
			configureForTest(t, stub.URL)

			_, err := Generate(context.Background(), dir, []string{"README.md"})
			if err == nil || err.Error() != notDescribed {
				t.Errorf("Generate() error = %v, want %q", err, notDescribed)
			}
			if got := len(stub.requests()); got != tt.wantRequests {
				t.Errorf("LLM received %d requests, want %d", got, tt.wantRequests)
			}
			if _, ok, _ := Load(dir); ok {
				t.Error("a brief was stored after a failed Generate")
			}
		})
	}
}
