package main

import (
	"context"
	"fmt"
	"strings"

	"mdiff/internal/checks"
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

// GetAPIKey returns the currently stored API key, so the settings dialog can
// prefill it for editing.
func (a *App) GetAPIKey() (string, error) {
	key, _ := config.GetAPIKey()
	return key, nil
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
	_, err := llm.Explain(baseURL, model, apiKey, "Hi")
	return err
}

// Explain asks the configured LLM to explain the diffs of paths, scoped to
// the working tree when hash is empty or to the given commit otherwise. A
// single path uses the terse per-file prompt; multiple paths are combined
// into one diff blob and explained holistically. It returns a clear error
// if paths is empty, or if the base URL, model, or API key is not
// configured.
func (a *App) Explain(hash string, paths []string) (string, error) {
	if len(paths) == 0 {
		return "", fmt.Errorf("nothing to explain: no files are selected")
	}
	if len(paths) == 1 {
		diff, err := diffForPath(hash, paths[0])
		if err != nil {
			return "", err
		}
		return explainDiff(diff)
	}
	combined, err := combineDiffs(paths, func(path string) (string, error) {
		return diffForPath(hash, path)
	})
	if err != nil {
		return "", err
	}
	return runExplain(allChangesPrompt(combined))
}

// diffForPath fetches the raw diff for path, scoped to the working tree
// when hash is empty or to the given commit otherwise.
func diffForPath(hash, path string) (string, error) {
	if hash == "" {
		return gitdiff.FileDiff(path)
	}
	return gitdiff.CommitFileDiff(hash, path)
}

// combineDiffs fetches the diff for each of paths via diffFn and
// concatenates them into one blob, with a "--- path ---" separator line
// preceding each file's diff, suitable as input to a holistic
// all-changes explanation prompt.
func combineDiffs(paths []string, diffFn func(path string) (string, error)) (string, error) {
	var b strings.Builder
	for i, p := range paths {
		diff, err := diffFn(p)
		if err != nil {
			return "", err
		}
		if i > 0 {
			b.WriteString("\n")
		}
		fmt.Fprintf(&b, "--- %s ---\n", p)
		b.WriteString(diff)
	}
	return b.String(), nil
}

// explainDiff asks the configured LLM to explain the given raw diff and
// returns its prose response. It returns a clear error, rather than an empty
// string, if the base URL, model, or API key is not configured.
func explainDiff(diff string) (string, error) {
	return runExplain(filePrompt(diff))
}

// filePrompt builds the terse per-file explanation prompt for a single diff.
func filePrompt(diff string) string {
	return fmt.Sprintf(
		"Explain what changed in the following diff and why, in plain prose. "+
			"Be as terse as the change deserves: a trivial or mechanical change "+
			"(e.g. one ignore-list entry, a formatting fix, a comment tweak) gets "+
			"one short sentence, not a full breakdown. Only go longer when the "+
			"change is genuinely substantial. No preamble, no restating the diff "+
			"line by line, no generic best-practice commentary, light markdown "+
			"like **bold** or inline `code` is fine if it genuinely helps but "+
			"don't force headings, bullet lists, or heavy structure onto a "+
			"short or simple explanation, no summary at the end.\n\n%s",
		diff,
	)
}

// allChangesPrompt builds the holistic-synthesis prompt for a combined diff
// spanning every file in a changeset, asking for one conceptual explanation
// of the changeset's overall intent rather than a per-file recap.
func allChangesPrompt(diff string) string {
	return fmt.Sprintf(
		"The following is a combined diff covering every changed file in one "+
			"changeset, each file's diff preceded by a \"--- path ---\" separator "+
			"line. Explain the overall intent and theme of this changeset as a "+
			"whole, synthesizing in your own words what it accomplishes "+
			"conceptually. Do not describe what each file does one by one, and do "+
			"not just concatenate per-file summaries. Be as terse as the change "+
			"deserves: one coherent paragraph or two is correct for a small or "+
			"mechanical changeset; only go longer if the change is genuinely large "+
			"or touches many unrelated concerns. If the changeset genuinely mixes "+
			"multiple unrelated concerns, briefly flag that fact, but do not "+
			"invent a multi-concern narrative for a changeset that is actually one "+
			"coherent thing. No preamble, no restating diff lines, light markdown "+
			"like **bold** or inline `code` is fine if it genuinely helps but "+
			"don't force headings, bullet lists, or per-file breakdown onto a "+
			"short or simple explanation, no summary at the end.\n\n%s",
		diff,
	)
}

// runExplain sends prompt to the configured LLM and returns its prose
// response. It returns a clear error, rather than an empty string, if the
// base URL, model, or API key is not configured.
func runExplain(prompt string) (string, error) {
	cfg, err := config.Load()
	if err != nil {
		return "", err
	}
	apiKey, _ := config.GetAPIKey()

	if cfg.BaseURL == "" || cfg.Model == "" || apiKey == "" {
		return "", fmt.Errorf("mdiff is not configured for Explain: set the base URL, model, and API key first")
	}

	return llm.Explain(cfg.BaseURL, cfg.Model, apiKey, prompt)
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
	if len(paths) == 0 {
		return "", fmt.Errorf("nothing to check: no files are selected")
	}
	c, err := findCheck(checkName)
	if err != nil {
		return "", err
	}
	combined, err := combineDiffs(paths, func(path string) (string, error) {
		return diffForPath(hash, path)
	})
	if err != nil {
		return "", err
	}
	return runExplain(checkPrompt(combined, c))
}

// findCheck loads every available check and returns the one named
// checkName, or a clear error if none matches.
func findCheck(checkName string) (checks.Check, error) {
	all, err := checks.List()
	if err != nil {
		return checks.Check{}, err
	}
	for _, c := range all {
		if c.Name == checkName {
			return c, nil
		}
	}
	return checks.Check{}, fmt.Errorf("no check named %q is configured", checkName)
}

// checkPrompt builds the prompt for running a single user-defined check
// against a single diff, embedding the check's own instructions verbatim.
func checkPrompt(diff string, c checks.Check) string {
	return fmt.Sprintf(
		"You are running a specific automated check against the following "+
			"diff. The check's instructions are:\n\n%s\n\nApply those "+
			"instructions to the diff below and report your findings in plain "+
			"prose. No preamble, no restating the diff, no unrelated "+
			"commentary.\n\n%s",
		c.Prompt,
		diff,
	)
}
