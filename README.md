# Magik Diff

![version](https://img.shields.io/badge/version-0.4.0-blue)

A native git diff viewer with an on-demand LLM explanation pane, covering a single file, all tracked changes, or any past commit. You stay in the review loop; the model just gets you there faster.

![Demo](docs/demo.gif)

## Why?

Diff tools show *what* changed. Magik Diff adds a right-hand pane that explains *why* it matters, and only when you ask for it — never automatically, never per hunk. Review and judgment stay yours; the model shortens the path to them. The tool is strictly read-only: it never stages, commits, or checks anything out.

## Split view

Each file's diff renders as unified (`+`/`-` inline) or split (old file on the left, new on the right, changes tinted). The Unified/Split toggle sits next to the file name and persists across files.

## Shortcuts

| Keys | Action |
| --- | --- |
| `Ctrl/Cmd` + `M` | Open model config |
| `Ctrl/Cmd` + `B` | Show/hide the Changes/History pane |
| `Ctrl/Cmd` + `+` / `-` | Zoom in / out |
| `Ctrl/Cmd` + `0` | Reset zoom |
| `F11` | Toggle zen mode |
| `Esc` | Close open menu |

## Checks

Beyond the built-in explanation, you can run your own checks against any diff or commit, each as an independent LLM call with its own prompt. A check is a Markdown file with a front matter block (`name`, `description`, `color`) and a prompt body; see `checks/language-consistency.md` for a working example. Install one via the CLI:

```sh
mdiff check add path/to/my-check.md
```

Installed checks appear as buttons beside Explain, with scrollable result panels.

## CLI

```sh
mdiff                          launch the GUI
mdiff check add <file.md>      install a check
mdiff --version, -v            show version
mdiff --help, -h               show this help
```

## Build

A native desktop binary built with [Wails](https://wails.io) (Go backend, React frontend). It must be compiled on the target OS.

### Windows

```sh
./build-win.ps1
```

Builds `build/bin/mdiff.exe`.

### Linux

```sh
./build-linux.sh
```

Builds `build/bin/mdiff` (amd64, webkit2_41).

### macOS

No build script yet; use Wails directly:

```sh
wails build
```

## Requirements

- Go >= 1.25
- [Wails CLI](https://wails.io/docs/gettingstarted/installation)
- Node.js (for the frontend build, invoked automatically by Wails)
- An OpenAI-compatible HTTP endpoint for the explain feature (any provider, no SDK lock-in)

## License

MIT © [koder0x](https://github.com/gsscoder)
