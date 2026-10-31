package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testSchematic = "4dd8e3a8b6203d3c14f049da8db4d3bb0d6d3e70c5e89dfcc1e709e81914f63c"

func writePack(t *testing.T, talosVersion string) string {
	t.Helper()
	dir := t.TempDir()
	members := map[string]string{
		"machine-config/naslos-installer.yaml.tmpl": "# node subnet {{NODE_SUBNET}}\nmachine:\n  install:\n    disk: \"{{INSTALL_DISK}}\"\n---\napiVersion: v1alpha1\nkind: KubeFlannelCNIConfig\n$patch: delete\n",
		"schematic/naslos.yaml":                     "customization: {}\n",
	}
	checksums := map[string]string{}
	for rel, data := range members {
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256([]byte(data))
		checksums[rel] = hex.EncodeToString(sum[:])
	}
	meta := map[string]any{
		"name":          "naslos-install-pack",
		"formatVersion": 1,
		"naslosVersion": "0.1.0",
		"talosVersion":  talosVersion,
		"schematicId":   testSchematic,
		"isoUrls":       map[string]string{"metal-amd64": "https://example/iso"},
		"checksums":     checksums,
	}
	raw, err := json.Marshal(meta)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "metadata.json"), raw, 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func lastEvent(t *testing.T, out string) map[string]any {
	t.Helper()
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) == 0 {
		t.Fatal("no output")
	}
	var ev map[string]any
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &ev); err != nil {
		t.Fatalf("last line is not JSON: %v (%q)", err, lines[len(lines)-1])
	}
	return ev
}

func TestDryRunCompletes(t *testing.T) {
	t.Setenv("NASLOS_ADMIN_PASSWORD", "Correct1")
	dir := writePack(t, ExpectedTalosVersion)
	var buf bytes.Buffer
	err := run([]string{
		"--node-ip", "192.168.1.117", "--domain", "naslos.local",
		"--admin-user", "admin",
		"--dry-run", "--pack-dir", dir, "--state-dir", t.TempDir(),
	}, &buf)
	if err != nil {
		t.Fatalf("dry-run: %v\n%s", err, buf.String())
	}
	ev := lastEvent(t, buf.String())
	if ev["step"] != "done" || ev["status"] != "ok" {
		t.Fatalf("want done/ok, got %v", ev)
	}
}

func TestDryRunWritesMachineConfig(t *testing.T) {
	t.Setenv("NASLOS_ADMIN_PASSWORD", "Correct1")
	dir := writePack(t, ExpectedTalosVersion)
	stateDir := t.TempDir()
	var buf bytes.Buffer
	if err := run([]string{
		"--node-ip", "192.168.1.117", "--domain", "naslos.local",
		"--admin-user", "admin",
		"--dry-run", "--pack-dir", dir, "--state-dir", stateDir,
	}, &buf); err != nil {
		t.Fatalf("run: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(stateDir, "machine-config.yaml"))
	if err != nil {
		t.Fatalf("machine config not written: %v", err)
	}
	if !strings.Contains(string(raw), "192.168.1.0/24") || strings.Contains(string(raw), "{{") {
		t.Fatalf("unexpected machine config: %s", raw)
	}
}

func TestInvalidInputFailsBeforeNode(t *testing.T) {
	t.Setenv("NASLOS_ADMIN_PASSWORD", "Correct1")
	var buf bytes.Buffer
	err := run([]string{
		"--node-ip", "not-an-ip", "--domain", "naslos.local",
		"--admin-user", "admin",
		"--dry-run", "--state-dir", t.TempDir(),
	}, &buf)
	if err == nil {
		t.Fatal("want validation error")
	}
	ev := lastEvent(t, buf.String())
	if ev["error"] == nil {
		t.Fatalf("want an error event, got %v", ev)
	}
}

func TestPackVersionSkewRefused(t *testing.T) {
	t.Setenv("NASLOS_ADMIN_PASSWORD", "Correct1")
	dir := writePack(t, "v9.9.9")
	var buf bytes.Buffer
	err := run([]string{
		"--node-ip", "192.168.1.117", "--domain", "naslos.local",
		"--admin-user", "admin",
		"--dry-run", "--pack-dir", dir, "--state-dir", t.TempDir(),
	}, &buf)
	if err == nil || !strings.Contains(err.Error(), "talosVersion") {
		t.Fatalf("want version skew error, got %v", err)
	}
}

func TestParseReadsThePasswordFromEnv(t *testing.T) {
	t.Setenv("NASLOS_ADMIN_PASSWORD", "Correct1")
	opts, err := parse([]string{
		"--node-ip", "10.0.0.1",
		"--domain", "naslos.local",
		"--admin-user", "admin",
	})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if opts.input.AdminPassword != "Correct1" {
		t.Fatalf("password = %q", opts.input.AdminPassword)
	}
	if _, still := os.LookupEnv("NASLOS_ADMIN_PASSWORD"); still {
		t.Fatal("the password env var must be unset after it is read")
	}
}

func TestParseRejectsTheRemovedArgvPassword(t *testing.T) {
	_, err := parse([]string{
		"--node-ip", "10.0.0.1",
		"--domain", "naslos.local",
		"--admin-user", "admin",
		"--admin-password", "Correct1",
	})
	if err == nil || !strings.Contains(err.Error(), "removed") {
		t.Fatalf("want a removed-flag error, got %v", err)
	}
}

func TestParseReadsThePasswordFromStdin(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdin
	os.Stdin = r
	defer func() { os.Stdin = old }()
	go func() {
		_, _ = w.WriteString("Correct1\n")
		_ = w.Close()
	}()

	opts, err := parse([]string{
		"--node-ip", "10.0.0.1",
		"--domain", "naslos.local",
		"--admin-user", "admin",
		"--admin-password-stdin",
	})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if opts.input.AdminPassword != "Correct1" {
		t.Fatalf("password = %q", opts.input.AdminPassword)
	}
}
