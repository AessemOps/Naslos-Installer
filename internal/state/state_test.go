package state

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AessemOps/Naslos-Installer/internal/config"
)

func TestLoadMissingReturnsNil(t *testing.T) {
	s, err := Load(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if s != nil {
		t.Fatalf("want nil state for a missing file, got %+v", s)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	in := config.Input{NodeIP: "192.168.1.117", Domain: "naslos.local", AdminUser: "admin"}
	s := New(in, "0.1.0")
	s.SetStep("talos", Done, "bootstrapped")

	path := filepath.Join(t.TempDir(), "sub", "state.json")
	if err := s.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("state file mode = %o, want 600", perm)
	}

	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.PackVersion != "0.1.0" || got.Input.NodeIP != "192.168.1.117" {
		t.Fatalf("unexpected state: %+v", got)
	}
	if !got.StepDone("talos") {
		t.Fatal("talos step should be done")
	}
	if got.StepDone("helm") {
		t.Fatal("helm step should not be done")
	}
}

func TestLoadRejectsWrongVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	if err := os.WriteFile(path, []byte(`{"version":99}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("want version error")
	}
}

func TestPasswordIsNeverPersisted(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	s := New(config.Input{NodeIP: "10.0.0.1", AdminPassword: "Correct1"}, "0.1.0")
	if err := s.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "Correct1") || strings.Contains(strings.ToLower(string(raw)), "adminpassword") {
		t.Fatalf("the administrator password was persisted:\n%s", raw)
	}
}

func TestLoadScrubsALegacyPassword(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	legacy := `{"version":1,"input":{"NodeIP":"10.0.0.1","AdminPassword":"Correct1"},"createdAt":"2026-01-01T00:00:00Z"}`
	if err := os.WriteFile(path, []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err != nil {
		t.Fatalf("Load: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "Correct1") {
		t.Fatalf("a legacy password was not scrubbed:\n%s", raw)
	}
}

func TestLockIsExclusive(t *testing.T) {
	dir := t.TempDir()
	unlock, err := Lock(dir)
	if err != nil {
		t.Fatalf("first Lock: %v", err)
	}
	if _, err := Lock(dir); err == nil {
		t.Fatal("a second engine acquired the same state directory")
	}
	unlock()
	unlock2, err := Lock(dir)
	if err != nil {
		t.Fatalf("Lock after release: %v", err)
	}
	unlock2()
}
