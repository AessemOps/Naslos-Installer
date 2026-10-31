#!/usr/bin/env bash
# Every in-tree version must agree. The release workflow stamps the tag version
# at build time, but a mismatched checkout makes local builds and `--version`
# lie, and the parity rule requires the installer to be the same X.Y.Z as Naslos.
set -euo pipefail
cd "$(dirname "$0")/.."
expected="${1:-}"
fail=0
read_version() { python3 -c "import json,sys; print(json.load(open(sys.argv[1]))[sys.argv[2]])" "$@"; }
tauri=$(read_version desktop/src-tauri/tauri.conf.json version)
pkg=$(read_version desktop/package.json version)
cargo=$(sed -n 's/^version *= *"\(.*\)"/\1/p' desktop/src-tauri/Cargo.toml | head -1)
lock=$(python3 - <<'PY'
import re
text = open("desktop/src-tauri/Cargo.lock", encoding="utf-8").read()
m = re.search(r'name = "naslos-installer"\nversion = "([^"]+)"', text)
print(m.group(1) if m else "?")
PY
)
for pair in "tauri.conf.json=$tauri" "desktop/package.json=$pkg" "Cargo.toml=$cargo" "Cargo.lock=$lock"; do
  name="${pair%%=*}"; value="${pair#*=}"
  if [ "$value" != "${expected:-$tauri}" ]; then
    echo "check-version: $name is $value, expected ${expected:-$tauri}" >&2
    fail=1
  fi
done
if [ "$fail" -ne 0 ]; then
  exit 1
fi
echo "check-version: all in-tree versions are ${expected:-$tauri}"
