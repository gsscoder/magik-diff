// Package brief lists a repository's documentation files and, on explicit
// request, sends the ones the user selects through the configured LLM in
// two calls: the first writes an overview under fixed headings, the second
// shortens it into a few bullets, the "project brief". Only the brief is
// persisted, per repository and outside it, so it survives restarts; it is
// never checked against the files again, regenerating it is the user's job.
package brief

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"mdiff/internal/config"
	"mdiff/internal/llm"
)

// repoStoreDirName is the subdirectory of config.BaseDir() that holds one
// persisted Brief per repository.
const repoStoreDirName = "repos"

// maxFiles is the most doc files one Generate call accepts.
const maxFiles = 5

// maxPromptTokens caps the estimated size of the overview prompt. Too much
// text is rejected, never truncated.
const maxPromptTokens = 100_000

// notDescribed is the error Generate returns when the overview call gives
// no usable answer.
const notDescribed = "The selected files do not describe the project. Select other files."

// Brief is the persisted result for one repository: the doc files it was
// generated from, relative to RepoPath, and the brief text.
type Brief struct {
	RepoPath string   `json:"repo_path"`
	Files    []string `json:"files"`
	Text     string   `json:"text"`
}

// State is the brief-related status of a repository.
type State struct {
	// Stored reports whether a brief has been persisted for this repo.
	Stored bool
	// Brief is only meaningful when Stored is true.
	Brief Brief
}

// storePath returns the on-disk location of the persisted Brief for the
// repository rooted at root: <config.BaseDir()>/repos/<first 16 hex chars
// of sha256(normalized absolute root path))>.json. Hashing the absolute,
// cleaned path (rather than flattening it into a filename) avoids both
// filename collisions and Windows path-length problems. The path is
// normalized by resolving symlinks (falling back to the pre-resolution path
// if that fails, e.g. on a filesystem where it isn't meaningful) and, on
// Windows, lowercasing it, since NTFS is case-insensitive/case-preserving;
// this keeps two on-disk spellings of the same repo (a mapped drive letter,
// a symlink/junction, or a differently-cased path) mapped to the same store
// file, without merging genuinely different repos on case-sensitive
// filesystems.
func storePath(root string) (string, error) {
	dir, err := config.BaseDir()
	if err != nil {
		return "", err
	}

	abs, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("brief: failed to resolve absolute path for %s: %w", root, err)
	}
	abs = filepath.Clean(abs)

	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		abs = resolved
	}
	if runtime.GOOS == "windows" {
		abs = strings.ToLower(abs)
	}

	sum := sha256.Sum256([]byte(abs))
	name := hex.EncodeToString(sum[:])[:16] + ".json"
	return filepath.Join(dir, repoStoreDirName, name), nil
}

// Load reads the persisted Brief for the repository rooted at root. A
// missing file is not an error: it returns Brief{}, false, nil, mirroring
// config.Load's handling of a missing config file.
func Load(root string) (Brief, bool, error) {
	path, err := storePath(root)
	if err != nil {
		return Brief{}, false, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Brief{}, false, nil
		}
		return Brief{}, false, err
	}

	var b Brief
	if err := json.Unmarshal(data, &b); err != nil {
		return Brief{}, false, fmt.Errorf("brief: failed to parse stored brief %s: %w", path, err)
	}
	return b, true, nil
}

// Save writes b to its per-repository store location, creating the parent
// directory if needed.
func Save(b Brief) error {
	path, err := storePath(b.RepoPath)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("brief: failed to create store directory: %w", err)
	}

	data, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		return fmt.Errorf("brief: failed to encode brief: %w", err)
	}

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("brief: failed to write %s: %w", path, err)
	}
	return nil
}

// checkSelection requires between 1 and maxFiles selected paths, each one
// a doc ListDocs found: this keeps the model input inside the doc list,
// whatever the caller sends.
func checkSelection(docs []DocFile, files []string) error {
	if len(files) == 0 {
		return errors.New("Select at least one file.")
	}
	if len(files) > maxFiles {
		return fmt.Errorf("Select at most %d files.", maxFiles)
	}
	for _, f := range files {
		if !slices.ContainsFunc(docs, func(d DocFile) bool { return d.Path == f }) {
			return fmt.Errorf("Not a documentation file of this repo: %s", f)
		}
	}
	return nil
}

// checkBudget rejects a prompt whose estimated size, one token per four
// bytes, exceeds maxPromptTokens.
func checkBudget(prompt string) error {
	tokens := len(prompt) / 4
	if tokens > maxPromptTokens {
		return fmt.Errorf("The selected files are too large for one request (about %d tokens, the limit is %d). Select fewer files.", tokens, maxPromptTokens)
	}
	return nil
}

// overviewRequest returns the prompt of the overview call for the selected
// doc files of the repository rooted at root, after checking the selection
// and the prompt size.
func overviewRequest(ctx context.Context, root string, files []string) (string, error) {
	docs, err := ListDocs(ctx, root)
	if err != nil {
		return "", err
	}
	if err := checkSelection(docs, files); err != nil {
		return "", err
	}

	blocks := make([]string, len(files))
	for i, f := range files {
		text, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(f)))
		if err != nil {
			return "", fmt.Errorf("brief: failed to read %s: %w", f, err)
		}
		blocks[i] = docBlock(f, string(text))
	}

	request := prompt(overviewPrompt, strings.Join(blocks, "\n"))
	if err := checkBudget(request); err != nil {
		return "", err
	}
	return request, nil
}

// requestOverview runs the overview call, asking once more after an
// unusable answer but not after the sentinel answer.
func requestOverview(ctx context.Context, ep config.LLM, request string) (string, error) {
	for range 2 {
		answer, err := llm.Explain(ctx, ep, request, nil)
		if err != nil {
			return "", err
		}
		overview, err := validateOverview(answer)
		if err == nil {
			return overview, nil
		}
		if errors.Is(err, errInsufficientSources) {
			break
		}
	}
	return "", errors.New(notDescribed)
}

// requestBrief runs the brief call over overview.
func requestBrief(ctx context.Context, ep config.LLM, overview string) (string, error) {
	answer, err := llm.Explain(ctx, ep, prompt(briefPrompt, overviewBlock(overview)), nil)
	if err != nil {
		return "", err
	}
	return validateBrief(answer)
}

// Generate turns files, doc paths of the repository rooted at root as
// ListDocs returns them, into a project brief via the configured LLM,
// persists it and returns it. The selection and the prompt size are checked
// before any LLM call; it returns a clear error if they are invalid, if the
// LLM settings are incomplete, or if the model finds no project description
// in the files.
func Generate(ctx context.Context, root string, files []string) (Brief, error) {
	request, err := overviewRequest(ctx, root, files)
	if err != nil {
		return Brief{}, err
	}

	ep, err := config.LoadLLM()
	if err != nil {
		return Brief{}, fmt.Errorf("mdiff is not configured for Explain: %w", err)
	}

	overview, err := requestOverview(ctx, ep, request)
	if err != nil {
		return Brief{}, err
	}
	text, err := requestBrief(ctx, ep, overview)
	if err != nil {
		return Brief{}, err
	}

	b := Brief{RepoPath: root, Files: files, Text: dropLineEndPeriods(text)}
	if err := Save(b); err != nil {
		return Brief{}, err
	}
	return b, nil
}

// GetState loads the persisted brief of the repository rooted at root, if
// any.
func GetState(root string) (State, error) {
	b, ok, err := Load(root)
	if err != nil {
		return State{}, err
	}
	return State{Stored: ok, Brief: b}, nil
}
