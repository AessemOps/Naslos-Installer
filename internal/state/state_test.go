package state

import (
	"os"
	"path/filepath"
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
