// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

package experiment

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/launchd"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// exitBuffer is how many exits are held before the supervisor consumes them.
//
// One per watched job is enough -- a job exits once per experiment, because nothing relaunches an
// -exp job -- and the buffer is oversized so that a Tick that arrives late still finds the event
// rather than a watcher blocked trying to report it.
const exitBuffer = 16

// KqueueWatcher watches the experiment jobs' processes with kqueue.
//
// It is named for its mechanism rather than for its role because the role's name, ProcWatcher, is
// the interface the supervisor is written against, and that interface has to compile on every
// platform while this does not.
//
// Polling launchctl would also work and would be simpler. kqueue is used because the interesting
// failure is an experiment that dies seconds after it starts, and the gap between a poll interval
// and an EVFILT_PROC notification is the length of time the host runs on no Agent at all: the
// stable jobs are already stopped by then, and the experiment's are gone.
type KqueueWatcher struct {
	mu sync.Mutex

	kq int
	// watched maps a PID to the label whose job it is, so an exit can be reported by name.
	watched map[int]string
	exits   chan ExitEvent

	launchctl *launchd.Client

	// done closes when Close is called, which is what stops the reader goroutine.
	done   chan struct{}
	closed bool
}

// NewKqueueWatcher returns a watcher with its kqueue open and its reader running.
func NewKqueueWatcher() (*KqueueWatcher, error) {
	kq, err := unix.Kqueue()
	if err != nil {
		return nil, fmt.Errorf("could not open a kqueue: %w", err)
	}
	// The kqueue descriptor must not survive into a child: the installer forks the Agent's
	// package scripts and helper binaries, and an inherited descriptor would keep the kernel
	// queue alive past this process.
	unix.CloseOnExec(kq)

	w := &KqueueWatcher{
		kq:        kq,
		watched:   map[int]string{},
		exits:     make(chan ExitEvent, exitBuffer),
		launchctl: launchd.NewClient(launchd.System),
		done:      make(chan struct{}),
	}
	go w.read()
	return w, nil
}

// Exits yields one event per observed exit.
func (w *KqueueWatcher) Exits() <-chan ExitEvent { return w.exits }

// Watch registers the current processes of the given labels, replacing whatever was registered
// before. With no labels, nothing is watched.
//
// A label with no running process is not an error and not an exit: the job may not have started
// yet. Nothing is inferred from its absence -- the supervisor calls Watch again on every tick, and
// a job that never starts at all is caught by the deadline, not here. Inferring an exit from an
// absent process would revert every experiment during the seconds between bootstrap and launch.
func (w *KqueueWatcher) Watch(ctx context.Context, labels []string) error {
	resolved := map[int]string{}
	var errs []error
	for _, label := range labels {
		status, err := w.launchctl.Print(ctx, label)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if status.PID == 0 {
			continue
		}
		resolved[status.PID] = label
	}

	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return errors.New("the process watcher is closed")
	}

	// Registrations are per-PID and a process that has exited unregisters itself, so the only
	// work here is adding what is newly running. Dropping registrations for PIDs that are no
	// longer reported would lose the exit of a process that died between the print and now.
	for pid, label := range resolved {
		if _, alreadyWatched := w.watched[pid]; alreadyWatched {
			continue
		}
		if err := w.register(pid); err != nil {
			// A process that exited between the print above and the registration is
			// reported as an exit, which is exactly what it is.
			if errors.Is(err, unix.ESRCH) {
				w.report(ExitEvent{Label: label, PID: pid})
				continue
			}
			errs = append(errs, fmt.Errorf("could not watch %s (pid %d): %w", label, pid, err))
			continue
		}
		w.watched[pid] = label
	}
	if len(labels) == 0 {
		// Stopping is a matter of not reporting any more: the registrations are torn down by
		// the kernel when the processes exit, and an exit reported for a job nobody is
		// watching is what the supervisor's drain is for.
		w.watched = map[int]string{}
	}
	return errors.Join(errs...)
}

// Close releases the kqueue and stops the reader.
func (w *KqueueWatcher) Close() error {
	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		return nil
	}
	w.closed = true
	close(w.done)
	kq := w.kq
	w.mu.Unlock()

	// Closing the descriptor is what wakes the reader out of its blocking Kevent.
	if err := unix.Close(kq); err != nil {
		return fmt.Errorf("could not close the kqueue: %w", err)
	}
	return nil
}

// register asks the kernel to report the process's exit once.
func (w *KqueueWatcher) register(pid int) error {
	event := unix.Kevent_t{
		Ident:  uint64(pid),
		Filter: unix.EVFILT_PROC,
		Flags:  unix.EV_ADD | unix.EV_ONESHOT,
		Fflags: unix.NOTE_EXIT,
	}
	_, err := unix.Kevent(w.kq, []unix.Kevent_t{event}, nil, nil)
	return err
}

// read blocks on the kqueue and turns exits into events.
func (w *KqueueWatcher) read() {
	events := make([]unix.Kevent_t, exitBuffer)
	for {
		select {
		case <-w.done:
			return
		default:
		}

		w.mu.Lock()
		kq := w.kq
		closed := w.closed
		w.mu.Unlock()
		if closed {
			return
		}

		// A nil timeout blocks until something happens or the descriptor is closed, which is
		// what Close does. There is no poll interval to tune.
		n, err := unix.Kevent(kq, nil, events, nil)
		if err != nil {
			if errors.Is(err, unix.EINTR) {
				continue
			}
			select {
			case <-w.done:
			default:
				log.Warnf("The experiment process watcher stopped reading: %v", err)
			}
			return
		}
		for i := 0; i < n; i++ {
			event := events[i]
			if event.Filter != unix.EVFILT_PROC || event.Fflags&unix.NOTE_EXIT == 0 {
				continue
			}
			pid := int(event.Ident)
			w.mu.Lock()
			label, watched := w.watched[pid]
			delete(w.watched, pid)
			w.mu.Unlock()
			if !watched {
				continue
			}
			// The exit status is not in the notification in a form worth trusting
			// across macOS releases, so it is read back from launchd, which recorded it.
			// Failing to read it is not failing to report the exit.
			exitStatus := 0
			if status, err := w.launchctl.Print(context.Background(), label); err == nil {
				exitStatus = status.LastExitStatus
			}
			w.report(ExitEvent{Label: label, PID: pid, ExitStatus: exitStatus})
		}
	}
}

// report queues an event, dropping it if the supervisor is somehow that far behind. Dropping is
// safe: the deadline still bounds the experiment, and losing one of several exits during the same
// experiment costs nothing, because the first one to be read reverts.
func (w *KqueueWatcher) report(event ExitEvent) {
	select {
	case w.exits <- event:
	default:
		log.Warnf("Dropped the exit of %s (pid %d): the supervisor is not consuming events", event.Label, event.PID)
	}
}
