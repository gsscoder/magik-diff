# Local Windows build script. No CI yet (see memory-bank/detailed-spec/architecture.md section 1).
# Build on other OSes by running `wails build` there directly (Wails apps use
# CGO for the native webview binding, so this can't cross-compile them).

$ErrorActionPreference = "Stop"

wails build

Write-Host ""
Write-Host "Built: build\bin\mdiff.exe"
