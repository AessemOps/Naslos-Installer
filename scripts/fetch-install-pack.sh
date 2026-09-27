#!/usr/bin/env bash
#
# fetch-install-pack.sh - download the versioned Naslos install pack published
# by the Naslos-Linux release CI, verify its sha256, and extract it into
# ./installpack/ (which is go:embed-ed and gitignored).
#
# The pack is produced by `make install-pack` in AessemOps/Naslos-Linux and
# attached to a `v<version>` GitHub release:
#   naslos-install-pack-<version>.tar.gz
#   naslos-install-pack-<version>.tar.gz.sha256
#
# Environment:
#   PACK_VERSION   required; the release version (e.g. 0.1.0)
#   PACK_REPO      owner/repo that publishes the pack (default AessemOps/Naslos-Linux)
#   PACK_BASE_URL  override the download base (default GitHub releases)
#   PACK_SHA256    optional; expected tarball sha256 (also accepted as $1)
#
# Usage: PACK_VERSION=0.1.0 scripts/fetch-install-pack.sh [sha256]
set -euo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
PACK_VERSION="${PACK_VERSION:-}"
PACK_REPO="${PACK_REPO:-AessemOps/Naslos-Linux}"
PACK_SHA256="${PACK_SHA256:-${1:-}}"

if [ -z "$PACK_VERSION" ]; then
    echo "fetch-install-pack: PACK_VERSION is required (e.g. PACK_VERSION=0.1.0)" >&2
    exit 1
fi

name="naslos-install-pack-${PACK_VERSION}.tar.gz"
base="${PACK_BASE_URL:-https://github.com/${PACK_REPO}/releases/download/v${PACK_VERSION}}"
url="${base}/${name}"

for tool in curl sha256sum tar; do
    command -v "$tool" >/dev/null 2>&1 || { echo "fetch-install-pack: missing '$tool'" >&2; exit 1; }
done

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

echo "fetch-install-pack: downloading ${url}"
curl -fsSL -o "$tmp/$name" "$url"

# Prefer a caller-supplied checksum; otherwise fetch the published .sha256.
if [ -z "$PACK_SHA256" ]; then
    if curl -fsSL -o "$tmp/$name.sha256" "${url}.sha256" 2>/dev/null; then
        PACK_SHA256=$(awk '{print $1; exit}' "$tmp/$name.sha256")
    else
        echo "fetch-install-pack: no .sha256 published and PACK_SHA256 unset; refusing an unverified pack" >&2
        exit 1
    fi
fi

actual=$(sha256sum "$tmp/$name" | awk '{print $1}')
if [ "$actual" != "$PACK_SHA256" ]; then
    echo "fetch-install-pack: checksum mismatch for $name" >&2
    echo "  expected: $PACK_SHA256" >&2
    echo "  actual:   $actual" >&2
    exit 1
fi
echo "fetch-install-pack: sha256 verified ($actual)"

# Replace the embedded pack contents, keeping the tracked placeholder + embed.go.
dest="$root/installpack"
mkdir -p "$dest"
find "$dest" -mindepth 1 -maxdepth 1 \
    ! -name README.md ! -name embed.go -exec rm -rf {} +
tar -xzf "$tmp/$name" -C "$dest" --strip-components=1

[ -f "$dest/metadata.json" ] || { echo "fetch-install-pack: extracted pack has no metadata.json" >&2; exit 1; }
echo "fetch-install-pack: extracted pack ${PACK_VERSION} into $dest"
