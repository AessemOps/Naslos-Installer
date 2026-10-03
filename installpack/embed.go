// Package installpack embeds the versioned Naslos install pack.
//
// The pack data is gitignored and populated by `make fetch-pack`
// (scripts/fetch-install-pack.sh). Only this declaration and README.md are
// committed, so the embed pattern matches at least one file on a fresh
// checkout. The loader that reads and verifies the pack is in
// internal/installpack.
package installpack

import "embed"

// FS is the embedded pack root (metadata.json, charts/, machine-config/, ...).
//
// `all:` is required: Go's embed otherwise skips files and directories whose
// names begin with `_` or `.`, which silently drops Helm's
// charts/naslos/templates/_helpers.tpl and makes the pack fail its own
// checksum verification (found in the live drill).
//
//go:embed all:*
var FS embed.FS
