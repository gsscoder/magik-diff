// Package diffparse parses the raw unified diff text produced by
// internal/gitdiff (equivalent to `git diff -- <path>`) into structured
// data that is easy for a frontend to render as +/- line classes.
package diffparse

import (
	"fmt"
	"regexp"
	"strings"
)

// LineType classifies a single line within a hunk.
type LineType string

const (
	Context LineType = "context"
	Added   LineType = "added"
	Removed LineType = "removed"
)

// Line is one line of a hunk's body, with its leading +/-/space marker
// stripped off.
type Line struct {
	Type    LineType
	Content string
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

// hunkHeader matches an "@@ -l,s +l,s @@" line, allowing for the optional
// trailing function-context text git appends after the closing "@@".
var hunkHeader = regexp.MustCompile(`^@@ -\d+(?:,\d+)? \+\d+(?:,\d+)? @@`)

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
	for _, line := range lines[1:] {
		switch {
		case hunkHeader.MatchString(line):
			fd.Hunks = append(fd.Hunks, Hunk{Header: line})
			hunk = &fd.Hunks[len(fd.Hunks)-1]

		case hunk != nil && strings.HasPrefix(line, "\\"):
			// "\ No newline at end of file" - not a content line, ignore.

		case hunk != nil && strings.HasPrefix(line, "+"):
			hunk.Lines = append(hunk.Lines, Line{Type: Added, Content: line[1:]})

		case hunk != nil && strings.HasPrefix(line, "-"):
			hunk.Lines = append(hunk.Lines, Line{Type: Removed, Content: line[1:]})

		case hunk != nil && strings.HasPrefix(line, " "):
			hunk.Lines = append(hunk.Lines, Line{Type: Context, Content: line[1:]})

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

	return fd, nil
}
