package diffparse

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

// gitDiffIn runs `git diff <args...>` inside dir and returns its stdout.
func gitDiffIn(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"diff"}, args...)...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git diff %s failed: %v", strings.Join(args, " "), err)
	}
	return string(out)
}

// initRepo creates a fresh git repo in a temp dir with a usable identity for
// commits. Unlike gitdiff's tests, it does not chdir the process: every git
// invocation here explicitly targets dir via cmd.Dir.
func initRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runGitIn(t, dir, "init", "-q")
	runGitIn(t, dir, "config", "user.email", "test@example.com")
	runGitIn(t, dir, "config", "user.name", "Test User")
	return dir
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

func TestParse_ModifiedFile(t *testing.T) {
	dir := initRepo(t)
	writeFile(t, dir, "file.txt", "unchanged line\nold line\nother unchanged\n")
	runGitIn(t, dir, "add", "file.txt")
	runGitIn(t, dir, "commit", "-q", "-m", "initial")

	writeFile(t, dir, "file.txt", "unchanged line\nnew line\nother unchanged\n")

	raw := gitDiffIn(t, dir, "--", "file.txt")

	fd, err := Parse(raw)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if fd.Path != "file.txt" {
		t.Errorf("Path = %q, want %q", fd.Path, "file.txt")
	}
	if len(fd.Hunks) != 1 {
		t.Fatalf("len(Hunks) = %d, want 1", len(fd.Hunks))
	}
	hunk := fd.Hunks[0]
	if !strings.HasPrefix(hunk.Header, "@@ ") {
		t.Errorf("Header = %q, want prefix %q", hunk.Header, "@@ ")
	}

	want := []Line{
		{Type: Context, Content: "unchanged line", OldNum: 1, NewNum: 1},
		{Type: Removed, Content: "old line", OldNum: 2, Highlight: Span{Start: 0, End: 3}},
		{Type: Added, Content: "new line", NewNum: 2, Highlight: Span{Start: 0, End: 3}},
		{Type: Context, Content: "other unchanged", OldNum: 3, NewNum: 3},
	}
	if len(hunk.Lines) != len(want) {
		t.Fatalf("len(Lines) = %d, want %d; got %+v", len(hunk.Lines), len(want), hunk.Lines)
	}
	for i, w := range want {
		if hunk.Lines[i] != w {
			t.Errorf("Lines[%d] = %+v, want %+v", i, hunk.Lines[i], w)
		}
	}
}

func TestParse_RenamedNoContentChange(t *testing.T) {
	dir := initRepo(t)
	writeFile(t, dir, "old.txt", "same content\n")
	runGitIn(t, dir, "add", "old.txt")
	runGitIn(t, dir, "commit", "-q", "-m", "initial")

	runGitIn(t, dir, "mv", "old.txt", "renamed.txt")

	// A plain `git diff -- <path>` shows nothing here since git mv stages
	// both the index and the working tree. Comparing against HEAD (with
	// both the old and new paths given, so git can match them up) is what
	// produces the rename-header shape with no hunks.
	raw := gitDiffIn(t, dir, "HEAD", "--", "old.txt", "renamed.txt")
	if !strings.Contains(raw, "rename from old.txt") {
		t.Fatalf("test setup: expected a rename diff, got:\n%s", raw)
	}

	fd, err := Parse(raw)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if fd.Path != "renamed.txt" {
		t.Errorf("Path = %q, want %q", fd.Path, "renamed.txt")
	}
	if len(fd.Hunks) != 0 {
		t.Errorf("len(Hunks) = %d, want 0; got %+v", len(fd.Hunks), fd.Hunks)
	}
}

func TestParse_BinaryFile(t *testing.T) {
	dir := initRepo(t)
	writeFile(t, dir, "img.bin", "\x00\x01\x02binary")
	runGitIn(t, dir, "add", "img.bin")
	runGitIn(t, dir, "commit", "-q", "-m", "initial")

	writeFile(t, dir, "img.bin", "\x00\x01\x02changed")

	raw := gitDiffIn(t, dir, "--", "img.bin")
	if !strings.Contains(raw, "Binary files") {
		t.Fatalf("test setup: expected a binary diff, got:\n%s", raw)
	}

	fd, err := Parse(raw)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if fd.Path != "img.bin" {
		t.Errorf("Path = %q, want %q", fd.Path, "img.bin")
	}
	if len(fd.Hunks) != 0 {
		t.Errorf("len(Hunks) = %d, want 0; got %+v", len(fd.Hunks), fd.Hunks)
	}
}

func TestParse_NoTrailingNewline(t *testing.T) {
	dir := initRepo(t)
	writeFile(t, dir, "nofinalnl.txt", "line one\nline two")
	runGitIn(t, dir, "add", "nofinalnl.txt")
	runGitIn(t, dir, "commit", "-q", "-m", "initial")

	writeFile(t, dir, "nofinalnl.txt", "line one changed\nline two")

	raw := gitDiffIn(t, dir, "--", "nofinalnl.txt")
	if !strings.Contains(raw, "\\ No newline at end of file") {
		t.Fatalf("test setup: expected a no-newline marker, got:\n%s", raw)
	}

	fd, err := Parse(raw)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if fd.Path != "nofinalnl.txt" {
		t.Errorf("Path = %q, want %q", fd.Path, "nofinalnl.txt")
	}
	if len(fd.Hunks) != 1 {
		t.Fatalf("len(Hunks) = %d, want 1", len(fd.Hunks))
	}

	want := []Line{
		{Type: Removed, Content: "line one", OldNum: 1},
		{Type: Added, Content: "line one changed", NewNum: 1, Highlight: Span{Start: 8, End: 16}},
		{Type: Context, Content: "line two", OldNum: 2, NewNum: 2},
	}
	got := fd.Hunks[0].Lines
	if len(got) != len(want) {
		t.Fatalf("len(Lines) = %d, want %d; got %+v", len(got), len(want), got)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("Lines[%d] = %+v, want %+v", i, got[i], w)
		}
	}
}

func TestParse_NoDiff(t *testing.T) {
	fd, err := Parse("")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if fd.Path != "" || len(fd.Hunks) != 0 {
		t.Errorf("Parse(\"\") = %+v, want zero value", fd)
	}
}

func TestParse_Malformed(t *testing.T) {
	if _, err := Parse("this is not a diff\njust some text\n"); err == nil {
		t.Fatal("Parse: expected an error for malformed input, got nil")
	}
}

func TestParse_LineNumbersResumeAcrossHunks(t *testing.T) {
	dir := initRepo(t)
	writeFile(t, dir, "file.txt", "l1\nl2\nl3\nl4\nl5\nl6\nl7\nl8\nl9\nl10\nl11\nl12\nl13\nl14\n")
	runGitIn(t, dir, "add", "file.txt")
	runGitIn(t, dir, "commit", "-q", "-m", "initial")

	writeFile(t, dir, "file.txt", "l1\nL2\nl3\nl4\nl5\nl6\nl7\nl8\nl9\nl10\nl11\nL12\nl13\nl14\n")

	raw := gitDiffIn(t, dir, "--", "file.txt")

	fd, err := Parse(raw)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(fd.Hunks) != 2 {
		t.Fatalf("len(Hunks) = %d, want 2", len(fd.Hunks))
	}
	second := fd.Hunks[1]
	if second.Lines[0].OldNum != 9 || second.Lines[0].NewNum != 9 {
		t.Errorf("second hunk first line = %+v, want OldNum 9, NewNum 9", second.Lines[0])
	}
	var removed, added Line
	for _, l := range second.Lines {
		switch l.Type {
		case Removed:
			removed = l
		case Added:
			added = l
		}
	}
	if removed.OldNum != 12 || added.NewNum != 12 {
		t.Errorf("changed pair = removed %+v, added %+v, want both at line 12", removed, added)
	}
}

func TestAddIntralineHighlights_PureInsertionGetsNoSpan(t *testing.T) {
	lines := []Line{
		{Type: Context, Content: "before", OldNum: 1, NewNum: 1},
		{Type: Added, Content: "inserted", NewNum: 2},
		{Type: Context, Content: "after", OldNum: 2, NewNum: 3},
	}
	addIntralineHighlights(lines)
	if lines[1].Highlight != (Span{}) {
		t.Errorf("pure insertion Highlight = %+v, want zero Span", lines[1].Highlight)
	}
}

func TestChangedSpans(t *testing.T) {
	for _, tc := range []struct {
		name             string
		old, new         string
		wantOld, wantNew Span
	}{
		{"identical", "same", "same", Span{}, Span{}},
		{"prefix kept", "line one", "line one changed", Span{}, Span{8, 16}},
		{"suffix kept", "old line", "new line", Span{0, 3}, Span{0, 3}},
		{"middle", "a x b", "a y b", Span{2, 3}, Span{2, 3}},
		{"unicode", "héllo wörld", "héllo welt", Span{7, 11}, Span{7, 10}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gotOld, gotNew := changedSpans(tc.old, tc.new)
			if gotOld != tc.wantOld || gotNew != tc.wantNew {
				t.Errorf("changedSpans(%q, %q) = (%v, %v), want (%v, %v)",
					tc.old, tc.new, gotOld, gotNew, tc.wantOld, tc.wantNew)
			}
		})
	}
}
