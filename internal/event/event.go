// Package event defines the engine's newline-delimited JSON progress protocol.
//
// The headless engine writes one JSON object per line to stdout; the Tauri
// shell (or a script) renders the progress bar and a log pane, and cancels the
// install by killing the child. The protocol is a stable contract
// (docs/installer-contract.md §5).
package event

import (
	"encoding/json"
	"fmt"
	"io"
	"sync"
)

// Status values for a progress event.
type Status string

const (
	Running Status = "running"
	OK      Status = "ok"
	Failed  Status = "failed"
)

// Error is the terminal failure payload.
type Error struct {
	Step   string `json:"step,omitempty"`
	Msg    string `json:"msg"`
	Output string `json:"output,omitempty"`
}

// Event is one line of the progress stream. Exactly one of Error or the
// step/status fields is set.
type Event struct {
	Step   string `json:"step,omitempty"`
	Status Status `json:"status,omitempty"`
	Pct    *int   `json:"pct,omitempty"`
	Msg    string `json:"msg,omitempty"`
	Error  *Error `json:"error,omitempty"`
}

// Emitter serialises events to a writer, one JSON object per line.
type Emitter struct {
	mu sync.Mutex
	w  io.Writer
}

// New returns an Emitter writing to w.
func New(w io.Writer) *Emitter { return &Emitter{w: w} }

func (e *Emitter) emit(ev Event) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	line, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	if _, err := e.w.Write(append(line, '\n')); err != nil {
		return err
	}
	return nil
}

// Progress emits a step update.
func (e *Emitter) Progress(step string, status Status, pct int, msg string) error {
	return e.emit(Event{Step: step, Status: status, Pct: &pct, Msg: msg})
}

// Step emits a running update at the given percentage.
func (e *Emitter) Step(step, msg string, pct int) error {
	return e.Progress(step, Running, pct, msg)
}

// Done emits the terminal success event.
func (e *Emitter) Done(msg string) error {
	return e.emit(Event{Step: "done", Status: OK, Pct: intPtr(100), Msg: msg})
}

// Fail emits the terminal error event and returns an error carrying the same
// message, so the caller can simply `return em.Fail(...)`.
func (e *Emitter) Fail(step, msg, output string) error {
	if err := e.emit(Event{Error: &Error{Step: step, Msg: msg, Output: output}}); err != nil {
		return err
	}
	if output != "" {
		return fmt.Errorf("%s: %s\n%s", step, msg, output)
	}
	return fmt.Errorf("%s: %s", step, msg)
}

func intPtr(v int) *int { return &v }
