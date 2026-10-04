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
#   PACK_VERSION   release version (e.g. 0.1.0). When empty, the newest
#                  vX.Y.Z tag on PACK_REPO is resolved and used.
#   PACK_TAG       the release tag to download from (default: v<PACK_VERSION>)
#   PACK_REPO      owner/repo that publishes the pack (default AessemOps/Naslos-Linux)
#   PACK_REPO_URL  git URL used to resolve the newest tag (default the HTTPS URL)
#   PACK_BASE_URL  override the download base (default GitHub releases)
#   PACK_SHA256    optional; expected tarball sha256 (also accepted as $1)
#
# Usage: scripts/fetch-install-pack.sh                 # newest vX.Y.Z tag
#        PACK_VERSION=0.1.0 scripts/fetch-install-pack.sh [sha256]
set -euo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
PACK_VERSION="${PACK_VERSION:-}"
PACK_TAG="${PACK_TAG:-}"
PACK_REPO="${PACK_REPO:-AessemOps/Naslos-Linux}"
PACK_REPO_URL="${PACK_REPO_URL:-https://github.com/${PACK_REPO}.git}"
PACK_SHA256="${PACK_SHA256:-${1:-}}"

# resolve_latest_tag prints the highest vX.Y.Z tag on PACK_REPO. Only strict
# semantic tags are considered, so a moving `latest` tag can never be picked.
resolve_latest_tag() {
    command -v git >/dev/null 2>&1 || { echo "fetch-install-pack: git is required to resolve the latest tag" >&2; return 1; }
    git ls-remote --tags --refs --sort=-v:refname "$PACK_REPO_URL" 'v*' 2>/dev/null \
        | awk '{print $2}' \
        | sed 's#^refs/tags/##' \
        | grep -E '^v[0-9]+\.[0-9]+\.[0-9]+$' \
        | head -n1
}

if [ -z "$PACK_VERSION" ]; then
    if [ -z "$PACK_TAG" ]; then
        PACK_TAG="$(resolve_latest_tag || true)"
    fi
    if [ -z "$PACK_TAG" ]; then
        echo "fetch-install-pack: no vX.Y.Z tag found in ${PACK_REPO}; set PACK_VERSION or PACK_TAG" >&2
        exit 1
    fi
    PACK_VERSION="${PACK_TAG#v}"
fi
# An explicit PACK_VERSION without a tag means the conventional v<version> tag.
if [ -z "$PACK_TAG" ]; then
    PACK_TAG="v${PACK_VERSION}"
fi

name="naslos-install-pack-${PACK_VERSION}.tar.gz"
base="${PACK_BASE_URL:-https://github.com/${PACK_REPO}/releases/download/${PACK_TAG}}"
url="${base}/${name}"

for tool in curl tar; do
    command -v "$tool" >/dev/null 2>&1 || { echo "fetch-install-pack: missing '$tool'" >&2; exit 1; }
done

# sha256sum is GNU coreutils; macOS ships `shasum -a 256` instead.
if command -v sha256sum >/dev/null 2>&1; then
    sha256() { sha256sum "$1" | awk '{print $1}'; }
elif command -v shasum >/dev/null 2>&1; then
    sha256() { shasum -a 256 "$1" | awk '{print $1}'; }
else
    echo "fetch-install-pack: neither sha256sum nor shasum is available" >&2
    exit 1
fi

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

actual=$(sha256 "$tmp/$name")
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
echo "fetch-install-pack: extracted pack ${PACK_VERSION} (${PACK_TAG}) into $dest"
