package installpack

import (
	"io/fs"
	"testing"

	embedded "github.com/AessemOps/Naslos-Installer/installpack"
)

// TestEmbeddedPackVerifies guards the embed pattern in installpack/embed.go.
// It is skipped when this checkout has no fetched pack (the installpack/
// payload is gitignored), so run it after `make fetch-pack`/`fetch-latest-pack`
// — as the ci.yml pack-build job does.
//
// Regression: `//go:embed *` drops files whose names begin with `_`, so
// charts/naslos/templates/_helpers.tpl was missing from the embedded pack and
// every real install failed at "checksum lists a missing member".
func TestEmbeddedPackVerifies(t *testing.T) {
	if _, err := fs.Stat(embedded.FS, "metadata.json"); err != nil {
		t.Skip("no embedded install pack in this checkout")
	}
	p, err := Load(embedded.FS, "", "")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if err := p.Verify(); err != nil {
		t.Fatalf("Verify: %v", err)
	}
}
