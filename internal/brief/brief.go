// Package brief scans a repository for AI-agent instruction files (AGENTS.md,
// CLAUDE.md, and similar), and, on explicit request, sends their contents
// through the configured LLM to extract a short factual "project brief"
// (purpose, language, stack, architectural pattern), discarding any agent
// directives/rules found in those files. The result is persisted to disk per
// repository so it survives restarts and is only regenerated when the
// source files change.
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
	"strings"
	"unicode/utf8"

	"mdiff/internal/config"
	"mdiff/internal/llm"
)

// candidateFiles lists the AI-agent instruction filenames Scan looks for,
// most-meaningful first. This is deliberately not user-configurable: it is
// a fixed, opinionated list of the conventions in common use.
var candidateFiles = []string{
	"AGENTS.md",
	"CLAUDE.md",
	".github/copilot-instructions.md",
	"GEMINI.md",
	".cursorrules",
}

// maxSources caps how many distinct instruction files Scan keeps, so the
// extraction prompt stays bounded even in a repo with every convention
// present at once.
const maxSources = 3

// maxPromptInputBytes caps the total size of file content fed to the LLM
// for extraction. Content beyond this limit is truncated from the end of
// the concatenated blob, which (since files are concatenated in
// candidateFiles priority order) drops the least-priority files' content
// first.
const maxPromptInputBytes = 32 * 1024

// repoStoreDirName is the subdirectory of config.BaseDir() that holds one
// persisted Brief per repository.
const repoStoreDirName = "repos"

// Source identifies one AI-agent instruction file Scan found, by its path
// relative to the repository root and the hex-encoded sha256 hash of its
// raw bytes.
type Source struct {
	Path string `json:"path"`
	Hash string `json:"hash"`
}

// Brief is the persisted extraction result for one repository.
type Brief struct {
	RepoPath string   `json:"repo_path"`
	Sources  []Source `json:"sources"`
	Text     string   `json:"text"`
}

// State is the brief-related status of a repository, as surfaced to
// callers deciding whether to offer generating or regenerating a brief.
type State struct {
	// HasSources reports whether any candidate instruction files exist in
	// the repository right now.
	HasSources bool
	// Sources is the current scan result.
	Sources []Source
	// Stored reports whether a brief has been persisted for this repo
	// before.
	Stored bool
	// Stale is only meaningful when Stored is true: it reports whether the
	// stored brief's sources differ from the current scan.
	Stale bool
	// Brief is only meaningful when Stored is true.
	Brief Brief
}

// Scan looks for each name in candidateFiles under root, in priority order,
// reading and hashing every regular file found. A file byte-identical to
// one already kept (e.g. the same instructions copy-pasted into both
// AGENTS.md and CLAUDE.md) is skipped. Scan stops once maxSources distinct
// sources are kept, so earlier entries in candidateFiles win ties for the
// available slots. It returns nil, nil if none of the candidates exist.
func Scan(root string) ([]Source, error) {
	var sources []Source
	seen := make(map[string]bool)

	for _, name := range candidateFiles {
		if len(sources) >= maxSources {
			break
		}

		path := filepath.Join(root, name)
		info, err := os.Stat(path)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return nil, fmt.Errorf("brief: failed to stat %s: %w", path, err)
		}
		if !info.Mode().IsRegular() {
			continue
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("brief: failed to read %s: %w", path, err)
		}

		hash := hashBytes(data)
		if seen[hash] {
			continue
		}
		seen[hash] = true
		sources = append(sources, Source{Path: name, Hash: hash})
	}

	return sources, nil
}

// hashBytes returns the hex-encoded sha256 hash of data.
func hashBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
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

// Stale reports whether current differs from stored at all: a different
// number of sources, a source present in one but not the other, or a
// matching path with a different hash. Comparison is order-independent, so
// this only flags real changes, not re-ordering, and it catches a new
// candidate file appearing (or an existing one disappearing) just as much
// as a changed hash.
func Stale(stored, current []Source) bool {
	if len(stored) != len(current) {
		return true
	}

	byPath := make(map[string]string, len(stored))
	for _, s := range stored {
		byPath[s.Path] = s.Hash
	}

	for _, c := range current {
		hash, ok := byPath[c.Path]
		if !ok || hash != c.Hash {
			return true
		}
	}
	return false
}

// sourcesContent reads sources fresh from disk under root and concatenates
// them, each preceded by a "--- path ---" separator line, in the order
// given. The combined blob is truncated to maxPromptInputBytes from the end
// if it exceeds that size; truncated reports whether that happened.
func sourcesContent(root string, sources []Source) (combined string, truncated bool, err error) {
	blobs := make([]string, 0, len(sources))
	for _, s := range sources {
		data, err := os.ReadFile(filepath.Join(root, s.Path))
		if err != nil {
			return "", false, fmt.Errorf("brief: failed to read %s: %w", s.Path, err)
		}
		blobs = append(blobs, fmt.Sprintf("--- %s ---\n%s", s.Path, data))
	}

	combined = strings.Join(blobs, "\n\n")
	if len(combined) > maxPromptInputBytes {
		combined = combined[:maxPromptInputBytes]
		for !utf8.ValidString(combined) && len(combined) > 0 {
			combined = combined[:len(combined)-1]
		}
		truncated = true
	}
	return combined, truncated, nil
}

// extractionPrompt builds the prompt asking the LLM to extract a factual
// project brief from combined, the concatenated content of a repository's
// AI-agent instruction files. It explicitly instructs the model to discard
// agent directives/rules (e.g. "do not over-engineer", coding style rules,
// guardrails) and extract only concrete project facts: purpose/brief,
// language(s), frameworks/libraries, architectural pattern, and notable
// structure, in 4 to 6 sentences of plain prose with no meta-commentary
// about the extraction itself.
func extractionPrompt(combined string, truncated bool) string {
	note := ""
	if truncated {
		note = " the content below was truncated to fit a size limit; work with what is given.\n\n"
	}
	return fmt.Sprintf(
		"the following is the concatenated content of one or more AI-agent "+
			"instruction files found in a software repository (e.g. AGENTS.md, "+
			"CLAUDE.md, copilot-instructions.md), each preceded by a "+
			"\"--- path ---\" separator line. these files mix two kinds of "+
			"content: agent directives/rules, which are instructions telling an "+
			"AI coding agent how to behave (e.g. \"do not over-engineer\", "+
			"\"write clean code\", coding style rules, guardrails, workflow "+
			"rules), and concrete facts about the project itself. discard all "+
			"agent directives/rules entirely and extract only the concrete "+
			"project facts: the project's purpose/brief, its language(s), its "+
			"frameworks/libraries, its architectural pattern, and any notable "+
			"structure. write the result as plain prose in a minimum of 4 and "+
			"a maximum of 6 sentences, no headings or bullet lists, no "+
			"meta-commentary about this extraction process itself, no "+
			"restating that directives were discarded, and do not game the "+
			"sentence count by writing artificially long run-on sentences "+
			"stitched together with semicolons, commas, or excessive "+
			"conjunctions instead of real sentence breaks; each sentence "+
			"should be a normal, natural-length sentence.%s\n\n%s",
		note, combined,
	)
}

// Acquire scans root for AI-agent instruction files, sends their content
// through the configured LLM to extract a factual project brief, persists
// the result, and returns it. It returns a clear error if Scan finds no
// instruction files, or if the base URL, model, or API key is not
// configured.
func Acquire(ctx context.Context, root string) (Brief, error) {
	sources, err := Scan(root)
	if err != nil {
		return Brief{}, err
	}
	if len(sources) == 0 {
		return Brief{}, fmt.Errorf("no AI instruction files found in this repository")
	}

	combined, truncated, err := sourcesContent(root, sources)
	if err != nil {
		return Brief{}, err
	}
	prompt := extractionPrompt(combined, truncated)

	cfg, err := config.Load()
	if err != nil {
		return Brief{}, err
	}
	apiKey, _ := config.GetAPIKey()
	if cfg.BaseURL == "" || cfg.Model == "" || apiKey == "" {
		return Brief{}, fmt.Errorf("mdiff is not configured for Explain: set the base URL, model, and API key first")
	}

	result, err := llm.Explain(ctx, cfg.BaseURL, cfg.Model, apiKey, prompt, nil)
	if err != nil {
		return Brief{}, err
	}

	b := Brief{RepoPath: root, Sources: sources, Text: strings.TrimSpace(result)}
	if err := Save(b); err != nil {
		return Brief{}, err
	}
	return b, nil
}

// GetState scans root for AI-agent instruction files and loads any
// previously persisted brief, reporting whether the stored brief (if any)
// is stale relative to the current scan.
func GetState(root string) (State, error) {
	sources, err := Scan(root)
	if err != nil {
		return State{}, err
	}

	stored, ok, err := Load(root)
	if err != nil {
		return State{}, err
	}

	state := State{
		HasSources: len(sources) > 0,
		Sources:    sources,
		Stored:     ok,
	}
	if ok {
		state.Stale = Stale(stored.Sources, sources)
		state.Brief = stored
	}
	return state, nil
}
