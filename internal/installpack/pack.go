// Package installpack loads and verifies a Naslos install pack.
//
// A pack is the versioned artifact published by the Naslos-Linux release CI
// (see docs/installer-contract.md). This package parses metadata.json, verifies
// every member's sha256, renders the parameterised machine-config template, and
// can extract the whole pack to a directory for the Helm loader.
package installpack

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"regexp"
	"strings"
)

// FormatVersion is the metadata schema this loader understands.
const FormatVersion = 1

// PackName is the expected metadata name.
const PackName = "naslos-install-pack"

// Metadata mirrors the pack's metadata.json.
type Metadata struct {
	Name            string            `json:"name"`
	FormatVersion   int               `json:"formatVersion"`
	NaslosVersion   string            `json:"naslosVersion"`
	ChartVersion    string            `json:"chartVersion"`
	ChartAppVersion string            `json:"chartAppVersion"`
	TalosVersion    string            `json:"talosVersion"`
	SchematicID     string            `json:"schematicId"`
	ISOUrls         map[string]string `json:"isoUrls"`
	GeneratedAt     string            `json:"generatedAt"`
	Checksums       map[string]string `json:"checksums"`
}

// ISOURL returns the metal-amd64 ISO URL (the only one published today).
func (m Metadata) ISOURL() string { return m.ISOUrls["metal-amd64"] }

// Pack is a parsed, not-yet-verified install pack.
type Pack struct {
	meta Metadata
	fsys fs.FS
}

// Load parses metadata.json and checks the pack's identity. It does NOT verify
// member checksums — call Verify for that. expectedTalos/expectedSchematic may
// be empty to skip the version gate (tests), otherwise a mismatch is refused.
func Load(fsys fs.FS, expectedTalos, expectedSchematic string) (*Pack, error) {
	raw, err := fs.ReadFile(fsys, "metadata.json")
	if err != nil {
		return nil, fmt.Errorf("install pack has no metadata.json (run `make fetch-pack`): %w", err)
	}
	var meta Metadata
	if err := json.Unmarshal(raw, &meta); err != nil {
		return nil, fmt.Errorf("parsing metadata.json: %w", err)
	}
	if meta.Name != PackName {
		return nil, fmt.Errorf("unexpected pack name %q, want %q", meta.Name, PackName)
	}
	if meta.FormatVersion != FormatVersion {
		return nil, fmt.Errorf("unsupported pack formatVersion %d, want %d", meta.FormatVersion, FormatVersion)
	}
	if meta.TalosVersion == "" || meta.SchematicID == "" {
		return nil, errors.New("metadata.json must pin talosVersion and schematicId")
	}
	if expectedTalos != "" && meta.TalosVersion != expectedTalos {
		return nil, fmt.Errorf("pack talosVersion %q != expected %q", meta.TalosVersion, expectedTalos)
	}
	if expectedSchematic != "" && meta.SchematicID != expectedSchematic {
		return nil, fmt.Errorf("pack schematicId %q != expected %q", meta.SchematicID, expectedSchematic)
	}
	return &Pack{meta: meta, fsys: fsys}, nil
}

// Metadata returns the pack metadata.
func (p *Pack) Metadata() Metadata { return p.meta }

// Verify recomputes the sha256 of every member listed in metadata.json.
func (p *Pack) Verify() error {
	if len(p.meta.Checksums) == 0 {
		return errors.New("metadata.json lists no checksums")
	}
	for rel, want := range p.meta.Checksums {
		rel = path.Clean(rel)
		if strings.HasPrefix(rel, "..") || path.IsAbs(rel) {
			return fmt.Errorf("checksum entry escapes the pack: %q", rel)
		}
		f, err := p.fsys.Open(rel)
		if err != nil {
			return fmt.Errorf("checksum lists a missing member %q: %w", rel, err)
		}
		h := sha256.New()
		_, copyErr := io.Copy(h, f)
		closeErr := f.Close()
		if copyErr != nil {
			return fmt.Errorf("reading %q: %w", rel, copyErr)
		}
		if closeErr != nil {
			return fmt.Errorf("closing %q: %w", rel, closeErr)
		}
		if got := hex.EncodeToString(h.Sum(nil)); got != want {
			return fmt.Errorf("checksum mismatch for %q: got %s, want %s", rel, got, want)
		}
	}
	return nil
}

// machineConfigPath is the template member.
const machineConfigPath = "machine-config/naslos-installer.yaml.tmpl"

var placeholderRE = regexp.MustCompile(`\{\{[A-Z0-9_]+\}\}`)

// RenderMachineConfig substitutes {{KEY}} placeholders (e.g. NODE_SUBNET,
// INSTALL_DISK) in the pack's machine-config template. It fails if any
// placeholder is left unresolved, so a typo can never reach Talos.
func (p *Pack) RenderMachineConfig(vars map[string]string) ([]byte, error) {
	raw, err := fs.ReadFile(p.fsys, machineConfigPath)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", machineConfigPath, err)
	}
	out := string(raw)
	for k, v := range vars {
		out = strings.ReplaceAll(out, "{{"+k+"}}", v)
	}
	if left := placeholderRE.FindAllString(out, -1); len(left) > 0 {
		return nil, fmt.Errorf("unresolved template placeholders: %s", strings.Join(left, ", "))
	}
	return []byte(out), nil
}

// ReadFile returns the raw bytes of one pack member (relative path). It is
// guarded against escaping the pack so a malformed metadata cannot read the
// host filesystem.
func (p *Pack) ReadFile(rel string) ([]byte, error) {
	rel = path.Clean(rel)
	if strings.HasPrefix(rel, "..") || path.IsAbs(rel) {
		return nil, fmt.Errorf("pack member escapes the pack: %q", rel)
	}
	return fs.ReadFile(p.fsys, rel)
}

// Extract writes the whole pack under dst, preserving its layout.
func (p *Pack) Extract(dst string) error {
	return fs.WalkDir(p.fsys, ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if name == "." || name == "metadata.json" || name == "README.md" {
			return nil
		}
		target := path.Join(dst, name)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := fs.ReadFile(p.fsys, name)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(path.Dir(target), 0o755); err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
}
