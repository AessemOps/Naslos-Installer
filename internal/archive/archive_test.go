package archive

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBuildContainsCredentialsAndNoPassword(t *testing.T) {
	stateDir := t.TempDir()
	for _, name := range stateMembers {
		if err := os.WriteFile(filepath.Join(stateDir, name), []byte("x-"+name), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	outDir := t.TempDir()

	path, err := Build(Options{
		Dir: outDir, StateDir: stateDir,
		Domain: "naslos.local", AdminUser: "admin", LoginURL: "https://naslos.local/authelia",
		NaslosVersion: "0.1.0", TalosVersion: "v1.14.1", SchematicID: "abc123",
		ISOURL:     "https://factory.talos.dev/image/abc123/v1.14.1/metal-amd64.iso",
		ReadMember: func(name string) ([]byte, error) { return []byte("schematic"), nil },
		Now:        time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if filepath.Base(path) != "naslos-recovery-naslos.local-20261003T120000Z.zip" {
		t.Fatalf("unexpected name: %s", filepath.Base(path))
	}

	zr, err := zip.OpenReader(path)
	if err != nil {
		t.Fatalf("open zip: %v", err)
	}
	defer zr.Close()

	want := map[string]bool{
		"talosconfig": false, "controlplane.yaml": false, "talos-secrets.json": false,
		"kubeconfig": false, "schematic/naslos.yaml": false, "ISO.md": false, "README.txt": false,
	}
	var readme string
	for _, f := range zr.File {
		if _, ok := want[f.Name]; ok {
			want[f.Name] = true
		}
		if f.Name == "README.txt" {
			rc, _ := f.Open()
			buf := make([]byte, f.UncompressedSize64)
			_, _ = rc.Read(buf)
			rc.Close()
			readme = string(buf)
		}
	}
	for name, found := range want {
		if !found {
			t.Errorf("ZIP is missing %s", name)
		}
	}
	if strings.Contains(readme, "password is deliberately NOT") == false {
		t.Errorf("README should state the password is excluded:\n%s", readme)
	}
	if !strings.Contains(readme, "buddy-identity.json") {
		t.Errorf("README must warn about buddy-identity.json:\n%s", readme)
	}
	if !strings.Contains(readme, "https://naslos.local/authelia") {
		t.Errorf("README must show the login URL:\n%s", readme)
	}
}
