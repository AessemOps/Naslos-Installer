#!/usr/bin/env bash
#
# build-sidecar.sh - build the naslos-install engine and place it where Tauri
# expects the external binary:
#
#   desktop/src-tauri/binaries/naslos-install-<target-triple>
#
# The triple defaults to the host's rustc host triple; set TAURI_TARGET_TRIPLE
# and GOOS/GOARCH to build for another target (CI).
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

VERSION="${VERSION:-$(git -C "$root" describe --tags --always --dirty 2>/dev/null || echo dev)}"

mkdir -p "$root/desktop/src-tauri/binaries"
( cd "$root" && make build VERSION="$VERSION" )

dest="$root/desktop/src-tauri/binaries/naslos-install-${target}"
cp "$root/dist/naslos-install" "$dest"
chmod +x "$dest"
echo "build-sidecar: wrote desktop/src-tauri/binaries/naslos-install-${target}"
