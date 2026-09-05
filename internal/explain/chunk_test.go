package explain

import (
	"strings"
	"testing"
)

// groupBytes returns the total diff byte length of a group, as planChunks
// budgets against.
func groupBytes(group []fileDiff) int {
	total := 0
	for _, f := range group {
		total += len(f.diff)
	}
	return total
}

// flatten concatenates every group's files back into one ordered slice, so
// tests can assert on overall ordering across group boundaries.
func flatten(groups [][]fileDiff) []fileDiff {
	var out []fileDiff
	for _, g := range groups {
		out = append(out, g...)
	}
	return out
}

func TestPlanChunks_EmptyInputReturnsNil(t *testing.T) {
	got := planChunks(nil)
	if got != nil {
		t.Errorf("planChunks(nil) = %v, want nil", got)
	}
	got = planChunks([]fileDiff{})
	if len(got) != 0 {
		t.Errorf("planChunks([]) = %v, want empty", got)
	}
}

func TestPlanChunks_EverythingFitsInOneGroup(t *testing.T) {
	files := []fileDiff{
		{path: "a.txt", diff: strings.Repeat("a", 100)},
		{path: "b.txt", diff: strings.Repeat("b", 100)},
		{path: "c.txt", diff: strings.Repeat("c", 100)},
	}
	got := planChunks(files)
	if len(got) != 1 {
		t.Fatalf("planChunks() returned %d groups, want 1", len(got))
	}
	if len(got[0]) != 3 {
		t.Errorf("planChunks() single group has %d files, want 3", len(got[0]))
	}
}

func TestPlanChunks_OversizedFileGetsItsOwnGroupUnsplit(t *testing.T) {
	oversized := fileDiff{path: "huge.txt", diff: strings.Repeat("x", maxChunkBytes+1)}
	files := []fileDiff{
		{path: "a.txt", diff: "small a"},
		oversized,
		{path: "b.txt", diff: "small b"},
	}
	got := planChunks(files)
	if len(got) != 3 {
		t.Fatalf("planChunks() returned %d groups, want 3 (a, huge alone, b)", len(got))
	}
	huge := got[1]
	if len(huge) != 1 || huge[0].path != "huge.txt" {
		t.Fatalf("planChunks() group[1] = %v, want the oversized file alone", huge)
	}
	if huge[0].diff != oversized.diff {
		t.Error("planChunks() must not truncate or alter an oversized file's diff bytes")
	}
}

func TestPlanChunks_PacksSmallFilesAgainstTheBudgetBoundary(t *testing.T) {
	// f1, f2 are 40% of the budget each (fit together, 80%); f3 is another
	// 40%, which would push the running total to 120% so it must start a
	// new group; f4 is small enough (10%) to join f3 in that new group.
	big := maxChunkBytes * 2 / 5
	small := maxChunkBytes / 10
	files := []fileDiff{
		{path: "a.txt", diff: strings.Repeat("a", big)},
		{path: "b.txt", diff: strings.Repeat("b", big)},
		{path: "c.txt", diff: strings.Repeat("c", big)},
		{path: "d.txt", diff: strings.Repeat("d", small)},
	}
	got := planChunks(files)
	if len(got) != 2 {
		t.Fatalf("planChunks() returned %d groups, want 2", len(got))
	}
	for i, g := range got {
		if groupBytes(g) > maxChunkBytes {
			t.Errorf("planChunks() group %d exceeds budget: %d bytes > %d", i, groupBytes(g), maxChunkBytes)
		}
	}
	wantPaths := [][]string{{"a.txt", "b.txt"}, {"c.txt", "d.txt"}}
	for i, g := range got {
		if len(g) != len(wantPaths[i]) {
			t.Fatalf("planChunks() group %d has %d files, want %d", i, len(g), len(wantPaths[i]))
		}
		for j, f := range g {
			if f.path != wantPaths[i][j] {
				t.Errorf("planChunks() group %d file %d = %q, want %q", i, j, f.path, wantPaths[i][j])
			}
		}
	}
}

func TestPlanChunks_PreservesInputOrderAcrossGroups(t *testing.T) {
	oversized := fileDiff{path: "huge.txt", diff: strings.Repeat("x", maxChunkBytes+1)}
	files := []fileDiff{
		{path: "a.txt", diff: "1"},
		{path: "b.txt", diff: "2"},
		oversized,
		{path: "c.txt", diff: "3"},
		{path: "d.txt", diff: "4"},
	}
	got := flatten(planChunks(files))
	if len(got) != len(files) {
		t.Fatalf("flatten(planChunks()) has %d files, want %d", len(got), len(files))
	}
	for i, f := range got {
		if f.path != files[i].path {
			t.Errorf("flatten(planChunks())[%d].path = %q, want %q (order not preserved)", i, f.path, files[i].path)
		}
	}
}
