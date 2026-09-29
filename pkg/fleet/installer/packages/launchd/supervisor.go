// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

package launchd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Deadline is a wall-clock bound on an unsupervised experiment, persisted to disk so it survives
// a restart of whichever process enforces it. It is the only coverage for an experiment that is
// alive but hung: the ExitObserver sees exit, nothing else.
type Deadline struct {
	// Path is the file the deadline is persisted to.
	Path string
}

// Write persists a deadline of now+window. The write goes to a temporary file in the same
// directory and is renamed into place, matching the convention Job.Write uses for job
// definitions, so a reader never observes a partially written deadline.
func (d Deadline) Write(window time.Duration) error {
	deadline := time.Now().Add(window).Format(time.RFC3339)
	path := d.Path
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp")
	if err != nil {
		return fmt.Errorf("could not create temporary deadline file: %w", err)
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.WriteString(deadline); err != nil {
		tmp.Close()
		return fmt.Errorf("could not write deadline file: %w", err)
	}
	if err := tmp.Chmod(0644); err != nil {
		tmp.Close()
		return fmt.Errorf("could not set deadline file mode: %w", err)
	}
	// Owned by root:wheel so a compromised experiment, running as _dd-agent, cannot extend its
	// own window. Best-effort: this also runs unprivileged in tests.
	if err := tmp.Chown(0, 0); err != nil {
		log.Warnf("could not set root:wheel ownership on deadline file %s: %v", path, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("could not close deadline file: %w", err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("could not move deadline file into place: %w", err)
	}
	return nil
}

// Clear removes the deadline file. It succeeds when the file is already absent, matching the
// idempotency convention the rest of this package follows for teardown.
func (d Deadline) Clear() error {
	if err := os.Remove(d.Path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("could not remove deadline file: %w", err)
	}
	return nil
}

// Present reports whether the deadline file still exists.
//
// A caller about to act on an exit event or a tick calls this immediately before acting: a
// deliberate stop clears the file first, and racing that clear is how the watcher tells a
// commanded stop apart from a real failure. There is no other signal between the two processes.
func (d Deadline) Present() (bool, error) {
	if _, err := os.Stat(d.Path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("could not stat deadline file: %w", err)
	}
	return true, nil
}

// Expired reports whether the deadline has passed, is unparseable, or is implausibly far in the
// future by more than maxSkew -- the host's clock having moved backwards since the deadline was
// written. All three are treated as expired: a hung experiment must not survive a clock that has
// become unreliable. A missing file reports not expired, since there is nothing to enforce.
func (d Deadline) Expired(maxSkew time.Duration) (bool, error) {
	raw, err := os.ReadFile(d.Path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("could not read deadline file: %w", err)
	}
	deadline, err := time.Parse(time.RFC3339, strings.TrimSpace(string(raw)))
	if err != nil {
		return true, nil
	}
	now := time.Now()
	if now.After(deadline) {
		return true, nil
	}
	if deadline.Sub(now) > maxSkew {
		return true, nil
	}
	return false, nil
}

// ExitEvent is delivered when an observed process exits.
type ExitEvent struct {
	// Pid is the process that exited.
	Pid int
	// Status is the wait(2) status word delivered with the exit, or 0 for a pid that had
	// already exited by the time it was armed (see ArmExitObserver).
	Status int
}

// ExitObserver watches a fixed set of pids for exit via kqueue/EVFILT_PROC. It is one-shot per
// pid and is never re-armed: nothing relaunches an unsupervised job, so a second exit of the
// same pid cannot occur, and re-arming would only be useful for a job this package's whole
// design makes terminal by construction.
type ExitObserver struct {
	kq int
}

// ArmExitObserver opens a kqueue and arms an EVFILT_PROC/NOTE_EXIT|NOTE_EXITSTATUS watch on every
// given pid. The returned channel receives one ExitEvent per pid, including pids that had
// already exited before they could be armed: ESRCH on arming is that pid's exit notification,
// not an error to surface.
//
// Residual, unclosable risk: pid recycling between the caller reading the pid (e.g. via
// JobSet.Pids) and this function arming the filter. There is no way to close this window from
// user space; it is accepted, not fixed.
func ArmExitObserver(pids []int) (*ExitObserver, <-chan ExitEvent, error) {
	kq, err := unix.Kqueue()
	if err != nil {
		return nil, nil, fmt.Errorf("could not open kqueue: %w", err)
	}

	events := make(chan ExitEvent, len(pids)+1)
	var immediate []int
	for _, pid := range pids {
		var kev unix.Kevent_t
		unix.SetKevent(&kev, pid, unix.EVFILT_PROC, unix.EV_ADD|unix.EV_ENABLE|unix.EV_ONESHOT)
		kev.Fflags = unix.NOTE_EXIT | unix.NOTE_EXITSTATUS
		if _, err := unix.Kevent(kq, []unix.Kevent_t{kev}, nil, nil); err != nil {
			if errors.Is(err, unix.ESRCH) {
				immediate = append(immediate, pid)
				continue
			}
			unix.Close(kq)
			return nil, nil, fmt.Errorf("could not arm exit observer for pid %d: %w", pid, err)
		}
	}
	for _, pid := range immediate {
		events <- ExitEvent{Pid: pid}
	}

	o := &ExitObserver{kq: kq}
	go o.watch(events, len(pids)-len(immediate))
	return o, events, nil
}

// watch blocks in kevent(2) until every armed, non-immediate pid has delivered its one-shot
// event, then closes the channel. remaining is 0 when every pid was already dead at arm time.
func (o *ExitObserver) watch(out chan ExitEvent, remaining int) {
	defer close(out)
	if remaining <= 0 {
		return
	}
	buf := make([]unix.Kevent_t, remaining)
	for remaining > 0 {
		n, err := unix.Kevent(o.kq, nil, buf, nil)
		if err != nil {
			if errors.Is(err, unix.EINTR) {
				continue
			}
			// EBADF: the kqueue was closed from under us via Disarm. Not an error worth
			// surfacing -- the caller tore this down on purpose.
			if !errors.Is(err, unix.EBADF) {
				log.Errorf("exit observer: kevent wait failed: %v", err)
			}
			return
		}
		for i := 0; i < n; i++ {
			out <- ExitEvent{Pid: int(buf[i].Ident), Status: int(buf[i].Data)}
		}
		remaining -= n
	}
}

// Disarm closes the kqueue, ending the watch. Idempotent.
func (o *ExitObserver) Disarm() error {
	if o == nil || o.kq < 0 {
		return nil
	}
	err := unix.Close(o.kq)
	o.kq = -1
	if err != nil {
		return fmt.Errorf("could not close kqueue: %w", err)
	}
	return nil
}
