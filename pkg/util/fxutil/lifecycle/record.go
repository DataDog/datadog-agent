// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package lifecycle

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

const recordVersion = 1

// maxRecordSize bounds the size of the record read from disk.
const maxRecordSize = 64 * 1024

// State is the state of a process as persisted in its lifecycle record.
type State string

// Process states.
const (
	// StateRunning means the process started and has not begun its shutdown.
	StateRunning State = "running"
	// StateStopping means the process began its shutdown.
	StateStopping State = "stopping"
	// StateStopped means the process completed its shutdown.
	StateStopped State = "stopped"
)

// Outcome is how the previous run of a process ended, derived from the state of its record.
type Outcome string

// Outcomes of the previous run.
const (
	// OutcomeClean means the previous process completed its shutdown.
	OutcomeClean Outcome = "clean"
	// OutcomeKilledDuringShutdown means the previous process began its shutdown but did not complete
	// it: it was killed (for instance by a SIGKILL after a grace period) or crashed while stopping.
	OutcomeKilledDuringShutdown Outcome = "killed_during_shutdown"
	// OutcomeDiedWhileRunning means the previous process was ready but never began its shutdown: it
	// crashed, was OOM-killed or was killed without a prior SIGTERM.
	OutcomeDiedWhileRunning Outcome = "died_while_running"
	// OutcomeDiedDuringStartup means the previous process died before it was ready, without beginning
	// its shutdown.
	OutcomeDiedDuringStartup Outcome = "died_during_startup"
)

// Record is the lifecycle record persisted to disk.
type Record struct {
	Version          int                `json:"version"`
	State            State              `json:"state"`
	Trigger          string             `json:"trigger,omitempty"`
	ProcessStartAt   time.Time          `json:"process_start_at"`
	ReadyAt          time.Time          `json:"ready_at,omitzero"`
	StopStartAt      time.Time          `json:"stop_start_at,omitzero"`
	UpdatedAt        time.Time          `json:"updated_at"`
	CurrentPhase     string             `json:"current_phase,omitempty"`
	CurrentComponent string             `json:"current_component,omitempty"`
	Phases           map[string]float64 `json:"phases,omitempty"`
	SlowComponents   []Timing           `json:"slow_components,omitempty"`
}

// Outcome returns how the run described by the record ended.
func (r Record) Outcome() Outcome {
	switch r.State {
	case StateStopped:
		return OutcomeClean
	case StateStopping:
		return OutcomeKilledDuringShutdown
	}
	if r.ReadyAt.IsZero() {
		return OutcomeDiedDuringStartup
	}
	return OutcomeDiedWhileRunning
}

// ShutdownSeconds returns the duration of the shutdown. When the shutdown did not complete, it is
// the time between the beginning of the shutdown and the last update of the record, a lower bound
// of the time the process spent stopping before it was killed.
func (r Record) ShutdownSeconds() (float64, bool) {
	if total, ok := r.Phases[PhaseTotal]; ok {
		return total, true
	}
	if r.StopStartAt.IsZero() || r.UpdatedAt.Before(r.StopStartAt) {
		return 0, false
	}
	return r.UpdatedAt.Sub(r.StopStartAt).Seconds(), true
}

// LoadPrevious reads and removes the lifecycle record left at path by the previous run of the
// process. It returns false when there is no record.
func LoadPrevious(path string) (Record, bool, error) {
	f, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return Record{}, false, nil
	}
	if err != nil {
		return Record{}, false, err
	}
	defer f.Close()

	// Remove the record so that it is reported once, even if it cannot be decoded.
	removeErr := os.Remove(path)

	var rec Record
	dec := json.NewDecoder(io.LimitReader(f, maxRecordSize))
	if err := dec.Decode(&rec); err != nil {
		return Record{}, false, errors.Join(fmt.Errorf("invalid lifecycle record %s: %w", path, err), removeErr)
	}
	if rec.Version != recordVersion {
		return Record{}, false, errors.Join(fmt.Errorf("unsupported lifecycle record version %d", rec.Version), removeErr)
	}
	return rec, true, removeErr
}

// writeRecord atomically writes the record at path.
func writeRecord(path string, rec Record) error {
	data, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	_, writeErr := tmp.Write(data)
	closeErr := tmp.Close()
	if err := errors.Join(writeErr, closeErr); err != nil {
		_ = os.Remove(tmp.Name())
		return err
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		_ = os.Remove(tmp.Name())
		return err
	}
	return nil
}
