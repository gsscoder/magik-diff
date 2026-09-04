package brief

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"unicode/utf8"
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

func TestScan_NoCandidatesReturnsNilNil(t *testing.T) {
	dir := t.TempDir()

	got, err := Scan(dir)
	if err != nil {
		t.Fatalf("Scan() error = %v, want nil", err)
	}
	if got != nil {
		t.Fatalf("Scan() = %+v, want nil", got)
	}
}

func TestScan_StopsAtThreeInPriorityOrder(t *testing.T) {
	dir := t.TempDir()
	// Four of the five candidates present, each with distinct content, so
	// dedup does not interfere with the "top 3" check.
	writeFile(t, dir, "AGENTS.md", "agents content")
	writeFile(t, dir, "CLAUDE.md", "claude content")
	writeFile(t, dir, ".github/copilot-instructions.md", "copilot content")
	writeFile(t, dir, "GEMINI.md", "gemini content")

	got, err := Scan(dir)
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}

	want := []string{"AGENTS.md", "CLAUDE.md", ".github/copilot-instructions.md"}
	if len(got) != len(want) {
		t.Fatalf("Scan() returned %d sources, want %d: %+v", len(got), len(want), got)
	}
	for i, w := range want {
		if got[i].Path != w {
			t.Errorf("Scan()[%d].Path = %q, want %q", i, got[i].Path, w)
		}
	}
}

func TestScan_DedupDropsByteIdenticalFile(t *testing.T) {
	dir := t.TempDir()
	same := "identical instructions\n"
	writeFile(t, dir, "AGENTS.md", same)
	writeFile(t, dir, "CLAUDE.md", same)
	writeFile(t, dir, "GEMINI.md", "distinct content")

	got, err := Scan(dir)
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}

	if len(got) != 2 {
		t.Fatalf("Scan() returned %d sources, want 2 (byte-identical CLAUDE.md deduped): %+v", len(got), got)
	}
	if got[0].Path != "AGENTS.md" || got[1].Path != "GEMINI.md" {
		t.Fatalf("Scan() = %+v, want [AGENTS.md, GEMINI.md]", got)
	}
	if got[0].Hash != hashBytes([]byte(same)) {
		t.Errorf("Scan()[0].Hash = %q, want sha256 of shared content", got[0].Hash)
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

func TestSourcesContent_TruncatesOnUTF8Boundary(t *testing.T) {
	dir := t.TempDir()
	prefix := fmt.Sprintf("--- %s ---\n", "AGENTS.md")
	// Pad so a 3-byte rune ("—") straddles the maxPromptInputBytes cutoff:
	// its first byte lands at the last kept position, its remaining two
	// bytes fall just past it.
	padLen := maxPromptInputBytes - len(prefix) - 1
	content := strings.Repeat("a", padLen) + "—" + strings.Repeat("a", 100)
	writeFile(t, dir, "AGENTS.md", content)

	sources, err := Scan(dir)
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}

	combined, truncated, err := sourcesContent(dir, sources)
	if err != nil {
		t.Fatalf("sourcesContent() error = %v", err)
	}
	if !truncated {
		t.Fatal("sourcesContent() truncated = false, want true")
	}
	if !utf8.ValidString(combined) {
		t.Fatal("sourcesContent() returned invalid UTF-8 at the truncation boundary")
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

func TestStale(t *testing.T) {
	base := []Source{
		{Path: "AGENTS.md", Hash: "aaa"},
		{Path: "CLAUDE.md", Hash: "bbb"},
	}

	tests := map[string]struct {
		current []Source
		want    bool
	}{
		"identical sets, different order": {
			current: []Source{
				{Path: "CLAUDE.md", Hash: "bbb"},
				{Path: "AGENTS.md", Hash: "aaa"},
			},
			want: false,
		},
		"changed hash": {
			current: []Source{
				{Path: "AGENTS.md", Hash: "ccc"},
				{Path: "CLAUDE.md", Hash: "bbb"},
			},
			want: true,
		},
		"added source": {
			current: []Source{
				{Path: "AGENTS.md", Hash: "aaa"},
				{Path: "CLAUDE.md", Hash: "bbb"},
				{Path: "GEMINI.md", Hash: "ddd"},
			},
			want: true,
		},
		"removed source": {
			current: []Source{
				{Path: "AGENTS.md", Hash: "aaa"},
			},
			want: true,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if got := Stale(base, tt.current); got != tt.want {
				t.Errorf("Stale() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSaveThenLoadRoundTrip(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	root := t.TempDir()

	want := Brief{
		RepoPath: root,
		Sources:  []Source{{Path: "AGENTS.md", Hash: "aaa"}},
		Text:     "a factual project brief",
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

func TestAcquire_NoSourcesReturnsClearError(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	dir := t.TempDir()

	_, err := Acquire(context.Background(), dir)
	if err == nil {
		t.Fatal("Acquire() error = nil, want error for a repo with no instruction files")
	}
	if err.Error() != "no AI instruction files found in this repository" {
		t.Errorf("Acquire() error = %q, want %q", err.Error(), "no AI instruction files found in this repository")
	}
}

func TestGetState(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	root := t.TempDir()
	writeFile(t, root, "AGENTS.md", "agents content")

	// No stored brief yet.
	state, err := GetState(root)
	if err != nil {
		t.Fatalf("GetState() error = %v", err)
	}
	if !state.HasSources {
		t.Error("GetState().HasSources = false, want true")
	}
	if state.Stored {
		t.Error("GetState().Stored = true, want false before any Save")
	}
	if len(state.Sources) != 1 || state.Sources[0].Path != "AGENTS.md" {
		t.Errorf("GetState().Sources = %+v, want [AGENTS.md]", state.Sources)
	}

	// Persist a brief matching the current scan: not stale.
	sources, err := Scan(root)
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	stored := Brief{RepoPath: root, Sources: sources, Text: "brief text"}
	if err := Save(stored); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	state, err = GetState(root)
	if err != nil {
		t.Fatalf("GetState() error = %v", err)
	}
	if !state.Stored {
		t.Error("GetState().Stored = false, want true after Save")
	}
	if state.Stale {
		t.Error("GetState().Stale = true, want false when sources are unchanged")
	}
	if !reflect.DeepEqual(state.Brief, stored) {
		t.Errorf("GetState().Brief = %+v, want %+v", state.Brief, stored)
	}

	// Change the source content: now stale.
	writeFile(t, root, "AGENTS.md", "changed content")
	state, err = GetState(root)
	if err != nil {
		t.Fatalf("GetState() error = %v", err)
	}
	if !state.Stale {
		t.Error("GetState().Stale = false, want true after the source file changed")
	}
}
