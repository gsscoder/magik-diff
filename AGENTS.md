# Project Overview & AI Instructions

## Project Brief
A git diff viewer that sets itself apart with on-demand LLM diff explanations — one file, all tracked changes, or a past commit. Permanently read-only: never stages, commits, or checks out. Explicitly not shaped like GitHub Desktop or LazyGit — do not drift toward their layout or feature set as a safe default.

## Core Technologies
Go + Wails (native desktop shell, OS webview, no bundled Chromium), React (frontend), `os/exec` (shells out to real `git`), any OpenAI-compatible HTTP endpoint (no LLM SDK)

## Layers
- `app.go`, `main.go` — Wails bootstrap and bound methods exposed to the frontend (the per-call entry points)
- `internal/gitdiff` — `Repo`-scoped git operations (no process-global cwd), parses raw diff/log/status output
- `internal/gitexec` — shared context-aware git-invocation plumbing (used by `gitdiff` and `watch`)
- `internal/diffparse` — hunk/line structuring and word-level intraline diffing
- `internal/llm` — OpenAI-compatible HTTP client for the Explain/Explain-All calls
- `internal/explain` — prompt construction and Explain/RunCheck orchestration, plus the in-memory explain cache
- `internal/checks` — user-defined diff checks (loaded from `checks/*.md`)
- `internal/config` — settings and API key persistence
- `internal/watch` — filesystem watcher that emits `repo:changed` events for background UI refresh
- `frontend/src` — React UI (`App.tsx` plus `FileList`/`DiffPane`/`ExplanationPane`: left rail, center diff, right AI pane)

## Architecture
Single native desktop binary: Go backend exposing bound methods to a React UI via Wails' Go↔JS bridge (not an HTTP server opened in a browser tab):
- Left rail (dual-purpose) — file list in Working tree/Staged mode, commit list in History mode; one region, contents switch by tab
- Center — unified diff with word-level intraline highlighting (common-prefix/suffix trim)
- Right pane — AI prose. Scroll-lock to the diff and hover-to-highlight-source-hunk are planned but not yet implemented
- Explain is on-demand only (never automatic), cached by hash of (diff + model + prompt version) in an in-memory map — not persisted across restarts; explain-all is one batched call per file/commit, not N per-hunk calls
- Hunk parsing happens server-side in Go; React only ever receives structured data, never raw diff text
- Accepted trade-off: CGO and a per-OS webview runtime (e.g. WebView2 on Windows, not bundled in the binary) are now required — the prior plain-Go build was CGO-free with zero runtime deps; Wails was chosen anyway for the native-app UX and to use React
- Full decision record and rejected alternatives: `memory-bank/architecture.md`

## Maturity
- The system is currently under development
- Backward compatibility is not required when changing existing features

## Memory
The development documents are in the `memory-bank` dir — they primarily focus on specific feature implementation details

## Output
- Code: match the architectural and stylistic conventions of the existing codebase
- Language: use English for all generated artifacts and symbols by default. Content in another language is allowed only in user-facing strings, messages, and labels when the application has a single localization
- Quality: production-grade — every line will be reviewed
- Markdown: compact, no linting compliance, formatting identical to this file

### Go Code Standards
- Generate production-grade Go (1.23+) code, favoring idiomatic, safe, and performant patterns over legacy or over-engineered abstractions
- Avoid `panic` for expected errors; wrap with `fmt.Errorf` using `%w` and check with `errors.Is`/`errors.As`
- Do not spawn unmanaged goroutines; use `errgroup` for lifecycles and always pass `context.Context` as the first parameter
- Avoid premature interface definitions; accept interfaces and return structs, defining them only in the consuming package
- Always preallocate slices and maps with `make` when capacity is known; mutate via index in `range` loops to avoid copy pitfalls
- Avoid `interface{}` and legacy `log`; use `any`, `slices`/`maps` packages, and `log/slog` for modern, type-safe operations

### TypeScript Code Standards
- Generate code for Node.js ESM (ES2022) with TypeScript 6, favoring modern and expressive syntax over legacy patterns
- Avoid `any`; use `unknown` for uncertain types and narrow before use
- Do not use non-null assertion (`!`); handle `null`/`undefined` explicitly
- Avoid `object`/`{}`/`Record<string, any>`; use specific interfaces or type aliases
- Avoid wide unions; use discriminated unions with `type`/`kind` for explicit handling
- Avoid `as` casting; use type guards (`is`) for safe narrowing

## Operational Rules
- If you're Claude Code, you may have specialized subagents available for many use cases — check `.claude/agents/` and prefer delegating to a matching one over doing the work directly
- Read files in `memory-bank` only when required by the current task; scan filenames first and read file contents only if they are relevant to the task
- Review/audit/report requests end at the report; fixing findings needs its own separate request — authorization never carries across turns
- NEVER update `AGENTS.md` without an explicit request
- NEVER modify `*.md` files in `memory-bank` (at any depth in the dir tree) without an explicit request
- NEVER initiate any codebase modifications without an explicit request
- NEVER commit changes to Git history without explicit authorization

## Guardrails
These are hard constraints, not suggestions, and they bias toward caution over speed — scale their application proportionately for trivial or throwaway tasks. When a request conflicts with a guardrail, follow the guardrail and say why.

### Before Implementing — Reason First
- Surface Uncertainty, Don't Guess Through It: When requirements are ambiguous, contradictory, or incomplete, stop and ask rather than assuming intent and proceeding silently. State assumptions explicitly; if multiple readings are viable, present them instead of silently choosing one. Resolving intent up front is what makes autonomous execution safe afterward.
- Plan Before Implementing: For non-trivial tasks, outline a brief approach before writing code so wrong directions surface early. For multi-step work, list the steps with a verification check for each

### Design & Scope — Code Minimally
- Simplicity Over Abstraction: Write the simplest solution that meets the requirements. Avoid speculative features, single-use abstractions, unrequested configurability, and error handling for impossible cases. If a construction could be materially shorter without sacrificing correctness, rewrite it — ask whether a senior engineer would call it overcomplicated.
- Surgical Scope: Every changed line must trace directly to the current task. Match the surrounding style even where you would choose differently. Remove imports, variables, and comments that *your* changes made obsolete, but never modify, reformat, or delete code or comments unrelated to the task. If you spot unrelated dead code, mention it — don't delete it.

### Execution — Verify Against Goals
- Drive Toward Success Criteria: Turn the task into checkable goals and work until they are met — e.g. "add validation" → write tests for invalid inputs, then make them pass; "fix the bug" → write a failing test reproducing it, then make it pass. When the goal is clear, loop and self-verify rather than pausing for confirmation the criteria already answer. (Counterpart to *Surface Uncertainty*: clarify the goal before starting; do not reopen a settled goal mid-execution.)

### Collaboration — Communicate Honestly
- Honesty Over Agreement: Push back on questionable requests and defend sound technical choices rather than complying by default; avoid reflexive agreement.
- Signal Confidence Level: Indicate when a solution is a best guess versus a well-established approach, so review effort can be calibrated. (Complements *Surface Uncertainty*: if you could not proceed at all, ask; if you proceeded on a judgment call, flag it.)