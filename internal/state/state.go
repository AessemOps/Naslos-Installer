// Package state persists the installer's progress so an interrupted install can
// resume without re-keying an already-installed node (FR-INSTALL-09).
//
// The state file lives at <app-data>/naslos-install/state.json (0600) and
// records the inputs, the pack version and a per-step status.
package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/AessemOps/Naslos-Installer/internal/config"
)

// Version is the state schema version.
const Version = 1

// Status values for a step.
const (
	Pending = "pending"
	Running = "running"
	Done    = "done"
	Failed  = "failed"
)

// Step is the recorded state of one install milestone.
type Step struct {
	Status    string    `json:"status"`
	Msg       string    `json:"msg,omitempty"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// State is the whole persisted install state.
type State struct {
	Version     int             `json:"version"`
	Input       config.Input    `json:"input"`
	PackVersion string          `json:"packVersion,omitempty"`
	Steps       map[string]Step `json:"steps,omitempty"`
	CreatedAt   time.Time       `json:"createdAt"`
}

// New returns a fresh state for the given inputs.
func New(in config.Input, packVersion string) *State {
	return &State{
		Version:     Version,
		Input:       in,
		PackVersion: packVersion,
		Steps:       map[string]Step{},
		CreatedAt:   time.Now().UTC(),
	}
}

// DefaultPath returns <user-config-dir>/naslos-install/state.json.
func DefaultPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "naslos-install", "state.json"), nil
}

// Load reads a state file. A missing file returns (nil, nil) so the caller can
// start fresh without treating it as an error.
func Load(path string) (*State, error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var s State
	if err := json.Unmarshal(raw, &s); err != nil {
		// A truncated write (an older version wrote in place) must not make
		// the only resume record unreadable.
		if backup, berr := os.ReadFile(path + ".bak"); berr == nil {
			if json.Unmarshal(backup, &s) != nil {
				return nil, err
			}
		} else {
			return nil, err
		}
	}
	if s.Version != Version {
		return nil, errors.New("unsupported state version")
	}
	if s.Steps == nil {
		s.Steps = map[string]Step{}
	}
	// Re-assert permissions: the file holds the domain and admin identity.
	_ = os.Chmod(path, 0o600)

	// Scrub a password persisted by an older build.
	var probe map[string]json.RawMessage
	if json.Unmarshal(raw, &probe) == nil {
		if inputRaw, ok := probe["input"]; ok {
			var fields map[string]json.RawMessage
			if json.Unmarshal(inputRaw, &fields) == nil {
				_, legacy := fields["AdminPassword"]
				_, legacyLower := fields["adminPassword"]
				if legacy || legacyLower {
					if err := s.Save(path); err != nil {
						return nil, fmt.Errorf("scrubbing the persisted password: %w", err)
					}
				}
			}
		}
	}
	return &s, nil
}

// Save writes the state atomically (temp file, fsync, rename) with 0600,
// keeping the previous version as state.json.bak for a torn-write fallback.
func (s *State) Save(path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".state-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(raw); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	// Best-effort backup of the previous state.
	if previous, err := os.ReadFile(path); err == nil {
		_ = os.WriteFile(path+".bak", previous, 0o600)
	}
	return os.Rename(tmpName, path)
}

// Archive moves the engine's state files into a timestamped subdirectory so a
// fresh install can start without deleting the old PKI/kubeconfig. It returns
// the backup directory, or "" when there was nothing to move.
func Archive(dir string) (string, error) {
	names := []string{
		"state.json",
		"state.json.bak",
		"talosconfig",
		"controlplane.yaml",
		"kubeconfig",
		"machine-config.yaml",
		"talos-secrets.json",
	}
	present := false
	for _, name := range names {
		if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
			present = true
			break
		}
	}
	if !present {
		return "", nil
	}
	backup := filepath.Join(dir, "stale-"+time.Now().UTC().Format("20060102T150405Z"))
	if err := os.MkdirAll(backup, 0o700); err != nil {
		return "", err
	}
	for _, name := range names {
		src := filepath.Join(dir, name)
		if _, err := os.Stat(src); err != nil {
			continue
		}
		if err := os.Rename(src, filepath.Join(backup, name)); err != nil {
			return "", err
		}
	}
	return backup, nil
}

// Lock takes an exclusive lock for the state directory so two engines cannot
// drive one install. The returned func releases it. A lock older than 24 h is
// treated as stale (the engine that created it is gone).
func Lock(dir string) (func(), error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, ".lock")
	for attempt := 0; attempt < 2; attempt++ {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err == nil {
			_, _ = fmt.Fprintf(f, "%d\n", os.Getpid())
			return func() {
				_ = f.Close()
				_ = os.Remove(path)
			}, nil
		}
		info, statErr := os.Stat(path)
		if statErr != nil || time.Since(info.ModTime()) < 24*time.Hour {
			break
		}
		_ = os.Remove(path)
	}
	return nil, errors.New("another installer is already using this state directory")
}

// SetStep records a milestone's status.
func (s *State) SetStep(name, status, msg string) {
	if s.Steps == nil {
		s.Steps = map[string]Step{}
	}
	s.Steps[name] = Step{Status: status, Msg: msg, UpdatedAt: time.Now().UTC()}
}

// StepDone reports whether a milestone already completed (for resume).
func (s *State) StepDone(name string) bool {
	st, ok := s.Steps[name]
	return ok && st.Status == Done
}
