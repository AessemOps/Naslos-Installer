// Package state persists the installer's progress so an interrupted install can
// resume without re-keying an already-installed node (FR-INSTALL-09).
//
// The state file lives at <app-data>/naslos-install/state.json (0600) and
// records the inputs, the pack version and a per-step status.
package state

import (
	"encoding/json"
	"errors"
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
		return nil, err
	}
	if s.Version != Version {
		return nil, errors.New("unsupported state version")
	}
	if s.Steps == nil {
		s.Steps = map[string]Step{}
	}
	return &s, nil
}

// Save writes the state 0600, creating the parent directory 0700.
func (s *State) Save(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o600)
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
