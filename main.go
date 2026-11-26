package main

import (
	"embed"
	"fmt"
	"io"
	"log/slog"
	"os"
	goruntime "runtime"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

// version is shown by --version and the About dialog. It can be overridden at
// release build time via -ldflags "-X main.version=...". It is the single source
// of truth: after editing it, run `node scripts/sync-version.mjs`.
var version = "0.7.0"

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "--version", "-v":
			attachConsole()
			fmt.Println("mdiff " + version)
			return
		case "--help", "-h":
			attachConsole()
			printUsage(os.Stdout)
			return
		}
	}

	app := NewApp()

	title := "Magik Diff"
	if cwd, err := os.Getwd(); err == nil {
		title = cwd
	}

	// Launch the Wails application.
	err := wails.Run(&options.App{
		Title:     title,
		Width:     1024,
		Height:    768,
		MinWidth:  640,
		MinHeight: 480,
		// GNOME's window manager withholds all WM-driven interactive resize
		// (edge grips, Alt+drag) from undecorated windows, so a frameless
		// window can never be resized there regardless of app-side hit
		// detection; native decorations are the only reliable fix on Linux.
		Frameless: goruntime.GOOS != "linux",
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 26, G: 26, B: 28, A: 1},
		OnStartup:        app.startup,
		Bind: []any{
			app,
		},
	})

	if err != nil {
		slog.Error("wails run failed", "error", err)
	}
}

// printUsage writes the CLI help text to w.
func printUsage(w io.Writer) {
	fmt.Fprintln(w, "Magik Diff (mdiff)")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  mdiff                          launch the GUI")
	fmt.Fprintln(w, "  mdiff --version, -v            show version")
	fmt.Fprintln(w, "  mdiff --help, -h               show this help")
}
