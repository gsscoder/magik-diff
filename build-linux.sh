#!/usr/bin/env bash
# Linux build script (bash won't run on Windows — use build.ps1 there).
# Local build only. No CI yet (see memory-bank/detailed-spec/architecture.md section 1).
#
# Usage:
#   ./build-linux.sh

set -eu

echo "=== Building linux/amd64 ==="
wails build -platform linux/amd64 -tags webkit2_41

echo ""
echo "Binary: build/bin/"
