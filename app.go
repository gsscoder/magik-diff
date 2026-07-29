package main

import (
	"context"
	"fmt"

	"mdiff/internal/config"
	"mdiff/internal/diffparse"
	"mdiff/internal/gitdiff"
	"mdiff/internal/llm"
)

// App struct
type App struct {
	ctx context.Context
}

// NewApp creates a new App application struct
func NewApp() *App {
	return &App{}
}

// startup is called when the app starts. The context is saved
// so we can call the runtime methods
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

// Greet returns a greeting for the given name
func (a *App) Greet(name string) string {
	return fmt.Sprintf("Hello %s, It's show time!", name)
}

// ChangedFiles lists every changed file in the working tree of the repo
// rooted at the current working directory.
func (a *App) ChangedFiles() ([]gitdiff.FileChange, error) {
	return gitdiff.ChangedFiles()
}

// FileDiff returns the parsed unified diff for path in the working tree.
func (a *App) FileDiff(path string) (diffparse.FileDiff, error) {
	raw, err := gitdiff.FileDiff(path)
	if err != nil {
		return diffparse.FileDiff{}, err
	}
	return diffparse.Parse(raw)
}

// RecentCommits returns up to count commits from the repository history,
// newest first, skipping the first skip commits for paging.
func (a *App) RecentCommits(skip, count int) ([]gitdiff.Commit, error) {
	return gitdiff.RecentCommits(skip, count)
}

// CommitFiles lists every path changed by the given commit.
func (a *App) CommitFiles(hash string) ([]gitdiff.FileChange, error) {
	return gitdiff.CommitFiles(hash)
}

// CommitFileDiff returns the parsed unified diff for path as changed by
// the given commit.
func (a *App) CommitFileDiff(hash, path string) (diffparse.FileDiff, error) {
	raw, err := gitdiff.CommitFileDiff(hash, path)
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

// ExplainFile fetches the raw diff for path and asks the configured LLM to
// explain what changed and why, returning its prose response. It returns a
// clear error, rather than an empty string, if the base URL, model, or API
// key is not configured.
func (a *App) ExplainFile(path string) (string, error) {
	diff, err := gitdiff.FileDiff(path)
	if err != nil {
		return "", err
	}

	cfg, err := config.Load()
	if err != nil {
		return "", err
	}
	apiKey, _ := config.GetAPIKey()

	if cfg.BaseURL == "" || cfg.Model == "" || apiKey == "" {
		return "", fmt.Errorf("mdiff is not configured for Explain: set the base URL, model, and API key first")
	}

	prompt := fmt.Sprintf(
		"Explain what changed in the following diff and why, in plain prose. "+
			"Be as terse as the change deserves: a trivial or mechanical change "+
			"(e.g. one ignore-list entry, a formatting fix, a comment tweak) gets "+
			"one short sentence, not a full breakdown. Only go longer when the "+
			"change is genuinely substantial. No preamble, no restating the diff "+
			"line by line, no generic best-practice commentary, no headings or "+
			"lists, no summary at the end.\n\n%s",
		diff,
	)
	return llm.Explain(cfg.BaseURL, cfg.Model, apiKey, prompt)
}
