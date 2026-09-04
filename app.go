package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"mdiff/internal/brief"
	"mdiff/internal/checks"
	"mdiff/internal/config"
	"mdiff/internal/diffparse"
	"mdiff/internal/explain"
	"mdiff/internal/gitdiff"
	"mdiff/internal/llm"
	"mdiff/internal/watch"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type App struct {
	ctx       context.Context
	repo      *gitdiff.Repo
	repoValid bool
	watcher   *watch.Watcher
	explain   *explain.Service
}

func NewApp() *App {
	return &App{explain: explain.NewService()}
}

// startup is called when the app starts. The context is saved
// so we can call the runtime methods
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	cwd, err := os.Getwd()
	if err != nil {
		slog.Error("startup: get working directory", "error", err)
		return
	}
	root, err := gitdiff.FindRoot(ctx, cwd)
	if err != nil {
		a.repo = gitdiff.New(cwd)
		return
	}
	a.switchRepo(root)
}

// startWatcher starts a repo watcher rooted at root, emitting a
// "repo:changed" event to the frontend whenever the repo's tracked state
// changes from outside the running process. It logs and returns on error
// rather than failing the caller.
func (a *App) startWatcher(root string) {
	w, err := watch.New(root, func() {
		runtime.EventsEmit(a.ctx, "repo:changed")
	})
	if err != nil {
		slog.Error("start repo watcher", "root", root, "error", err)
		return
	}
	a.watcher = w
}

// switchRepo makes root — which must already be a resolved repository
// top-level directory (see gitdiff.FindRoot) — the active repo: it replaces
// a.repo, marks the repo valid, and restarts the filesystem watcher rooted
// there, closing any previous watcher first.
func (a *App) switchRepo(root string) {
	a.repo = gitdiff.New(root)
	a.repoValid = true
	if a.watcher != nil {
		if err := a.watcher.Close(); err != nil {
			slog.Error("close previous repo watcher", "error", err)
		}
		a.watcher = nil
	}
	a.startWatcher(root)
}

// WorkingDir returns the active repo's root directory, shown in the title
// bar.
func (a *App) WorkingDir() (string, error) {
	return a.repo.Dir(), nil
}

// IsGitRepo reports whether the active repo — resolved at startup from the
// process's working directory, or by OpenAndSwitchRepo — is a valid git
// repository. Unlike a bare stat of ".git" in the working directory, the
// resolution walks up parent directories, so launching from (or opening) a
// subdirectory of a repo still resolves to that repo.
func (a *App) IsGitRepo() bool {
	return a.repoValid
}

// CurrentBranch returns the name of the currently checked-out branch in the
// repo rooted at the current working directory, for display in the status
// bar. It returns an empty string, not an error, when HEAD is detached.
func (a *App) CurrentBranch() (string, error) {
	return a.repo.CurrentBranch(a.ctx)
}

// OpenFolderResult reports the outcome of OpenAndSwitchRepo.
type OpenFolderResult struct {
	// Canceled is true when the user dismissed the dialog without picking
	// a folder; Path and Valid are meaningless in that case.
	Canceled bool
	// Path is the resolved repository root — the folder the user picked, or
	// its nearest enclosing repo root if they picked a subdirectory — when
	// not Canceled.
	Path string
	// Valid reports whether Path is a git repository (contains .git). When
	// true, the app's active repo has already been switched to Path.
	Valid bool
}

// OpenAndSwitchRepo shows a native folder-selection dialog. If the user
// picks a folder containing .git, the app's active repo is switched to it
// (so all subsequent gitdiff calls target the new repo) and Valid is true.
// A picked subdirectory of a repo resolves to its enclosing repo root. If
// the picked folder is not a git repository, the active repo is left
// unchanged and Valid is false. If the user cancels the dialog, Canceled is
// true and Path/Valid are meaningless.
func (a *App) OpenAndSwitchRepo() (OpenFolderResult, error) {
	path, err := runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Open repository folder",
	})
	if err != nil {
		return OpenFolderResult{}, fmt.Errorf("open folder dialog: %w", err)
	}
	if path == "" {
		return OpenFolderResult{Canceled: true}, nil
	}
	root, err := gitdiff.FindRoot(a.ctx, path)
	if err != nil {
		return OpenFolderResult{Path: path, Valid: false}, nil
	}
	a.switchRepo(root)
	return OpenFolderResult{Path: root, Valid: true}, nil
}

// ChangedFiles lists every changed file in the working tree of the repo
// rooted at the current working directory.
func (a *App) ChangedFiles() ([]gitdiff.FileChange, error) {
	return a.repo.ChangedFiles(a.ctx)
}

// FileDiff returns the parsed unified diff for path in the working tree.
func (a *App) FileDiff(path string) (diffparse.FileDiff, error) {
	raw, err := a.repo.FileDiff(a.ctx, path)
	if err != nil {
		return diffparse.FileDiff{}, err
	}
	return diffparse.Parse(raw)
}

// RecentCommits returns up to count commits from the repository history,
// newest first, skipping the first skip commits for paging.
func (a *App) RecentCommits(skip, count int) ([]gitdiff.Commit, error) {
	return a.repo.RecentCommits(a.ctx, skip, count)
}

// CommitFiles lists every path changed by the given commit.
func (a *App) CommitFiles(hash string) ([]gitdiff.FileChange, error) {
	return a.repo.CommitFiles(a.ctx, hash)
}

// CommitFileDiff returns the parsed unified diff for path as changed by
// the given commit.
func (a *App) CommitFileDiff(hash, path string) (diffparse.FileDiff, error) {
	raw, err := a.repo.CommitFileDiff(a.ctx, hash, path)
	if err != nil {
		return diffparse.FileDiff{}, err
	}
	return diffparse.Parse(raw)
}

// GetConfig returns the current non-secret application settings.
func (a *App) GetConfig() (config.Config, error) {
	return config.Load()
}

// SaveConfig writes the non-secret application settings.
func (a *App) SaveConfig(cfg config.Config) error {
	return config.Save(cfg)
}

// HasAPIKey reports whether an API key is currently available, without ever
// returning the key itself to the frontend.
func (a *App) HasAPIKey() (bool, error) {
	key, _ := config.GetAPIKey()
	return key != "", nil
}

// APIKeyUsedFallback reports whether the currently available API key (if
// any) was read from the environment variable fallback rather than the OS
// keyring, so the frontend can surface that distinctly from "no key set".
func (a *App) APIKeyUsedFallback() (bool, error) {
	_, usedFallback := config.GetAPIKey()
	return usedFallback, nil
}

// SetAPIKey stores a new API key in the OS keyring.
func (a *App) SetAPIKey(key string) error {
	return config.SetAPIKey(key)
}

// VerifyLLMConfig sends a minimal chat-completions request to baseURL/model
// using apiKey, to confirm the endpoint is reachable and credentials are
// valid before the user saves them. If apiKey is empty, the currently
// stored key (if any) is used instead. It returns an error describing the
// failure, or nil on success.
func (a *App) VerifyLLMConfig(baseURL, model, apiKey string) error {
	if apiKey == "" {
		apiKey, _ = config.GetAPIKey()
	}
	_, err := llm.Explain(a.ctx, baseURL, model, apiKey, "Hi")
	return err
}

// Explain asks the configured LLM to explain the diffs of paths, scoped to
// the working tree when hash is empty or to the given commit otherwise. A
// single path uses the terse per-file prompt; multiple paths are combined
// into one diff blob and explained holistically. When useBrief is true and
// a project brief is stored for the active repo (regardless of staleness),
// its text is passed through as reference context; otherwise no brief is
// used. It returns a clear error if paths is empty, or if the base URL,
// model, or API key is not configured.
func (a *App) Explain(hash string, paths []string, useBrief bool) (string, error) {
	briefText := ""
	if useBrief {
		if st, err := brief.GetState(a.repo.Dir()); err == nil && st.Stored {
			briefText = st.Brief.Text
		}
	}
	return a.explain.Explain(a.ctx, a.repo, hash, paths, briefText)
}

// ProjectBrief returns the current state of the project brief for the
// active repo: which AI-instruction files are present, whether a brief has
// been extracted before, and whether it's stale relative to those files.
func (a *App) ProjectBrief() (brief.State, error) {
	return brief.GetState(a.repo.Dir())
}

// AcquireProjectBrief scans the active repo's AI-instruction files and
// extracts a fresh project brief via the configured LLM, persisting it and
// returning the resulting state. It returns a clear error if no
// AI-instruction files are present, or if the LLM is not configured.
func (a *App) AcquireProjectBrief() (brief.State, error) {
	root := a.repo.Dir()
	if _, err := brief.Acquire(a.ctx, root); err != nil {
		return brief.State{}, err
	}
	return brief.GetState(root)
}

// ListChecks returns every user-defined check available in the checks
// directory.
func (a *App) ListChecks() ([]checks.Check, error) {
	return checks.List()
}

// RunCheck runs the named user-defined check against the diffs of paths,
// scoped to the working tree when hash is empty or to the given commit
// otherwise, returning the LLM's prose response. It returns a clear error
// if paths is empty, if no check with that name exists, or if the base URL,
// model, or API key is not configured.
func (a *App) RunCheck(hash, checkName string, paths []string) (string, error) {
	return a.explain.RunCheck(a.ctx, a.repo, hash, checkName, paths)
}
