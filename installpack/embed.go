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
//go:embed *
var FS embed.FS
