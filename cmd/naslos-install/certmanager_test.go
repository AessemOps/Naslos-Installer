package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/AessemOps/Naslos-Installer/internal/installpack"
)

// addPackMember writes one more verified pack member and keeps metadata.json's
// checksum map in step, so installpack.Verify still passes.
func addPackMember(t *testing.T, dir, rel, content string) {
	t.Helper()
	p := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	metaPath := filepath.Join(dir, "metadata.json")
	raw, err := os.ReadFile(metaPath)
	if err != nil {
		t.Fatal(err)
	}
	var meta map[string]any
	if err := json.Unmarshal(raw, &meta); err != nil {
		t.Fatal(err)
	}
	sums, ok := meta["checksums"].(map[string]any)
	if !ok {
		t.Fatalf("metadata checksums have unexpected type %T", meta["checksums"])
	}
	sum := sha256.Sum256([]byte(content))
	sums[rel] = hex.EncodeToString(sum[:])
	meta["checksums"] = sums
	out, err := json.Marshal(meta)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(metaPath, out, 0o644); err != nil {
		t.Fatal(err)
	}
}

// A pack built before the certificate stack (0.2.5) must be usable: the step
// skips instead of failing the install over material the pack never had.
func TestPackHasCertManager(t *testing.T) {
	dir := writePack(t, "v1.14.1")
	p, err := installpack.Load(os.DirFS(dir), "", "")
	if err != nil {
		t.Fatal(err)
	}
	if packHasCertManager(p) {
		t.Fatal("pack without the certificate stack reported as having it")
	}

	addPackMember(t, dir, certManagerManifest, "apiVersion: apiextensions.k8s.io/v1\nkind: CustomResourceDefinition\n")
	p, err = installpack.Load(os.DirFS(dir), "", "")
	if err != nil {
		t.Fatal(err)
	}
	if !packHasCertManager(p) {
		t.Fatal("pack carrying cert-manager CRDs reported as missing them")
	}
}

func TestSingleChartRequiresExactlyOneMatch(t *testing.T) {
	dir := t.TempDir()
	if _, err := singleChart(dir, "cert-manager-*.tgz"); err == nil {
		t.Fatal("no match must be an error: a repacked chart must not be guessed")
	}
	want := filepath.Join(dir, "cert-manager-v1.18.2.tgz")
	if err := os.WriteFile(want, []byte("chart"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := singleChart(dir, "cert-manager-*.tgz")
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("got %s, want %s", got, want)
	}
	if err := os.WriteFile(filepath.Join(dir, "cert-manager-v1.19.0.tgz"), []byte("chart"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := singleChart(dir, "cert-manager-*.tgz"); err == nil {
		t.Fatal("two matches must be an error")
	}
}
