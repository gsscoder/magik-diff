package brief

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"mdiff/internal/gitexec"
)

// hiddenDocDirs are the dot-prefixed top-level folders whose docs ListDocs
// still lists; any other dot-prefixed path segment hides a file.
var hiddenDocDirs = []string{".github", ".agent-files", ".cursor"}

// agentDirs are the top-level folders whose docs are all suggested.
var agentDirs = []string{".agent-files", ".cursor"}

var docExtensions = []string{"md", "mdx", "markdown", "rst", "adoc"}

// textDocNames are the stems that make a .txt file a doc, so
// requirements.txt is not one.
var textDocNames = []string{
	"README", "NOTES", "ABOUT", "OVERVIEW", "ARCHITECTURE",
	"DESIGN", "INTENT", "SPEC", "CHANGELOG", "LICENSE",
}

// suggestedNames are the stems suggested at the repo root, besides README*.
var suggestedNames = []string{
	"ARCHITECTURE", "DESIGN", "OVERVIEW", "INTENT", "SPEC", "CLAUDE", "AGENTS",
}

// DocFile is one documentation file of a repository the user can select
// as a brief source. Path is relative to the repository root, with forward
// slashes; Size is in bytes; Suggested marks the files most likely to
// describe the project.
type DocFile struct {
	Path      string `json:"path"`
	Size      int64  `json:"size"`
	Suggested bool   `json:"suggested"`
}

// ListDocs returns the documentation files of the repository rooted at
// root: tracked and untracked files git does not ignore, filtered to doc
// extensions and outside hidden folders (see hiddenDocDirs). Symlinks and
// paths missing from the working tree are skipped, so every listed file
// is a regular file inside the repository. The result is sorted suggested
// first, then by folder depth, then by path, and is never nil.
func ListDocs(ctx context.Context, root string) ([]DocFile, error) {
	out, err := gitexec.Run(ctx, root, "ls-files", "-z", "--cached", "--others", "--exclude-standard")
	if err != nil {
		return nil, fmt.Errorf("brief: failed to list the files of %s: %w", root, err)
	}

	docs := []DocFile{}
	for _, p := range strings.Split(out, "\x00") {
		if p == "" || hasHiddenSegment(p) || !isDoc(path.Base(p)) {
			continue
		}
		info, err := os.Lstat(filepath.Join(root, filepath.FromSlash(p)))
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return nil, fmt.Errorf("brief: failed to stat %s: %w", p, err)
		}
		if !info.Mode().IsRegular() {
			continue
		}
		docs = append(docs, DocFile{Path: p, Size: info.Size(), Suggested: isSuggested(p)})
	}

	slices.SortFunc(docs, compareDocs)
	// An unmerged path is listed once per conflict stage; sorting made the
	// copies adjacent.
	return slices.CompactFunc(docs, func(a, b DocFile) bool { return a.Path == b.Path }), nil
}

// hasHiddenSegment reports whether any segment of the slash-separated path
// p starts with a dot, ignoring a leading folder from hiddenDocDirs.
func hasHiddenSegment(p string) bool {
	segments := strings.Split(p, "/")
	if len(segments) > 1 && slices.Contains(hiddenDocDirs, segments[0]) {
		segments = segments[1:]
	}
	return slices.ContainsFunc(segments, func(s string) bool { return strings.HasPrefix(s, ".") })
}

// isDoc reports whether the file name has a doc extension, or is a .txt
// file with one of textDocNames as its stem.
func isDoc(name string) bool {
	dot := strings.LastIndexByte(name, '.')
	if dot < 0 {
		return false
	}
	stem, ext := name[:dot], name[dot+1:]
	return containsFold(docExtensions, ext) || (strings.EqualFold(ext, "txt") && containsFold(textDocNames, stem))
}

// isSuggested reports whether the doc at the slash-separated path p is
// likely to describe the project: anything under agentDirs or directly in
// the top-level docs folder, and README* or a suggestedNames stem at the
// repo root.
func isSuggested(p string) bool {
	if top, _, nested := strings.Cut(p, "/"); nested && slices.Contains(agentDirs, top) {
		return true
	}
	switch path.Dir(p) {
	case "docs":
		return true
	case ".":
		stem := strings.TrimSuffix(p, path.Ext(p))
		isReadme := len(stem) >= len("README") && strings.EqualFold(stem[:len("README")], "README")
		return isReadme || containsFold(suggestedNames, stem)
	default:
		return false
	}
}

// compareDocs orders suggested docs first, then shallower paths, then
// paths in byte order.
func compareDocs(a, b DocFile) int {
	if a.Suggested != b.Suggested {
		if a.Suggested {
			return -1
		}
		return 1
	}
	return cmp.Or(
		cmp.Compare(strings.Count(a.Path, "/"), strings.Count(b.Path, "/")),
		strings.Compare(a.Path, b.Path),
	)
}

// containsFold reports whether names holds s, ignoring case.
func containsFold(names []string, s string) bool {
	return slices.ContainsFunc(names, func(name string) bool { return strings.EqualFold(name, s) })
}
