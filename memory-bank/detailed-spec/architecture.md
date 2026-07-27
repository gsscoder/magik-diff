# Magick Diff — Architecture Spec (pre-build, AI-only)

Status: nothing built yet. Audience: AI coding agent implementing v1. Not a human-facing doc, not a tutorial. Purpose: fix decisions and rationale so a future agent does not re-litigate them or silently revert them to a more "obvious"-looking default.

Scope note: do not infer a file/directory tree, function names, route paths, or diff/hunk JSON field names from this doc. Those are first-implementation decisions, left open. Exception: names that are themselves the decision are fixed, not illustrative — `base_url`/`model`/`api_key` (section 6), `config.json`, `OPENAI_API_KEY`, `DESIGN.md` (section 8).

What this is: a git diff viewer. Differentiator: point it at an OpenAI-compatible endpoint, ask an LLM to explain a diff — one file, all tracked changes, or a past commit.

Anti-references (negative constraints, not asides): GitHub Desktop, LazyGit. Do not converge on their layout, interaction model, or feature set (no repo picker, no staging workflow parity) as a safe default.

## Decision status legend
- LOCKED: confirmed via AskUserQuestion. Do not silently revise. Overturning requires a new explicit user decision, not agent judgment.
- DEFAULT: proposed unilaterally, user notified, user did not veto. Real commitment already communicated. Ask before overturning; may proceed without asking only if the same technical reason recurs concretely during implementation, not preemptively.
- OPEN: not decided anywhere in this doc or the source brainstorm. Confirm with the user before implementing. Do not treat as settled and do not invent a value silently.

## 1. Platform and distribution
Status: LOCKED
Decision: Go backend, Wails as the native-shell/binding layer, React for the UI.
Revision note: this supersedes a prior LOCKED decision ("Go, web UI embedded via `embed.FS`, plain HTML/JS/CSS, no npm/bundler"). Overturned via new explicit user decision, per the legend's own escape hatch for LOCKED items, not via agent judgment. Do not revert to the embed.FS/plain-HTML approach.
Rationale: Wails avoids Electron's bundled-Chromium bloat by binding to the OS's own webview (WebView2 on Windows, WebKit on macOS/Linux) instead of shipping a browser engine per platform. React is adopted because it is where prior UI work has shown strong results, unlike a plain-HTML approach. Wails' Go↔JS surface is small and well-trodden: a Go struct's exported methods auto-bind to JS, plus an events bus for push-from-Go — not an exotic or bespoke IPC layer.
Trade-off, accepted knowingly, not an oversight: this gives up the previous decision's "CGO-free, zero-runtime-dep, true static cross-compile" property. Wails requires CGO and a per-OS webview runtime present at run time — on Windows, the WebView2 redistributable is not bundled into the binary, it is an install-time/runtime dependency, the same category of thing the original decision explicitly avoided by rejecting Electron/Tauri. A future agent must not "helpfully" strip the CGO dependency or revert to `embed.FS` thinking this was unintentional; it was weighed and accepted.
Rejected alternatives, do not re-propose without new information:

| Alternative | Why rejected |
|---|---|
| go-git library | Reimplements git's own bugs. Shell out to real `git` via `os/exec` instead (`git diff`, `git show`, `git log --format`). Git CLI is the stable reference implementation. |
| Electron / Tauri | Bundle a full browser engine per platform (Electron: Chromium; Tauri: also CGO+webview but a different binding model), installers, 100MB+ artifacts for Electron, cross-build matrix pain. Wails is distinct from both: it reuses the OS-native webview like Tauri rather than bundling a browser engine like Electron, which is the specific property that makes it acceptable here. |
| Bun `--compile` | Works, but ~55MB/binary and still needs a per-target cross-build. No advantage over Go here. |
| Python + PyInstaller | Antivirus false positives, slow cold start, needs a builder machine per target OS. |
| Native multi-provider LLM adapters (separate OpenAI/Anthropic/Gemini client code) | Rejected in favor of "any OpenAI-compatible endpoint" (section 6). One endpoint shape covers what would otherwise be three request-shape code paths. |

Decision: distribution before public release is local builds only, no CI.
Detail: build mechanism is `wails build`, Wails' own CLI, which itself cross-compiles per target — not three raw `go build` invocations. Testers get the binary directly.
Deferred, not forgotten: GitHub Actions release matrix, GoReleaser. Do not add CI/release automation unprompted.

## 2. Scope boundaries
Status: LOCKED
Decision: read-only forever. The tool never stages, commits, or checks out. It only ever runs read commands: `git diff`, `git show`, `git log`, `git status`.
Rationale: halves the surface area, removes the need for confirmation dialogs or destructive-action safety UX entirely.
Rejected, both explicitly offered to the user and declined, not merely postponed: "read-only v1, add staging later"; "staging + commit support." Do not add stage/unstage/commit as a natural next step.

Decision: cwd-only repo targeting. The binary serves whatever git repo it is launched inside. No repo picker, no recents list, no multi-repo state in one running instance.
Rejected: GitHub-Desktop-style repo picker. GH Desktop is the named anti-pattern for this project.

## 3. Layout
Status: LOCKED — "Layout A, synced narrative"
Decision: three regions — a dual-purpose left rail, unified diff center, AI prose right pane. Scroll-locked: hovering an AI paragraph highlights its source hunk.
Left rail is one region, not two. Contents switch by top-level mode tab (Working tree / Staged / History): file list in working-tree/staged modes, commit list in history mode.
Rejected for the rail:

- Permanent two-rail layout (commit rail + file rail simultaneously): too cramped under 1400px viewport width.
- Ctrl+K-only palette navigation: insufficiently discoverable. "See what changed at a glance" was a stated requirement; a palette hides the list behind a keystroke.

Rejected layout concepts, legitimate, not strawmen. Do not resurrect casually for v1. Do not treat as permanently impossible either.

| Concept | Shape | Why rejected for v1 |
|---|---|---|
| Story feed | No file tree. Cards ordered by AI-judged blast radius, not alphabetically. Scrolling top-to-bottom is the review. | Bigger swing than Layout A. Could become an alternative view later if requested. |
| Ask bar / REPL | Thin commit rail + diff + persistent chat input bar at the bottom, toggleable scope chips (all tracked / `file.go` / commit sha) for conversational follow-ups. | Rejected as primary layout for v1. Compatible/complementary: an ask bar can bolt onto Layout A later without restructuring it, because Layout A's rail already encodes the same scope concept (file / all-tracked / commit) the chips would express. |

## 4. Diff rendering
Status: LOCKED
Decision: unified diff format, not side-by-side.
Rationale: three regions (rail, diff, prose) already compete for width. Side-by-side would squeeze diff columns to ~300px gutters.

Status: LOCKED (that this feature exists at all)
Decision: word-level intraline highlighting of the exact changed substring within a changed line, not just whole-line red/green.

Status: DEFAULT, see section 8. Do not read the section header above as promoting this sub-decision to LOCKED.
Decision: word-level diff algorithm is common-prefix/suffix trim, explicitly not full LCS/Myers.
Rationale: deliberate simplification. Catches the common single-token-change case in ~10 lines.
Upgrade path: swap in a real LCS diff if word-level highlighting on complex multi-token line edits proves inadequate in practice. Do not preemptively build the general algorithm now.

Status: LOCKED
Decision: hunk parsing happens server-side, in Go. The browser never parses raw diff text; it receives structured JSON conveying file path, hunk header, old/new line ranges, word-level highlight spans — concept only, exact field names not fixed here.
Rationale: keeps diff-format edge cases (renames, binary files, no-newline-at-eof) in one place, Go, testable against real git fixtures, instead of duplicated in JS.

## 5. LLM interaction model
Status: LOCKED
Decision: trigger is on-demand only, never automatic. Nothing calls the LLM until the user explicitly clicks Explain.
Rationale: explicit user correction mid-brainstorm. Auto-explain-on-select was offered and rejected specifically because a stray click on a 5k-line diff is expensive.

Decision: caching is bundled with on-demand as one decision, not two. Results cached keyed by hash of `(diff content + model + prompt version)`. Re-selecting an already-explained file/hunk is free and instant.
Rationale for bundling: on-demand only actually saves money/latency if re-visiting the same diff doesn't re-pay the cost.

Decision: per-hunk explain and explain-all-hunks-in-a-file both must exist, as one mechanism. User explicitly required both entry points. Explain-all is one batched LLM call per file, not N parallel per-hunk calls, not a single non-hunk-anchored essay.
Mechanism: send the whole file's diff once. Model returns prose keyed per-hunk, so each hunk still gets its own anchored paragraph in the right pane. Preserves hover-to-highlight.

| Rejected shape | Why |
|---|---|
| N parallel calls, one per hunk | N times the cost of one batched call. Hunks explained blind to each other, losing cross-hunk context, e.g. "this pairs with hunk 3." |
| Single whole-file essay, no per-hunk anchor | Breaks the hover-to-highlight interaction that is the entire point of Layout A. |

Decision: cross-file/cross-commit summary, "explain everything" across all tracked changes or a whole commit, renders as a pinned summary card at the top of the same right-hand prose pane used for per-file explain. Not a separate view/screen. Not rail tooltips. One pane, one mental model: user scrolls down from the summary card into the same per-hunk prose that per-file explain produces.

| Rejected shape | Why |
|---|---|
| Separate overview screen | Extra view to build and navigate. Contradicts the point of Layout A: one continuous reading surface. |
| Rail tooltips | Rail is ~200px wide, no room for real prose. |

## 6. LLM provider
Status: LOCKED
Decision: any OpenAI-compatible endpoint. Config surface: `base_url` + `model` + `api_key`, defaulting to OpenAI's endpoint.
Rationale: deliberately more general than "OpenAI only," which was offered and rejected. Costs one extra config field, gets Ollama / OpenRouter / local models for free.
Detail: `base_url` doubles as the test seam. Tests point it at an `httptest.Server` instead of mocking an SDK client, see section 9. No SDK dependency for the LLM call — a raw HTTP POST to the chat-completions-shaped endpoint.

## 7. Cost guard on large diffs
Status: LOCKED that both controls below are required together. User framing: "on-demand only matters if you can see what it costs."
Control 1: byte cap on what gets sent to the LLM, with a visible `[truncated]` marker when the cap is hit. No silent truncation.
Cap value: OPEN. Not pinned anywhere in this doc or the source brainstorm. A future agent must propose a value and confirm it with the user before implementing. Do not silently pick a number and treat it as settled.

Control 2: pre-click token estimate shown on the Explain button itself, e.g. "~14k tokens," before the user commits to the call.
Formula: `len(diff) / 4`. Status: DEFAULT, see section 8. No real tokenizer dependency, deliberately approximate. A UI cost hint, not a billing-accurate count.
Rejected: silent truncation with no visible marker. No cap at all — one click on a vendored/lockfile-heavy diff would blow both context window and API budget.

## 8. Defaults
Status: DEFAULT unless noted otherwise. Proposed unilaterally, user proceeded. See legend.

- Word-level diff via common-prefix/suffix trim. Rationale in section 4. Do not duplicate reasoning here.
- Token estimate via `len(diff)/4`, no tokenizer library. Rationale in section 7.
- LLM response streaming to the UI via Server-Sent Events. Prose appears token-by-token rather than waiting for the full response.
- Prose rendering: plain paragraphs plus inline `` `code` `` spans only. No markdown parser/library, no headings/lists/tables in AI output. If the model emits markdown-like syntax beyond inline code spans, it is not specially rendered. Rationale: OPEN. Not recoverable from the source brainstorm. A future agent must not assume a reason and must not "fix" this to full markdown rendering without confirming with the user first. Full markdown rendering is the more obvious default here and is exactly the kind of silent reversal this doc exists to prevent.
- No filesystem watcher for live-refresh. Refreshing the working-tree view is a manual button click. Rationale: OPEN. Not recoverable from the source brainstorm. Confirm with the user before adding a watcher, and before treating "no watcher" as a settled, permanent decision.
- Cache storage: JSON file under `os.UserCacheDir()`. Key is `sha256(diff + model + prompt_version)`. Prompt version is included in the key deliberately: changing the prompt template invalidates old cached explanations rather than serving stale prose under a new prompt silently.
- Config storage: `OPENAI_API_KEY` environment variable wins if set. Otherwise a config file at `os.UserConfigDir()/magik-diff/config.json`, written with `0600` permissions since it contains the API key and must not be world-readable. No in-app settings/onboarding screen for v1: config is edit-the-file or set-the-env-var.
- History rail, top tab "History": loads the most recent 200 commits via `git log`, with load-more-on-scroll for older history rather than loading full history upfront. Note: this 200 figure is pinned as a specific default, unlike the byte cap in section 7, which is left OPEN. Both are equally arbitrary magic numbers; only this one was actually proposed and not vetoed. Do not infer that the byte cap should also default to some derived value just because this one is pinned.
- Single human-facing design doc, `DESIGN.md`, separate from this AI-only file. User explicitly did not want documentation scattered across multiple files.

Local server security. Status: LOCKED (empirically resolved, not a judgment call).

Original mitigation, designed for the superseded model (Go binary runs a `net/http` server, user's regular browser opens a tab pointed at localhost):
- Server binds to `127.0.0.1` only, never `0.0.0.0`.
- Random port chosen per launch.
- Every request must carry a random per-launch token embedded in the URL the browser is opened with. Every handler validates it.
- Rationale under that model: without the token, any other web page open in the user's browser could POST to `http://127.0.0.1:port/...` from client-side JS and silently spend the user's configured LLM API budget. A same-origin-policy-adjacent localhost CSRF-like attack. Binding to loopback alone does not prevent this since any tab can still reach `127.0.0.1`. The token was the actual mitigation; the loopback bind was defense-in-depth against network-level exposure. This was a real vulnerability class, not paranoia.

Finding: empirically tested, not theorized. Built the real `mdiff.exe` via `wails build`, launched it, inspected `Get-NetTCPConnection -State Listen` before and after launch for both `mdiff.exe` and its child process `msedgewebview2.exe` (Wails on Windows spawns a separate WebView2 host process as a child). Result: zero new listening TCP ports for either process. The Go↔JS/webview channel is a named pipe (visible in the child process command line as `--mojo-named-platform-channel-pipe=<id>`), not a TCP socket.

Conclusion: the original localhost-CSRF token mitigation (127.0.0.1 bind, random port, per-launch URL token), designed for a `net/http`-server-plus-browser-tab model, is NOT NEEDED under Wails' actual transport. There is no listening port for another tab or process to reach. Do not carry that mitigation into implementation.

Caveat, do not overclaim: verified on Windows only, the test environment. macOS/Linux use different WebView2-equivalent backends (WebKit-based) and their IPC transport was not independently verified. If a future agent implements on macOS/Linux, spot-check the same "no listening port" assumption there rather than blindly assuming it is identical — though there is no specific reason to expect it differs, since Wails' architecture is transport-abstracted across platforms.

## 9. Testing approach
Status: decision made, not yet executed.

- Git layer: exercise real `git` against a real temp repo, `t.TempDir()` plus `git init` plus real commits as fixtures, rather than mocking git behavior. Mirrors the test-the-real-prior-layer-not-mocks philosophy for anything that shells out.
- LLM layer: point the `base_url` config at an `httptest.Server`. Tests the real HTTP-call code path, request shape, SSE streaming, error handling, without hitting a real API and without a mocking framework or SDK test-double.

## 10. Explicitly out of scope for v1
Do not build unprompted. Do not treat absence as a bug.

- Authentication.
- Multi-repo support within one running instance.
- Any git write operation: stage, unstage, commit, checkout.
- In-app settings/onboarding UI.
- Diff virtualization or streaming diff rendering for very large files.
- CI/release automation: GitHub Actions matrix, GoReleaser.

## Open items requiring user confirmation before implementation
- Byte cap value for the cost guard, section 7. No number pinned anywhere.
- Rationale for no-markdown-rendering, section 8. Decision itself is DEFAULT and stands; the why is unrecoverable and unconfirmed.
- Rationale for no-filesystem-watcher, section 8. Decision itself is DEFAULT and stands; the why is unrecoverable and unconfirmed.

## Cross-reference notes for the implementing agent
- Section 4's word-level trim algorithm and section 7's token estimate each appear once, in their most relevant section. Do not re-derive separate rationale for them if referenced from section 8; the section 8 entries point back here intentionally.
- The Ask bar / REPL rejected-layout entry, section 3, is compatible with Layout A specifically because Layout A's rail already carries the scope concept (file / all-tracked / commit) the ask bar's scope chips would otherwise introduce. If an ask bar is built later, reuse that existing scope concept rather than inventing a parallel one.
- Nothing in this document specifies package names, route paths, diff/hunk JSON field names, or a source directory layout. Those are first-implementation decisions; make them when writing the first code, not by inference from this spec. Exception: section 6's config surface (`base_url`/`model`/`api_key`) and section 8's storage names (`config.json`, `OPENAI_API_KEY`, `DESIGN.md`) are fixed