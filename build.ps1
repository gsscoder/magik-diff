# Local build script. No CI yet (see memory-bank/detailed-spec/architecture.md section 1).
#
# Wails apps use CGO for the native webview binding, so a real cross-platform
# binary can't be produced from one machine with just GOOS/GOARCH — each OS
# needs its own native toolchain (CGO, WebView2/WebKit libs). This script
# builds for the OS it's run on; build darwin/linux by running this same
# command on those OSes.

$ErrorActionPreference = "Stop"

wails build

Write-Host ""
Write-Host "Built: build\bin\mdiff.exe"
Write-Host "To build for macOS/Linux, run 'wails build' on those OSes (CGO can't cross-compile the webview binding from Windows)."
