package installpack

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

const (
	testTalos     = "v1.14.1"
	testSchematic = "4dd8e3a8b6203d3c14f049da8db4d3bb0d6d3e70c5e89dfcc1e709e81914f63c"
)

func testPack(t *testing.T, mutate func(*Metadata, map[string][]byte)) fstest.MapFS {
	t.Helper()
	members := map[string][]byte{
		"machine-config/naslos-installer.yaml.tmpl": []byte("apiVersion: v1alpha1\nkind: KubeNodeConfig\nsubnet: \"{{NODE_SUBNET}}\"\ndisk: \"{{INSTALL_DISK}}\"\n"),
		"schematic/naslos.yaml":                     []byte("customization: {}\n"),
		"charts/naslos/Chart.yaml":                  []byte("apiVersion: v2\nname: naslos\n"),
	}
	checksums := map[string]string{}
	for rel, data := range members {
		sum := sha256.Sum256(data)
		checksums[rel] = hex.EncodeToString(sum[:])
	}
	meta := Metadata{
		Name:            PackName,
		FormatVersion:   FormatVersion,
		NaslosVersion:   "0.1.0",
		ChartVersion:    "0.1.0",
		TalosVersion:    testTalos,
		SchematicID:     testSchematic,
		ISOUrls:         map[string]string{"metal-amd64": "https://example/iso"},
		Checksums:       checksums,
		ChartAppVersion: "0.1.0",
	}
	if mutate != nil {
		mutate(&meta, members)
	}
	raw, err := json.Marshal(meta)
	if err != nil {
		t.Fatalf("marshal metadata: %v", err)
	}
	fsys := fstest.MapFS{"metadata.json": &fstest.MapFile{Data: raw}}
	for rel, data := range members {
		fsys[rel] = &fstest.MapFile{Data: data}
	}
	return fsys
}

func TestLoadRejectsMissingMetadata(t *testing.T) {
	_, err := Load(fstest.MapFS{}, "", "")
	if err == nil || !strings.Contains(err.Error(), "metadata.json") {
		t.Fatalf("want missing metadata error, got %v", err)
	}
}

func TestLoadAcceptsValidPack(t *testing.T) {
	p, err := Load(testPack(t, nil), testTalos, testSchematic)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := p.Metadata().ISOURL(); got != "https://example/iso" {
		t.Fatalf("ISOURL = %q", got)
	}
}

func TestLoadRefusesVersionSkew(t *testing.T) {
	fsys := testPack(t, nil)
	if _, err := Load(fsys, "v9.9.9", testSchematic); err == nil {
		t.Fatal("want talosVersion mismatch error")
	}
	if _, err := Load(fsys, testTalos, "deadbeef"); err == nil {
		t.Fatal("want schematicId mismatch error")
	}
}

func TestLoadRefusesUnknownFormat(t *testing.T) {
	fsys := testPack(t, func(m *Metadata, _ map[string][]byte) { m.FormatVersion = 99 })
	if _, err := Load(fsys, "", ""); err == nil || !strings.Contains(err.Error(), "formatVersion") {
		t.Fatalf("want formatVersion error, got %v", err)
	}
}

func TestVerifyDetectsTampering(t *testing.T) {
	fsys := testPack(t, nil)
	p, err := Load(fsys, "", "")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if err := p.Verify(); err != nil {
		t.Fatalf("Verify clean pack: %v", err)
	}
	fsys["schematic/naslos.yaml"].Data = []byte("customization: {tampered: true}\n")
	if err := p.Verify(); err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("want checksum mismatch, got %v", err)
	}
}

func TestRenderMachineConfig(t *testing.T) {
	p, err := Load(testPack(t, nil), "", "")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	out, err := p.RenderMachineConfig(map[string]string{
		"NODE_SUBNET":  "192.168.1.0/24",
		"INSTALL_DISK": "/dev/vda",
	})
	if err != nil {
		t.Fatalf("RenderMachineConfig: %v", err)
	}
	if !strings.Contains(string(out), "192.168.1.0/24") || strings.Contains(string(out), "{{") {
		t.Fatalf("unexpected render: %s", out)
	}
	if _, err := p.RenderMachineConfig(map[string]string{"NODE_SUBNET": "x"}); err == nil {
		t.Fatal("want unresolved placeholder error")
	}
}

func TestExtract(t *testing.T) {
	p, err := Load(testPack(t, nil), "", "")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	dst := t.TempDir()
	if err := p.Extract(dst); err != nil {
		t.Fatalf("Extract: %v", err)
	}
	for _, rel := range []string{"machine-config/naslos-installer.yaml.tmpl", "schematic/naslos.yaml", "charts/naslos/Chart.yaml"} {
		if _, err := os.Stat(filepath.Join(dst, rel)); err != nil {
			t.Fatalf("missing extracted %s: %v", rel, err)
		}
	}
}
