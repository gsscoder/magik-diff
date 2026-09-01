// Package gitexec runs the real git binary as a subprocess. It is the sole
// place in the codebase that spawns git, shared by internal/gitdiff and
// internal/watch so the process-hiding and error-wrapping logic exists once.
package gitexec

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// Error is returned by Run when the git subprocess exits with a non-zero
// status. It carries the exit code and stderr text as distinct fields, so
// callers can use errors.As to inspect the actual failure (e.g. git's
// stderr message) instead of substring-matching Run's combined,
// human-oriented Error() string.
type Error struct {
	Args     []string
	ExitCode int
	Stderr   string
	err      error
}

func (e *Error) Error() string {
	return fmt.Sprintf("git %s: %v: %s", strings.Join(e.Args, " "), e.err, e.Stderr)
}

func (e *Error) Unwrap() error { return e.err }

// Run runs git with args, rooted at dir (the empty string means the current
// working directory), and returns its stdout. On failure it returns an
// *Error including git's stderr output.
func Run(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := Command(ctx, dir, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		exitCode := -1
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		}
		return "", &Error{Args: args, ExitCode: exitCode, Stderr: strings.TrimSpace(stderr.String()), err: err}
	}
	return stdout.String(), nil
}

// Command builds a git subprocess for args, rooted at dir (the empty string
// means the current working directory), with console-window hiding applied
// on Windows. It is exported for callers that need custom exit-code
// handling (e.g. `git diff --no-index`, which uses diff(1)-style exit
// codes) instead of Run's treat-any-error-as-failure behavior.
func Command(ctx context.Context, dir string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	hideConsole(cmd)
	return cmd
}
