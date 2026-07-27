// Package gitdiff shells out to the real git binary to inspect the working
// tree of the repo the current process is running in. It always targets the
// current working directory's repo (cwd-only, no repo-path parameter, no
// multi-repo support) and never runs a git command that writes to the repo.
package gitdiff

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

// ChangeType classifies how a path differs from HEAD in the working tree.
type ChangeType string

const (
	Modified ChangeType = "modified"
	Added    ChangeType = "added"
	Deleted  ChangeType = "deleted"
	Renamed  ChangeType = "renamed"
)

// FileChange describes one changed path in the working tree.
type FileChange struct {
	// Path is the current path of the file.
	Path string
	// OrigPath is the previous path, set only when Type is Renamed.
	OrigPath string
	Type     ChangeType
}

// ChangedFiles lists every changed file in the working tree of the repo
// rooted at the current working directory, covering both staged and
// unstaged changes as well as untracked files, analogous to `git status`.
func ChangedFiles() ([]FileChange, error) {
	out, err := runGit("status", "--porcelain=v1", "-z")
	if err != nil {
		return nil, err
	}
	return parsePorcelain(out), nil
}

// FileDiff returns the raw unified diff text for path in the working tree,
// equivalent to `git diff -- <path>`. It returns an empty string if path has
// no diff against HEAD (e.g. it is unchanged, or untracked).
func FileDiff(path string) (string, error) {
	out, err := runGit("diff", "--", path)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// runGit runs git with args in the current working directory and returns its
// stdout. On failure it returns an error including git's stderr output.
func runGit(args ...string) ([]byte, error) {
	cmd := exec.Command("git", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return stdout.Bytes(), nil
}

// parsePorcelain parses the NUL-delimited output of `git status --porcelain=v1 -z`.
//
// Each record is "XY PATH", NUL-terminated, where X is the index status and
// Y is the working-tree status. Renamed and copied entries carry an
// additional NUL-terminated ORIG_PATH record immediately after.
func parsePorcelain(out []byte) []FileChange {
	tokens := strings.Split(string(out), "\x00")
	var changes []FileChange
	for i := 0; i < len(tokens); i++ {
		record := tokens[i]
		if len(record) < 3 {
			continue
		}
		x, y := record[0], record[1]
		path := record[3:]

		change := FileChange{Path: path, Type: classify(x, y)}
		if x == 'R' || x == 'C' || y == 'R' || y == 'C' {
			i++
			if i < len(tokens) {
				change.OrigPath = tokens[i]
			}
		}
		changes = append(changes, change)
	}
	return changes
}

// classify maps the index/worktree status pair from `git status --porcelain`
// to a single ChangeType, in order of precedence: renamed, deleted, added,
// modified.
func classify(x, y byte) ChangeType {
	switch {
	case x == '?' && y == '?':
		return Added
	case x == 'R' || x == 'C' || y == 'R' || y == 'C':
		return Renamed
	case x == 'D' || y == 'D':
		return Deleted
	case x == 'A' || y == 'A':
		return Added
	default:
		return Modified
	}
}
