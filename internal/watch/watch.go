// Package watch monitors a git repository's working tree for mutations made
// from outside the running process (a coding agent editing files, or git
// commands run in another terminal) and calls back when the repo's tracked
// state actually changed. fsnotify only triggers a check; git itself is the
// authority on whether anything meaningful happened, via a signature built
// from `git status` and the current HEAD, so edits to ignored files or
// resaves of unchanged content never cause a spurious refresh.
package watch

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"

	"mdiff/internal/gitexec"
)

// debounceInterval batches a burst of filesystem events (e.g. an agent's
// multi-file write) into a single status check.
const debounceInterval = 300 * time.Millisecond

// Watcher watches a git repository's working tree rooted at root and calls
// onChange whenever the repo's tracked state (status + HEAD) changes.
type Watcher struct {
	root     string
	fsw      *fsnotify.Watcher
	onChange func()
	done     chan struct{}

	// ignored is built once in New and only ever read afterwards, from the
	// single background goroutine, so it needs no synchronization.
	ignored map[string]struct{}
	// sig is only read and written from the background goroutine.
	sig string
}

// New creates a Watcher rooted at root. It synchronously builds the set of
// gitignored directories to skip, walks root adding an fsnotify watch on
// every non-ignored directory (plus .git and .git/refs/heads), records the
// initial status signature, and starts the watcher's background goroutine
// before returning. onChange is invoked from that goroutine.
func New(root string, onChange func()) (*Watcher, error) {
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("create fsnotify watcher: %w", err)
	}

	w := &Watcher{
		root:     root,
		fsw:      fsw,
		onChange: onChange,
		done:     make(chan struct{}),
	}

	w.ignored, err = ignoredDirs(root)
	if err != nil {
		fsw.Close()
		return nil, err
	}
	if err := w.watchTree(); err != nil {
		fsw.Close()
		return nil, err
	}
	w.sig, err = status(root)
	if err != nil {
		fsw.Close()
		return nil, err
	}

	go w.loop()
	return w, nil
}

// Close stops the watcher's background goroutine and closes the underlying
// fsnotify watcher. Safe to call once.
func (w *Watcher) Close() error {
	close(w.done)
	return w.fsw.Close()
}

// watchTree walks w.root, adding an fsnotify watch on every directory
// except .git (handled separately by watchGitDir) and any directory in
// w.ignored, whose contents are skipped entirely rather than reimplementing
// gitignore matching.
func (w *Watcher) watchTree() error {
	err := filepath.WalkDir(w.root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(w.root, path)
		if err != nil {
			return err
		}
		if rel == ".git" {
			return fs.SkipDir
		}
		if rel != "." {
			if _, skip := w.ignored[rel]; skip {
				return fs.SkipDir
			}
		}
		if err := w.fsw.Add(path); err != nil {
			return fmt.Errorf("watch %s: %w", path, err)
		}
		return nil
	})
	if err != nil {
		return err
	}
	return w.watchGitDir()
}

// watchGitDir adds a non-recursive watch on .git itself, so staging/commit
// changes are seen, and on .git/refs/heads, so branch tip updates are seen.
// It deliberately does not recurse into .git/objects or .git/logs, which is
// pure noise for this purpose. A .git that is a file, not a directory
// (worktrees/submodules), has nothing to watch here.
func (w *Watcher) watchGitDir() error {
	gitDir := filepath.Join(w.root, ".git")
	if info, err := os.Stat(gitDir); err != nil || !info.IsDir() {
		return nil
	}
	if err := w.fsw.Add(gitDir); err != nil {
		return fmt.Errorf("watch %s: %w", gitDir, err)
	}
	refsHeads := filepath.Join(gitDir, "refs", "heads")
	if info, err := os.Stat(refsHeads); err == nil && info.IsDir() {
		if err := w.fsw.Add(refsHeads); err != nil {
			return fmt.Errorf("watch %s: %w", refsHeads, err)
		}
	}
	return nil
}

// loop is the watcher's single managed goroutine, tied to Close via w.done.
// Every fsnotify event resets a debounce timer; when the timer fires, it
// checks whether the repo's tracked state actually changed and, if so,
// calls onChange.
func (w *Watcher) loop() {
	timer := time.NewTimer(debounceInterval)
	if !timer.Stop() {
		<-timer.C
	}

	for {
		select {
		case <-w.done:
			timer.Stop()
			return
		case event, ok := <-w.fsw.Events:
			if !ok {
				return
			}
			if event.Has(fsnotify.Create) {
				w.watchIfNewDir(event.Name)
			}
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(debounceInterval)
		case err, ok := <-w.fsw.Errors:
			if !ok {
				return
			}
			slog.Error("watch: fsnotify error", "error", err)
		case <-timer.C:
			w.checkChanged()
		}
	}
}

// watchIfNewDir adds a watch on path if it is a newly created directory
// that isn't gitignored, so directories a coding agent creates later (e.g.
// via mkdir -p) get picked up. It checks the single path with
// `git check-ignore`, cheaper than rebuilding the whole ignored-dir set.
func (w *Watcher) watchIfNewDir(path string) {
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return
	}
	rel, err := filepath.Rel(w.root, path)
	if err != nil {
		return
	}
	if rel == ".git" || strings.HasPrefix(rel, ".git"+string(filepath.Separator)) {
		return
	}
	if checkIgnored(w.root, path) {
		return
	}
	if err := w.fsw.Add(path); err != nil {
		slog.Error("watch: add new directory", "path", path, "error", err)
	}
}

// checkChanged compares the repo's current status signature to the last
// known one, calling onChange and updating the stored signature only when
// they differ.
func (w *Watcher) checkChanged() {
	sig, err := status(w.root)
	if err != nil {
		slog.Error("watch: check status", "error", err)
		return
	}
	if sig == w.sig {
		return
	}
	w.sig = sig
	w.onChange()
}

// ignoredDirs returns the set of directories, relative to root, that git
// considers entirely ignored, via `git ls-files --others --ignored
// --exclude-standard --directory -z`. git only reports a directory here
// when its whole content is ignored, without recursing into it, which lets
// the walk skip whole ignored subtrees without reimplementing gitignore
// matching.
func ignoredDirs(root string) (map[string]struct{}, error) {
	out, err := gitexec.Run(context.Background(), root, "ls-files", "--others", "--ignored", "--exclude-standard", "--directory", "-z")
	if err != nil {
		return nil, err
	}
	dirs := make(map[string]struct{})
	for _, entry := range strings.Split(out, "\x00") {
		if entry == "" {
			continue
		}
		dirs[filepath.FromSlash(strings.TrimSuffix(entry, "/"))] = struct{}{}
	}
	return dirs, nil
}

// checkIgnored reports whether path is ignored by git, via
// `git check-ignore -q <path>` (exit code 0 means ignored).
func checkIgnored(root, path string) bool {
	cmd := gitexec.Command(context.Background(), root, "check-ignore", "-q", path)
	return cmd.Run() == nil
}

// status returns a signature for the repo's current tracked state, combining
// `git status --porcelain=v1 -z` with `git rev-parse HEAD`. A repository
// with no commits yet has no HEAD; that error is ignored silently.
func status(root string) (string, error) {
	ctx := context.Background()
	statusOut, err := gitexec.Run(ctx, root, "status", "--porcelain=v1", "-z")
	if err != nil {
		return "", err
	}
	head, _ := gitexec.Run(ctx, root, "rev-parse", "HEAD")
	return statusOut + "\x00" + head, nil
}
