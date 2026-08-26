package main

import (
	"embed"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"

	"mdiff/internal/checks"
)

//go:embed all:frontend/dist
var assets embed.FS

// version is overridden at release build time via
// -ldflags "-X main.version=...", kept in sync with wails.json's
// info.productVersion.
var version = "0.1.6"

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "check":
			attachConsole()
			runCheckCommand(os.Args[2:])
			return
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

	// Create an instance of the app structure
	app := NewApp()

	title := "Magik Diff"
	if cwd, err := os.Getwd(); err == nil {
		title = fmt.Sprintf("%s | Magik Diff", cwd)
	}

	// Create application with options
	err := wails.Run(&options.App{
		Title:  title,
		Width:  1024,
		Height: 768,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 26, G: 26, B: 28, A: 1},
		OnStartup:        app.startup,
		Bind: []interface{}{
			app,
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}

// printUsage writes the CLI help text to w.
func printUsage(w io.Writer) {
	fmt.Fprintln(w, "Magik Diff (mdiff)")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  mdiff                          launch the GUI")
	fmt.Fprintln(w, "  mdiff check add <file.md>      install a check")
	fmt.Fprintln(w, "  mdiff --version, -v            show version")
	fmt.Fprintln(w, "  mdiff --help, -h               show this help")
}

// runCheckCommand handles the "mdiff check ..." CLI subcommand and exits
// the process; it never returns to the caller.
func runCheckCommand(args []string) {
	if len(args) != 2 || args[0] != "add" {
		fmt.Fprintln(os.Stderr, "usage: mdiff check add <path-to-md-file>")
		os.Exit(1)
	}

	src := args[1]
	if _, err := checks.ParseFile(src); err != nil {
		fmt.Fprintf(os.Stderr, "mdiff check add: %v\n", err)
		os.Exit(1)
	}

	dir, err := checks.Dir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "mdiff check add: %v\n", err)
		os.Exit(1)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "mdiff check add: %v\n", err)
		os.Exit(1)
	}

	dst := filepath.Join(dir, filepath.Base(src))
	if err := copyFile(src, dst); err != nil {
		fmt.Fprintf(os.Stderr, "mdiff check add: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("added check %q -> %s\n", filepath.Base(src), dst)
	os.Exit(0)
}

// copyFile copies the file at src to dst, overwriting dst if it exists.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}
