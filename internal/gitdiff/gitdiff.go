// Package gitdiff shells out to the real git binary to inspect the working
// tree of a repo. Each Repo instance is rooted at one directory (no
// process-global cwd dependency), and it never runs a git command that
// writes to the repo.
package gitdiff

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"mdiff/internal/gitexec"
)

// Repo is a handle to a git repository rooted at a fixed directory. Every
// method issues its own git subprocess scoped to that directory, so calls
// against different Repo values never interfere with each other even when
// run concurrently.
type Repo struct {
	dir string
}

// New returns a Repo rooted at dir. dir may be "" to mean the current
// working directory, for callers that don't yet have a fixed root.
func New(dir string) *Repo {
	return &Repo{dir: dir}
}

// Dir returns the directory this Repo is rooted at.
func (r *Repo) Dir() string {
	return r.dir
}

// FindRoot resolves dir, or the nearest ancestor of dir that is a git
// working tree, to that repository's top-level directory. It returns an
// error if dir is not inside a git working tree.
func FindRoot(ctx context.Context, dir string) (string, error) {
	out, err := gitexec.Run(ctx, dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", fmt.Errorf("resolve repo root for %q: %w", dir, err)
	}
	return filepath.FromSlash(strings.TrimSpace(out)), nil
}

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
	// IsCode reports whether Path's extension is a mainstream source-code
	// language, used by the frontend to default-select files for LLM calls
	// (excluding build/config/lockfiles such as .csproj or package.json).
	IsCode bool
}

// codeExtensions holds the lowercase, dot-prefixed file extensions
// recognized as mainstream source-code languages.
var codeExtensions = map[string]struct{}{
	".go":    {},
	".cs":    {},
	".ts":    {},
	".tsx":   {},
	".js":    {},
	".jsx":   {},
	".py":    {},
	".java":  {},
	".kt":    {},
	".rs":    {},
	".c":     {},
	".cpp":   {},
	".h":     {},
	".rb":    {},
	".php":   {},
	".swift": {},
	".scala": {},
	".sql":   {},
	".sh":    {},
	".ps1":   {},
}

// isCodeFile reports whether path's extension identifies a mainstream
// source-code language, as opposed to a build/config/lockfile.
func isCodeFile(path string) bool {
	_, ok := codeExtensions[strings.ToLower(filepath.Ext(path))]
	return ok
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
func (r *Repo) RecentCommits(ctx context.Context, skip, count int) ([]Commit, error) {
	out, err := gitexec.Run(ctx, r.dir, "log",
		fmt.Sprintf("--skip=%d", skip),
		fmt.Sprintf("--max-count=%d", count),
		"--date=short",
		"--format=%H%x1f%an%x1f%ad%x1f%s%x1e",
	)
	if err != nil {
		if isUnbornHead(err) {
			return []Commit{}, nil
		}
		return nil, err
	}
	return parseLog(out), nil
}

// emptyTreeHash is git's well-known object hash for the empty tree, present
// in every repository regardless of history. Diffing against it is
// equivalent to diffing against "no commits", which lets FileDiff work in a
// freshly initialized repo where HEAD does not resolve to a commit yet.
const emptyTreeHash = "4b825dc642cb6eb9a060e54bf8d69288fbee4904"

// isUnbornHead reports whether err indicates that HEAD does not resolve to a
// commit yet, i.e. a freshly initialized repository with no commits. It
// inspects the stderr text carried by a *gitexec.Error rather than
// substring-matching Run's combined, human-oriented Error() string, since
// git reports this condition with different wording depending on the
// command (`git log` vs. `git diff HEAD`).
func isUnbornHead(err error) bool {
	var gitErr *gitexec.Error
	if !errors.As(err, &gitErr) {
		return false
	}
	return strings.Contains(gitErr.Stderr, "does not have any commits yet") ||
		strings.Contains(gitErr.Stderr, "ambiguous argument 'HEAD'") ||
		strings.Contains(gitErr.Stderr, "bad revision 'HEAD'")
}

// CurrentBranch returns the name of the currently checked-out branch,
// equivalent to `git branch --show-current`. It returns an empty string, not
// an error, when HEAD is detached.
func (r *Repo) CurrentBranch(ctx context.Context) (string, error) {
	out, err := gitexec.Run(ctx, r.dir, "branch", "--show-current")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// hashPattern matches a valid (possibly abbreviated) git commit hash. It
// rejects anything that could be parsed by git as a flag (e.g. "--output=x")
// instead of a revision, which matters because CommitFiles and
// CommitFileDiff pass hash straight into git's positional arguments.
var hashPattern = regexp.MustCompile(`^[0-9a-f]{4,64}$`)

// validateHash returns an error if hash is not a plausible git commit hash,
// so callers never pass an unvalidated string into a git command's
// positional arguments.
func validateHash(hash string) error {
	if !hashPattern.MatchString(hash) {
		return fmt.Errorf("invalid commit hash %q", hash)
	}
	return nil
}

// CommitFiles lists every path changed by the given commit, equivalent to
// `git diff-tree --no-commit-id --name-status <hash>`. The --root flag makes
// it work for the root commit too, listing all of its files as added. For a
// merge commit, -m --first-parent makes it diff against the merge's first
// parent as an ordinary diff, rather than diff-tree's default of emitting
// nothing for merges.
func (r *Repo) CommitFiles(ctx context.Context, hash string) ([]FileChange, error) {
	if err := validateHash(hash); err != nil {
		return nil, fmt.Errorf("CommitFiles: %w", err)
	}
	out, err := gitexec.Run(ctx, r.dir, "diff-tree", "--root", "--no-commit-id", "--name-status", "-z", "-r", "-M", "-m", "--first-parent", hash)
	if err != nil {
		return nil, err
	}
	return parseNameStatus(out), nil
}

// CommitFileDiff returns the raw unified diff text for path as changed by
// the given commit, equivalent to `git show --format= <hash> -- <path>`.
// For a merge commit, -m --first-parent makes it diff against the merge's
// first parent as an ordinary "diff --git" diff, rather than the
// "diff --cc" combined-diff format git show otherwise emits for merges.
func (r *Repo) CommitFileDiff(ctx context.Context, hash, path string) (string, error) {
	if err := validateHash(hash); err != nil {
		return "", fmt.Errorf("CommitFileDiff: %w", err)
	}
	out, err := gitexec.Run(ctx, r.dir, "show", "--format=", "-m", "--first-parent", hash, "--", path)
	if err != nil {
		return "", err
	}
	return out, nil
}

// ChangedFiles lists every changed file in the working tree, covering both
// staged and unstaged changes as well as untracked files, analogous to
// `git status`.
func (r *Repo) ChangedFiles(ctx context.Context) ([]FileChange, error) {
	// -uall expands an untracked directory into its individual files
	// instead of collapsing it into one "?? dir/" entry, which would fail
	// to diff (a directory has no file content of its own).
	out, err := gitexec.Run(ctx, r.dir, "status", "--porcelain=v1", "-uall", "-z")
	if err != nil {
		return nil, err
	}
	return parsePorcelain(out), nil
}

// FileDiff returns the raw unified diff text for path in the working tree,
// covering both staged and unstaged changes against HEAD (equivalent to
// `git diff HEAD -- <path>`). A genuinely untracked path has no index entry
// at all, so plain `git diff` (even against HEAD) never shows it; those are
// diffed against an empty file instead, so their whole content appears as
// added, matching how ChangedFiles reports them.
func (r *Repo) FileDiff(ctx context.Context, path string) (string, error) {
	statusOut, err := gitexec.Run(ctx, r.dir, "status", "--porcelain=v1", "-z", "--", path)
	if err != nil {
		return "", err
	}
	if len(statusOut) >= 2 && statusOut[0] == '?' && statusOut[1] == '?' {
		return r.diffAgainstEmpty(ctx, path)
	}

	out, err := gitexec.Run(ctx, r.dir, "diff", "HEAD", "--", path)
	if err != nil {
		if isUnbornHead(err) {
			// A freshly initialized repository has no HEAD commit to diff
			// against; the empty-tree hash stands in for "no commits".
			return gitexec.Run(ctx, r.dir, "diff", emptyTreeHash, "--", path)
		}
		return "", err
	}
	return out, nil
}

// diffAgainstEmpty diffs path against an empty file via `git diff --no-index`,
// so an untracked file's entire content is reported as added lines. Unlike
// plain `git diff`, --no-index uses diff(1)-style exit codes: 0 (no
// difference) and 1 (difference found) are both success, only >1 is a real
// error.
func (r *Repo) diffAgainstEmpty(ctx context.Context, path string) (string, error) {
	cmd := gitexec.Command(ctx, r.dir, "diff", "--no-index", "--", os.DevNull, path)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
			return stdout.String(), nil
		}
		return "", fmt.Errorf("git diff --no-index -- %s: %w: %s", path, err, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}

// parseLog parses the output of `git log` with fields separated by %x1f
// (unit separator) and records terminated by %x1e (record separator).
// Field order: hash, author, date, subject.
func parseLog(out string) []Commit {
	records := strings.Split(out, "\x1e")
	commits := make([]Commit, 0, len(records))
	for _, record := range records {
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
func parseNameStatus(out string) []FileChange {
	tokens := strings.Split(out, "\x00")
	changes := make([]FileChange, 0, len(tokens))
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
			changes = append(changes, FileChange{Path: tokens[i+1], OrigPath: tokens[i], Type: Renamed, IsCode: isCodeFile(tokens[i+1])})
			i += 2
			continue
		}
		if i >= len(tokens) {
			break
		}
		changes = append(changes, FileChange{Path: tokens[i], Type: classifyStatus(status[0]), IsCode: isCodeFile(tokens[i])})
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
func parsePorcelain(out string) []FileChange {
	tokens := strings.Split(out, "\x00")
	changes := make([]FileChange, 0, len(tokens))
	for i := 0; i < len(tokens); i++ {
		record := tokens[i]
		if len(record) < 3 {
			continue
		}
		x, y := record[0], record[1]
		path := record[3:]

		change := FileChange{Path: path, Type: classify(x, y), IsCode: isCodeFile(path)}
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
