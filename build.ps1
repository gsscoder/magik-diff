# Local build script. No CI yet (see memory-bank/detailed-spec/architecture.md section 1).
#
# Wails apps use CGO for the native webview binding, so a real cross-platform
# binary can't be produced from one machine with just GOOS/GOARCH — each OS
# needs its own native toolchain (CGO, WebView2/WebKit libs). Requesting a
# platform other than the one this script runs on WILL attempt the build, but
# it is expected to fail without that OS's own toolchain present — that's not
# a bug in this script, it's the CGO cross-compile limitation. Build each OS
# by running this same command on that OS.
#
# Usage:
#   .\build.ps1 -Platform windows
#   .\build.ps1 -Platform linux
#   .\build.ps1 -Platform darwin
#   .\build.ps1 -Platform all

param(
    [Parameter(Mandatory = $true)]
    [ValidateSet("windows", "linux", "darwin", "all")]
    [string]$Platform
)

$platforms = if ($Platform -eq "all") { @("windows", "linux", "darwin") } else { @($Platform) }

$results = @{}
foreach ($p in $platforms) {
    Write-Host "=== Building $p/amd64 ==="
    $output = wails build -platform "$p/amd64" 2>&1 | Tee-Object -Variable buildOutput
    $output | ForEach-Object { Write-Host $_ }
    if ($LASTEXITCODE -ne 0) {
        $results[$p] = "FAILED (exit $LASTEXITCODE)"
    } elseif (($buildOutput -join "`n") -match "not currently supported") {
        $results[$p] = "UNSUPPORTED (Wails cross-compile not supported from this host OS)"
    } else {
        $results[$p] = "OK"
    }
    Write-Host ""
}

Write-Host "--- Summary ---"
foreach ($p in $platforms) {
    Write-Host "$p/amd64 : $($results[$p])"
}
Write-Host ""
Write-Host "Binaries (on success): build\bin\"
Write-Host "A platform other than the host OS failing here is expected without that OS's own CGO toolchain — build it by running this script on that OS instead."
