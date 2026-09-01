// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package experiment

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// ExitEvent is the terminal exit of an experiment job.
//
// It is terminal by construction: the -exp job definitions carry no KeepAlive, so launchd does not
// relaunch them. An -exp process that is gone is a decision about the version, not a symptom to be
// papered over with a restart.
type ExitEvent struct {
	// Label is the launchd label of the job that exited.
	Label string
	// PID is the process that exited.
	PID int
	// ExitStatus is what launchd recorded, when it could be read. Zero means either a clean
	// exit or an unknown one; the supervisor reverts either way, so it does not have to tell
	// them apart.
	ExitStatus int
}

// ProcWatcher reports the exits of the experiment jobs it is watching.
type ProcWatcher interface {
	// Watch starts watching the given launchd labels. Calling it again replaces the watched
	// set, and calling it with no labels stops watching. It is expected to be called
	// repeatedly: a job that had not started yet the last time is picked up on a later call.
	Watch(ctx context.Context, labels []string) error
	// Exits yields one event per observed exit. The channel is never closed while the watcher
	// is open.
	Exits() <-chan ExitEvent
	// Close releases the watcher's resources.
	Close() error
}

// RevertFunc puts the host back on the stable version.
//
// There is exactly one, and every path that decides an experiment is over goes through it: the
// deadline passing, an experiment job exiting, and a daemon starting up to find an expired
// deadline. A second implementation of "put it back" is a second chance to get the ordering wrong.
type RevertFunc func(ctx context.Context, version string, reason string) error

// Supervisor bounds a running experiment.
type Supervisor struct {
	// mu serializes Arm, Disarm, Tick and Reconcile against each other. Tick runs on the
	// daemon's ticker while Arm and Disarm run on the request-handling path, and a Tick that
	// interleaved with an Arm could revert an experiment that had just been started.
	mu sync.Mutex

	deadline *Deadline
	watcher  ProcWatcher
	revert   RevertFunc

	// labels is the set of experiment job labels watched while an experiment is running.
	labels []string

	// now is time.Now, replaced in tests.
	now func() time.Time

	// armed mirrors whether the watcher is currently watching. It is a cache of what the
	// deadline file says, never the source of truth for whether an experiment is running.
	armed bool
}

// NewSupervisor returns a Supervisor over the given labels.
//
// watcher may be nil, in which case the supervisor bounds the experiment by its deadline alone.
// That is a degraded mode, not a supported configuration: without the watcher an experiment that
// crashes on startup burns its whole deadline before the host is back on stable.
func NewSupervisor(deadline *Deadline, watcher ProcWatcher, labels []string, revert RevertFunc) *Supervisor {
	return &Supervisor{
		deadline: deadline,
		watcher:  watcher,
		revert:   revert,
		labels:   labels,
		now:      time.Now,
	}
}

// Arm records that an experiment is running and starts watching its jobs.
//
// It is normally called before the experiment jobs are started, so there is no window in which an
// experiment runs unbounded. The watcher therefore resolves no processes on this first call, which
// is why Watch is re-issued on every Tick: a job that had not started yet is picked up then.
func (s *Supervisor) Arm(ctx context.Context, version string, duration time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	duration = ClampDuration(duration)
	expiresAt := s.now().Add(duration)
	if err := s.deadline.Set(version, expiresAt); err != nil {
		return err
	}
	log.Infof("Experiment %s will be reverted at %s unless it is promoted or stopped first", version, expiresAt.Format(time.RFC3339))
	s.armed = true
	// A watcher that cannot be started is logged rather than returned: the deadline is already
	// recorded, so the experiment is still bounded, and failing the start here would leave the
	// host on an experiment nobody armed.
	if err := s.startWatch(ctx); err != nil {
		log.Warnf("Could not watch the experiment jobs, falling back to the deadline alone: %v", err)
	}
	return nil
}

// Disarm records that no experiment is running and stops watching.
//
// It is called on both of the paths that end an experiment deliberately -- promote and stop -- and
// on the tail of a revert. The deadline is cleared last, after the watch is stopped, so no exit
// caused by the shutdown of the experiment jobs can be read as a reason to revert an experiment
// that is already over.
func (s *Supervisor) Disarm(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.disarmLocked(ctx)
}

func (s *Supervisor) disarmLocked(ctx context.Context) error {
	s.stopWatch(ctx)
	s.armed = false
	return s.deadline.Clear()
}

// Reconcile brings the supervisor in line with what is on disk. It is called once when the daemon
// starts.
//
// This is where a reboot or a daemon restart mid-experiment is handled. The deadline file is the
// only thing that survives either, so the three cases are: no deadline, and there is nothing to
// supervise; a deadline in the future, and the watch is resumed for what is left of it; and a
// deadline already passed, which is the case that matters -- the host has been running an
// experiment unsupervised for however long the daemon was down, and it goes back to stable now.
func (s *Supervisor) Reconcile(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	version, expiresAt, set, err := s.deadline.Get()
	if err != nil {
		return err
	}
	if !set {
		s.armed = false
		return nil
	}
	if !s.now().Before(expiresAt) {
		log.Warnf("Found experiment %s past its deadline of %s at startup, reverting", version, expiresAt.Format(time.RFC3339))
		return s.revertLocked(ctx, version, fmt.Sprintf("the experiment deadline of %s passed while the installer was not running", expiresAt.Format(time.RFC3339)))
	}
	log.Infof("Resuming supervision of experiment %s, which is due to be reverted at %s", version, expiresAt.Format(time.RFC3339))
	s.armed = true
	if err := s.startWatch(ctx); err != nil {
		log.Warnf("Could not resume watching the experiment jobs, falling back to the deadline alone: %v", err)
	}
	return nil
}

// Tick is called from the daemon's existing ticker. It reverts when the experiment is over.
//
// The exits are consumed before the deadline is examined, because an experiment job that has
// exited is a stronger and earlier signal than a deadline that has not yet passed, and because
// leaving events in the channel would have them decide a later experiment.
func (s *Supervisor) Tick(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	version, expiresAt, set, err := s.deadline.Get()
	if err != nil {
		return err
	}
	if !set {
		// No experiment is running. Drain anything the watcher observed so a stale exit
		// cannot decide the next experiment, and make sure the watch is down.
		s.drain()
		if s.armed {
			s.stopWatch(ctx)
			s.armed = false
		}
		return nil
	}

	if event, ok := s.nextExit(); ok {
		log.Warnf("Experiment job %s (pid %d) exited with status %d, reverting experiment %s", event.Label, event.PID, event.ExitStatus, version)
		return s.revertLocked(ctx, version, fmt.Sprintf("the %s experiment job exited with status %d", event.Label, event.ExitStatus))
	}

	if !s.now().Before(expiresAt) {
		log.Warnf("Experiment %s reached its deadline of %s, reverting", version, expiresAt.Format(time.RFC3339))
		return s.revertLocked(ctx, version, fmt.Sprintf("the experiment deadline of %s passed", expiresAt.Format(time.RFC3339)))
	}

	// Re-resolve the watched set. A job that was still starting when the experiment was armed
	// has a PID now, and a job launchd restarted out from under us has a different one.
	if err := s.startWatch(ctx); err != nil {
		log.Debugf("Could not refresh the experiment job watch: %v", err)
	}
	return nil
}

// revertLocked runs the revert callback and only then clears the deadline.
//
// The watch is stopped first: the revert boots out the experiment jobs, and their exits are a
// consequence of the revert rather than a reason for another one.
//
// The deadline is cleared last, and only if the revert succeeded. A revert that fails leaves the
// deadline in place, already expired, so the next Tick tries again -- which is the behaviour that
// makes a transient failure survivable. It also means a revert that fails permanently is retried
// every tick; that is deliberate, because the alternative is a host left on an experiment with
// nothing watching it, and each attempt logs why it failed.
func (s *Supervisor) revertLocked(ctx context.Context, version string, reason string) error {
	s.stopWatch(ctx)
	s.armed = false
	s.drain()

	if s.revert == nil {
		return fmt.Errorf("no revert is configured, cannot end experiment %s: %s", version, reason)
	}
	if err := s.revert(ctx, version, reason); err != nil {
		return fmt.Errorf("could not revert experiment %s (%s): %w", version, reason, err)
	}
	if err := s.deadline.Clear(); err != nil {
		return err
	}
	log.Infof("Reverted experiment %s: %s", version, reason)
	return nil
}

func (s *Supervisor) startWatch(ctx context.Context) error {
	if s.watcher == nil || len(s.labels) == 0 {
		return nil
	}
	return s.watcher.Watch(ctx, s.labels)
}

func (s *Supervisor) stopWatch(ctx context.Context) {
	if s.watcher == nil {
		return
	}
	if err := s.watcher.Watch(ctx, nil); err != nil {
		log.Warnf("Could not stop watching the experiment jobs: %v", err)
	}
}

func (s *Supervisor) nextExit() (ExitEvent, bool) {
	if s.watcher == nil {
		return ExitEvent{}, false
	}
	select {
	case event := <-s.watcher.Exits():
		return event, true
	default:
		return ExitEvent{}, false
	}
}

func (s *Supervisor) drain() {
	if s.watcher == nil {
		return
	}
	for {
		select {
		case <-s.watcher.Exits():
		default:
			return
		}
	}
}
