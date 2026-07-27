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

// Commit describes one commit in the repository history.
type Commit struct {
	Hash    string
	Author  string
	Date    string
	Subject string
}

// RecentCommits returns up to count commits from `git log`, newest first,
// skipping the first skip commits so the caller can page through history.
// A repository with no commits yet yields an empty list, not an error.
func RecentCommits(skip, count int) ([]Commit, error) {
	out, err := runGit("log",
		fmt.Sprintf("--skip=%d", skip),
		fmt.Sprintf("--max-count=%d", count),
		"--date=short",
		"--format=%H%x1f%an%x1f%ad%x1f%s%x1e",
	)
	if err != nil {
		if strings.Contains(err.Error(), "does not have any commits yet") {
			return []Commit{}, nil
		}
		return nil, err
	}
	return parseLog(out), nil
}

// CommitFiles lists every path changed by the given commit, equivalent to
// `git diff-tree --no-commit-id --name-status <hash>`. The --root flag makes
// it work for the root commit too, listing all of its files as added.
func CommitFiles(hash string) ([]FileChange, error) {
	out, err := runGit("diff-tree", "--root", "--no-commit-id", "--name-status", "-z", "-r", "-M", hash)
	if err != nil {
		return nil, err
	}
	return parseNameStatus(out), nil
}

// CommitFileDiff returns the raw unified diff text for path as changed by
// the given commit, equivalent to `git show --format= <hash> -- <path>`.
func CommitFileDiff(hash, path string) (string, error) {
	out, err := runGit("show", "--format=", hash, "--", path)
	if err != nil {
		return "", err
	}
	return string(out), nil
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
	hideConsole(cmd)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return stdout.Bytes(), nil
}

// parseLog parses the output of `git log` with fields separated by %x1f
// (unit separator) and records terminated by %x1e (record separator).
// Field order: hash, author, date, subject.
func parseLog(out []byte) []Commit {
	commits := []Commit{}
	for _, record := range strings.Split(string(out), "\x1e") {
		fields := strings.Split(strings.TrimSpace(record), "\x1f")
		if len(fields) != 4 {
			continue
		}
		commits = append(commits, Commit{
			Hash:    fields[0],
			Author:  fields[1],
			Date:    fields[2],
			Subject: fields[3],
		})
	}
	return commits
}

// parseNameStatus parses the NUL-delimited output of
// `git diff-tree --name-status -z`. Each record is a status token ("M",
// "A", "D", "R100", ...) followed by one path, or two paths (original then
// current) for renames and copies.
func parseNameStatus(out []byte) []FileChange {
	tokens := strings.Split(string(out), "\x00")
	changes := []FileChange{}
	for i := 0; i < len(tokens); {
		status := tokens[i]
		i++
		if status == "" {
			continue
		}
		if status[0] == 'R' || status[0] == 'C' {
			if i+1 >= len(tokens) {
				break
			}
			changes = append(changes, FileChange{Path: tokens[i+1], OrigPath: tokens[i], Type: Renamed})
			i += 2
			continue
		}
		if i >= len(tokens) {
			break
		}
		changes = append(changes, FileChange{Path: tokens[i], Type: classifyStatus(status[0])})
		i++
	}
	return changes
}

// classifyStatus maps a single-letter status from `git diff-tree
// --name-status` to a ChangeType, in order of precedence: renamed, deleted,
// added, modified.
func classifyStatus(status byte) ChangeType {
	switch status {
	case 'R', 'C':
		return Renamed
	case 'D':
		return Deleted
	case 'A':
		return Added
	default:
		return Modified
	}
}

// parsePorcelain parses the NUL-delimited output of `git status --porcelain=v1 -z`.
//
// Each record is "XY PATH", NUL-terminated, where X is the index status and
// Y is the working-tree status. Renamed and copied entries carry an
// additional NUL-terminated ORIG_PATH record immediately after.
func parsePorcelain(out []byte) []FileChange {
	tokens := strings.Split(string(out), "\x00")
	changes := []FileChange{}
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
