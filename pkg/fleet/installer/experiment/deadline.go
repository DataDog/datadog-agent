// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package experiment supervises a running update experiment: it bounds how long the experiment
// jobs may run and reverts when that bound is passed or when an experiment job exits.
//
// The supervisor is the only thing standing between a bad version and a host that stops
// reporting, so its state cannot live only in memory: the installer daemon is itself part of the
// payload being replaced and is restarted, and the host may reboot mid-experiment. The deadline is
// therefore persisted, and its presence on disk -- not a field on a struct -- is what says an
// experiment is running.
package experiment

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/paths"
)

const deadlineFileName = "experiment-deadline"

const (
	// DefaultDuration is how long an experiment may run when the task naming it does not say.
	//
	// One hour: long enough for a slow-manifesting regression -- a leak, a wedged check, a
	// crash loop with a long period -- to show up in the backend's evaluation, and short enough
	// that an experiment nobody is watching, on a host whose operator has gone home, is back on
	// stable within the hour. The backend normally promotes or stops well before this; the
	// deadline is what covers the case where it never says anything at all.
	DefaultDuration = time.Hour
	// MinDuration is the floor a requested duration is clamped to. Below a minute an experiment
	// cannot be distinguished from its own startup.
	MinDuration = time.Minute
	// MaxDuration is the ceiling a requested duration is clamped to. The clamp is applied on the
	// host rather than trusted from the task, so a malformed or hostile duration cannot leave an
	// experiment running unbounded -- which is the one outcome the deadline exists to prevent.
	MaxDuration = 24 * time.Hour
)

// ClampDuration returns the duration an experiment will actually be allowed to run.
//
// A zero or negative duration means the task did not name one, and gets the default.
func ClampDuration(requested time.Duration) time.Duration {
	if requested <= 0 {
		return DefaultDuration
	}
	if requested < MinDuration {
		return MinDuration
	}
	if requested > MaxDuration {
		return MaxDuration
	}
	return requested
}

// state is what is persisted. It is versioned so a future field can be added without a release
// where the daemon cannot read its own marker.
type state struct {
	// Version is the experiment's version, carried for logging and for the daemon to report
	// what it reverted from.
	Version string `json:"version"`
	// ExpiresAt is when the experiment must be reverted if nothing has promoted or stopped it.
	ExpiresAt time.Time `json:"expires_at"`
}

// Deadline is the persisted bound on a running experiment.
//
// The zero value is not usable; construct with NewDeadline.
type Deadline struct {
	path string
}

// NewDeadline returns the Deadline stored under the Agent's run directory.
func NewDeadline() *Deadline {
	return &Deadline{path: filepath.Join(paths.RunPath, deadlineFileName)}
}

// NewDeadlineAt returns a Deadline stored at an explicit path. Tests use it; production does not.
func NewDeadlineAt(path string) *Deadline {
	return &Deadline{path: path}
}

// Path returns where the deadline is stored.
func (d *Deadline) Path() string { return d.path }

// Set records that an experiment of the given version is running and must be reverted at
// expiresAt.
//
// The write is to a temporary file in the same directory and a rename into place, so a daemon that
// dies mid-write leaves either the old deadline or the new one, never a truncated file that would
// read as "no experiment is running" on a host where one is.
func (d *Deadline) Set(version string, expiresAt time.Time) error {
	if version == "" {
		return errors.New("an experiment deadline needs the version it applies to")
	}
	if err := os.MkdirAll(filepath.Dir(d.path), 0755); err != nil {
		return fmt.Errorf("could not create the run directory: %w", err)
	}
	content, err := json.Marshal(state{Version: version, ExpiresAt: expiresAt.UTC()})
	if err != nil {
		return fmt.Errorf("could not serialize the experiment deadline: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(d.path), "."+deadlineFileName+".tmp")
	if err != nil {
		return fmt.Errorf("could not create a temporary deadline file: %w", err)
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(content); err != nil {
		tmp.Close()
		return fmt.Errorf("could not write the experiment deadline: %w", err)
	}
	if err := tmp.Chmod(0644); err != nil {
		tmp.Close()
		return fmt.Errorf("could not set the deadline file mode: %w", err)
	}
	// The daemon may be killed at any point, and a deadline that is not on the platter is a
	// deadline that does not survive the reboot it exists to survive.
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("could not flush the experiment deadline: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("could not close the experiment deadline: %w", err)
	}
	if err := os.Rename(tmp.Name(), d.path); err != nil {
		return fmt.Errorf("could not move the experiment deadline into place: %w", err)
	}
	return nil
}

// Get returns the recorded deadline. The second return value is false when no experiment is
// recorded as running.
//
// An unreadable or unparseable file is an error, not an absent deadline: reporting "no experiment
// is running" for a file that exists but cannot be read would strand a running experiment with
// nothing left to revert it.
func (d *Deadline) Get() (version string, expiresAt time.Time, set bool, err error) {
	content, err := os.ReadFile(d.path)
	if errors.Is(err, os.ErrNotExist) {
		return "", time.Time{}, false, nil
	}
	if err != nil {
		return "", time.Time{}, false, fmt.Errorf("could not read the experiment deadline: %w", err)
	}
	var s state
	if err := json.Unmarshal(content, &s); err != nil {
		return "", time.Time{}, false, fmt.Errorf("could not parse the experiment deadline at %s: %w", d.path, err)
	}
	if s.Version == "" || s.ExpiresAt.IsZero() {
		return "", time.Time{}, false, fmt.Errorf("the experiment deadline at %s is incomplete", d.path)
	}
	return s.Version, s.ExpiresAt, true, nil
}

// Clear removes the deadline, which is what records that no experiment is running. It succeeds
// when there is nothing to remove.
func (d *Deadline) Clear() error {
	if err := os.Remove(d.path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("could not clear the experiment deadline: %w", err)
	}
	return nil
}
