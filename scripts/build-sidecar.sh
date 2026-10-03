#!/usr/bin/env bash
#
# build-sidecar.sh - build the naslos-install engine and place it where Tauri
# expects the external binary:
#
#   desktop/src-tauri/binaries/naslos-install-<target-triple>[.exe]
#
# The triple defaults to the host's rustc host triple; set TAURI_TARGET_TRIPLE
# and GOOS/GOARCH to build for another target (CI). It builds the Go binary
# directly (no make) so it also runs on the Windows CI runner, and reads the
# pack's metadata.json to pin the Talos version / schematic the same way the
# Makefile does.
#
# Usage: scripts/build-sidecar.sh
set -euo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)

target="${TAURI_TARGET_TRIPLE:-}"
if [ -z "$target" ]; then
    if command -v rustc >/dev/null 2>&1; then
        target=$(rustc -vV | sed -n 's/^host: //p')
    else
        echo "build-sidecar: rustc not found; set TAURI_TARGET_TRIPLE" >&2
        exit 1
    fi
fi
if [ -z "$target" ]; then
    echo "build-sidecar: could not determine the target triple" >&2
    exit 1
fi

version="${VERSION:-$(git -C "$root" describe --tags --always --dirty 2>/dev/null || echo dev)}"

meta="$root/installpack/metadata.json"
talos="${TALOS_VERSION:-$(sed -n 's/.*"talosVersion"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "$meta" 2>/dev/null)}"
schematic="${SCHEMATIC_ID:-$(sed -n 's/.*"schematicId"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "$meta" 2>/dev/null)}"
# Fallbacks for a checkout with no fetched pack (the constants the Makefile uses).
[ -n "$talos" ] || talos=v1.14.1
[ -n "$schematic" ] || schematic=4dd8e3a8b6203d3c14f049da8db4d3bb0d6d3e70c5e89dfcc1e709e81914f63c

ext=""
case "$target" in
    *windows*) ext=".exe" ;;
esac
dest="$root/desktop/src-tauri/binaries/naslos-install-${target}${ext}"
mkdir -p "$root/desktop/src-tauri/binaries"

(
    cd "$root"
    go build -trimpath \
        -ldflags "-X main.Version=${version} -X main.ExpectedTalosVersion=${talos} -X main.ExpectedSchematicID=${schematic}" \
        -o "$dest" ./cmd/naslos-install
)
chmod +x "$dest" 2>/dev/null || true
echo "build-sidecar: wrote desktop/src-tauri/binaries/naslos-install-${target}${ext}"
