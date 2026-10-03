package resolver

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUpsertAddsMarkedLine(t *testing.T) {
	got := Upsert("127.0.0.1 localhost\n", "192.168.1.117", "naslos.local")
	if !strings.Contains(got, "192.168.1.117\tnaslos.local "+Marker) {
		t.Fatalf("marked line missing:\n%s", got)
	}
	if !strings.Contains(got, "127.0.0.1 localhost") {
		t.Fatalf("existing line lost:\n%s", got)
	}
}

func TestUpsertReplacesOnlyItsLine(t *testing.T) {
	content := "127.0.0.1 localhost\n10.0.0.1 other.local\n1.2.3.4 naslos.local " + Marker + "\n"
	got := Upsert(content, "192.168.1.117", "naslos.local")
	if strings.Count(got, Marker) != 1 {
		t.Fatalf("want exactly one marked line:\n%s", got)
	}
	if strings.Contains(got, "1.2.3.4") {
		t.Fatalf("old marked line not replaced:\n%s", got)
	}
	if !strings.Contains(got, "10.0.0.1 other.local") {
		t.Fatalf("unrelated line lost:\n%s", got)
	}
}

func TestRemove(t *testing.T) {
	content := "127.0.0.1 localhost\n1.2.3.4 naslos.local " + Marker + "\n"
	got := Remove(content)
	if strings.Contains(got, Marker) || !strings.Contains(got, "127.0.0.1 localhost") {
		t.Fatalf("unexpected Remove result:\n%s", got)
	}
}

func TestInstallRewritesFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hosts")
	if err := os.WriteFile(path, []byte("127.0.0.1 localhost\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Install(path, "192.168.1.117", "naslos.local"); err != nil {
		t.Fatalf("Install: %v", err)
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "192.168.1.117\tnaslos.local") {
		t.Fatalf("hosts not updated:\n%s", data)
	}
}
