# Magik Diff

A native git diff viewer with an LLM-powered explain-on-demand pane — for one file, all tracked changes, or a past commit. Human-in-the-loop becomes AI-powered-human-in-the-loop.

## Why?

Diff tools show you *what* changed. Magik Diff adds a right-hand pane that explains *why* it matters, scroll-locked to the diff and generated only when you ask for it — never automatically, never per-hunk. You still review and decide, the model just gets you there faster. It's read-only: it never stages, commits, or checks out anything.

## Build

Native desktop binary built with [Wails](https://wails.io) (Go backend + React frontend), so it must be built on the target OS.

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

No build script yet — build directly with Wails:

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
