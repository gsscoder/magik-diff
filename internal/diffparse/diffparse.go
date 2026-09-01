// Package diffparse parses the raw unified diff text produced by
// internal/gitdiff (equivalent to `git diff -- <path>`) into structured
// data that is easy for a frontend to render as +/- line classes.
package diffparse

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// LineType classifies a single line within a hunk.
type LineType string

const (
	Context LineType = "context"
	Added   LineType = "added"
	Removed LineType = "removed"
)

// Span is a half-open rune range [Start, End) within a Line's Content.
// The zero value (Start == End) means "no span".
type Span struct {
	Start int
	End   int
}

// Line is one line of a hunk's body, with its leading +/-/space marker
// stripped off. OldNum and NewNum are the 1-based line numbers in the old
// and new revision of the file; OldNum is 0 for added lines, NewNum is 0
// for removed lines. Highlight marks the exact changed substring for
// word-level intraline highlighting; it is the zero Span when the line has
// no paired counterpart or nothing differs inside it.
type Line struct {
	Type      LineType
	Content   string
	OldNum    int
	NewNum    int
	Highlight Span
}

// Hunk is one `@@ -l,s +l,s @@` section of a diff, with its header line and
// the ordered lines that follow it.
type Hunk struct {
	Header string
	Lines  []Line
}

// FileDiff is the parsed form of the raw unified diff text for a single
// file, as returned by gitdiff.FileDiff.
type FileDiff struct {
	Path  string
	Hunks []Hunk
}

// gitDiffHeader matches the "diff --git a/<old> b/<new>" line that starts
// every per-file section of `git diff` output.
var gitDiffHeader = regexp.MustCompile(`^diff --git a/(.*) b/(.*)$`)

// renameTo matches the "rename to <path>" line emitted for a renamed file.
var renameTo = regexp.MustCompile(`^rename to (.*)$`)

// binaryFiles matches the "Binary files a/<old> and b/<new> differ" line
// emitted instead of hunks when the file is detected as binary.
var binaryFiles = regexp.MustCompile(`^Binary files a/(.*) and b/(.*) differ$`)

// hunkHeader matches an "@@ -l,s +l,s @@" line, capturing the old and new
// start lines and allowing for the optional trailing function-context text
// git appends after the closing "@@".
var hunkHeader = regexp.MustCompile(`^@@ -(\d+)(?:,\d+)? \+(\d+)(?:,\d+)? @@`)

// Parse parses raw unified diff text for a single file, as returned by
// gitdiff.FileDiff, into a FileDiff.
//
// It tolerates the real-world shapes `git diff` produces beyond a plain
// modified-file diff: a rename with no content change (no hunks at all), a
// binary file diff ("Binary files ... differ" instead of hunks), and
// "\ No newline at end of file" markers. For those, it returns as much
// structure as it reasonably can rather than an error. An error is returned
// only when raw does not look like git diff output at all.
func Parse(raw string) (FileDiff, error) {
	if strings.TrimSpace(raw) == "" {
		return FileDiff{}, nil
	}

	lines := strings.Split(raw, "\n")

	m := gitDiffHeader.FindStringSubmatch(lines[0])
	if m == nil {
		return FileDiff{}, fmt.Errorf("diffparse: expected first line to be a \"diff --git\" header, got %q", lines[0])
	}
	fd := FileDiff{Path: m[2]}

	var hunk *Hunk
	oldNum, newNum := 0, 0
	for _, line := range lines[1:] {
		m := hunkHeader.FindStringSubmatch(line)
		switch {
		case m != nil:
			oldNum, _ = strconv.Atoi(m[1])
			newNum, _ = strconv.Atoi(m[2])
			fd.Hunks = append(fd.Hunks, Hunk{Header: line})
			hunk = &fd.Hunks[len(fd.Hunks)-1]

		case hunk != nil && strings.HasPrefix(line, "\\"):
			// "\ No newline at end of file" - not a content line, ignore.

		case hunk != nil && strings.HasPrefix(line, "+"):
			hunk.Lines = append(hunk.Lines, Line{Type: Added, Content: line[1:], NewNum: newNum})
			newNum++

		case hunk != nil && strings.HasPrefix(line, "-"):
			hunk.Lines = append(hunk.Lines, Line{Type: Removed, Content: line[1:], OldNum: oldNum})
			oldNum++

		case hunk != nil && strings.HasPrefix(line, " "):
			hunk.Lines = append(hunk.Lines, Line{Type: Context, Content: line[1:], OldNum: oldNum, NewNum: newNum})
			oldNum++
			newNum++

		case hunk == nil:
			// Still in the per-file preamble (mode/index/rename/binary
			// lines) before any hunk. Recover the path from whichever
			// line names it most precisely, in case the "diff --git"
			// line's own heuristic split was wrong (e.g. paths
			// containing " b/").
			if rm := renameTo.FindStringSubmatch(line); rm != nil {
				fd.Path = rm[1]
			} else if bm := binaryFiles.FindStringSubmatch(line); bm != nil {
				fd.Path = bm[2]
			}
		}
	}

	for i := range fd.Hunks {
		addIntralineHighlights(fd.Hunks[i].Lines)
	}

	return fd, nil
}

// addIntralineHighlights sets the Highlight span of every removed/added
// line that has a paired counterpart. Pairing follows git's own emission
// order: a maximal run of removed lines immediately followed by a run of
// added lines forms a group, and lines are paired by position within the
// group. Unpaired lines (pure deletions, pure insertions, extra lines in
// the longer run) keep the zero Span.
func addIntralineHighlights(lines []Line) {
	i := 0
	for i < len(lines) {
		if lines[i].Type != Removed {
			i++
			continue
		}
		rStart := i
		for i < len(lines) && lines[i].Type == Removed {
			i++
		}
		aStart := i
		for i < len(lines) && lines[i].Type == Added {
			i++
		}
		removed, added := lines[rStart:aStart], lines[aStart:i]
		for k := 0; k < len(removed) && k < len(added); k++ {
			oldSpan, newSpan := changedSpans(removed[k].Content, added[k].Content)
			removed[k].Highlight = oldSpan
			added[k].Highlight = newSpan
		}
	}
}

// changedSpans locates the changed substring in each of an old/new line
// pair by trimming their common prefix and suffix (runes), returning the
// remaining middle range of each. An empty result (identical lines, or one
// line fully contained in the other) is normalized to the zero Span.
func changedSpans(oldLine, newLine string) (Span, Span) {
	o, n := []rune(oldLine), []rune(newLine)
	prefix := 0
	for prefix < len(o) && prefix < len(n) && o[prefix] == n[prefix] {
		prefix++
	}
	suffix := 0
	for suffix < len(o)-prefix && suffix < len(n)-prefix && o[len(o)-1-suffix] == n[len(n)-1-suffix] {
		suffix++
	}
	oldSpan, newSpan := Span{prefix, len(o) - suffix}, Span{prefix, len(n) - suffix}
	if oldSpan.Start == oldSpan.End {
		oldSpan = Span{}
	}
	if newSpan.Start == newSpan.End {
		newSpan = Span{}
	}
	return oldSpan, newSpan
}
