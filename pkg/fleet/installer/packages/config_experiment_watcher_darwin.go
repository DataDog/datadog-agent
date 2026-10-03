// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

package packages

import (
	"context"
	"fmt"
	"time"

	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/launchd"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// deadlineTickInterval is how often watchExperiment polls the deadline file while waiting on
// the exit observer, per the RFC's deadline-tick design.
const deadlineTickInterval = 30 * time.Second

// startupPollInterval and startupPollTimeout bound how long watchExperiment waits for a job that
// Kickstart was already told to start, but that launchd has not yet reported a pid for, before
// treating it as failed to launch. This process is spawned detached, after Start() has already
// returned, so a healthy job can legitimately still be forking when this runs; the bound exists
// only to distinguish that from a job that will never come up.
const (
	startupPollInterval = 100 * time.Millisecond
	startupPollTimeout  = 5 * time.Second
)

// watchExperiment runs in a detached process, launched by postStartConfigExperimentDatadogAgent
// after a configuration experiment has started successfully. It supervises the experiment job
// set for as long as the deadline file (configExperimentDeadlinePath) still holds the deadline it
// read on start, reverting to the stable set if an experiment job exits abnormally or if the
// deadline expires.
//
// A deliberate stop or promote (configExperiment.restoreStable) clears the deadline file before
// touching any job, which is this design's only signal between the two processes: there is no
// way to send this process a message, so every event handler below re-checks the file
// immediately before acting, and treats a cleared or rewritten file as "this experiment is over,
// someone else is handling it" rather than a crash to revert. The same check on every tick is
// what lets this process exit after a stop whose jobs all exited cleanly, which delivers no event
// it acts on.
func watchExperiment(ctx context.Context) error {
	deadline := launchd.Deadline{Path: configExperimentDeadlinePath}
	jobs := agentJobSet()

	token, err := deadline.Read()
	if err != nil {
		return fmt.Errorf("watcher: could not read experiment deadline: %w", err)
	}
	if token == "" {
		// Stopped or promoted before this process got to run: nothing left to supervise.
		return nil
	}

	pids, exited, err := resolveExperimentPids(ctx, jobs)
	if err != nil {
		return fmt.Errorf("watcher: could not resolve experiment pids: %w", err)
	}
	if exited != nil {
		// launchd itself recorded this job's exit -- a durable fact, not a timing guess --
		// before the watcher ever got a chance to arm an observer on it. As real a crash as
		// one caught mid-watch: revert immediately rather than silently leaving an
		// unsupervised, already-dead experiment in place.
		reason := fmt.Sprintf("experiment job %s exited before the watcher could observe it (status %d)", exited.Label, exited.LastExitStatus)
		_, err := revertExperimentIfStillPending(ctx, deadline, token, reason)
		return err
	}
	if len(pids) == 0 {
		// Every job in the set failed to come up at all within the startup grace period,
		// and none of them recorded an exit either -- Kickstart was accepted but nothing
		// ever ran. Treat that as a failed launch, same as a crash.
		_, err := revertExperimentIfStillPending(ctx, deadline, token, "experiment never started")
		return err
	}

	observer, exitCh, err := launchd.ArmExitObserver(pids)
	if err != nil {
		return fmt.Errorf("watcher: could not arm exit observer: %w", err)
	}
	defer func() {
		if err := observer.Disarm(); err != nil {
			log.Warnf("watcher: could not disarm exit observer: %v", err)
		}
	}()

	ticker := time.NewTicker(deadlineTickInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case ev, ok := <-exitCh:
			if !ok {
				// Every armed pid has delivered its one-shot event; nothing left to
				// receive from this channel, but the deadline tick still runs.
				exitCh = nil
				continue
			}
			if exitedCleanly(ev.Status) {
				// A job whose feature is disabled (e.g. system-probe with no module
				// enabled) exits on its own moments after being kickstarted, by
				// design -- the stable job set does exactly the same and nothing
				// watches it for that. Only an abnormal exit of a sibling job should
				// pull the rest of the set down with it.
				continue
			}
			reason := fmt.Sprintf("experiment pid %d exited (status %d)", ev.Pid, ev.Status)
			_, err := revertExperimentIfStillPending(ctx, deadline, token, reason)
			if err != nil {
				return err
			}
			// Whether this call reverted or found the deadline already cleared by a
			// deliberate stop/promote, the experiment this watcher was supervising is no
			// longer pending, so there is nothing left for this process to do.
			return nil
		case <-ticker.C:
			current, err := deadline.Read()
			if err != nil {
				log.Errorf("watcher: could not read experiment deadline: %v", err)
				continue
			}
			if current != token {
				// Stopped, promoted, or replaced by a newer experiment with its own watcher.
				return nil
			}
			expired, err := deadline.Expired(configExperimentDeadlineWindow)
			if err != nil {
				log.Errorf("watcher: could not check experiment deadline: %v", err)
				continue
			}
			if !expired {
				continue
			}
			if _, err := revertExperimentIfStillPending(ctx, deadline, token, "experiment deadline expired"); err != nil {
				return err
			}
			return nil
		}
	}
}

// resolveExperimentPids waits for every job in the set to either report a running pid or a
// recorded exit, bounded by startupPollTimeout. Kickstart was already issued for every job
// before this process was even spawned -- this runs detached, launched only after Start() has
// returned -- so a healthy job should acquire a pid almost immediately; the bound only covers
// the gap between that request and this process actually getting to check.
//
// A job that launchd itself recorded as an abnormal exit stops the wait immediately and is
// returned in exited, regardless of its siblings' state: that is a durable fact recorded by
// launchd, not something more waiting would resolve. A job recorded as having exited cleanly
// (LastExitStatus zero) is not: agentJobSet swaps its three jobs as one unit, and a job whose
// feature is off exits that way the instant it is kickstarted, same as the stable set -- exactly
// the case exitedCleanly exists to tolerate later in watchExperiment, and this pre-check must
// tolerate it too, or every experiment with one disabled feature reverts itself on startup before
// the caller ever gets a chance to observe it running. Unlike exitedCleanly's wait(2) status word,
// LastExitStatus is launchd's own simple signed code, so a direct zero check is what it takes to
// tell the two apart here.
func resolveExperimentPids(ctx context.Context, jobs launchd.JobSet) (pids []int, exited *launchd.JobStatus, err error) {
	giveUp := time.Now().Add(startupPollTimeout)
	for {
		statuses, err := jobs.Statuses(ctx, launchd.Experiment)
		if err != nil {
			return nil, nil, err
		}
		pids = pids[:0]
		pending := false
		for i, status := range statuses {
			if status.PID != 0 {
				pids = append(pids, status.PID)
				continue
			}
			if status.HasExited && status.LastExitStatus != 0 {
				return nil, &statuses[i], nil
			}
			if status.HasExited {
				continue
			}
			pending = true
		}
		if !pending || time.Now().After(giveUp) {
			return pids, nil, nil
		}
		select {
		case <-ctx.Done():
			return pids, nil, nil
		case <-time.After(startupPollInterval):
		}
	}
}

// exitedCleanly reports whether a wait(2) status word (as delivered by NOTE_EXITSTATUS) describes
// a normal, voluntary exit(0). agentJobSet swaps agent, sysprobe and data-plane as one unit, and a
// job whose feature is off exits this way the instant it is kickstarted -- that is expected
// lifecycle, not a crash, and must not cost the rest of the set an otherwise-healthy experiment.
//
// A pid that had already exited before ArmExitObserver could arm it also reports status 0 (its
// real exit status is unrecoverable by then, see ArmExitObserver's doc comment on that residual
// race): this treats that case the same as a clean exit rather than a crash, narrowing an already
// zero-width, documented race rather than introducing a new one.
func exitedCleanly(status int) bool {
	ws := unix.WaitStatus(status)
	return ws.Exited() && ws.ExitStatus() == 0
}

// revertExperimentIfStillPending reverts to the stable job set unless the deadline file no longer
// holds token: cleared by a deliberate stop or promote, in which case that path owns the outcome,
// or rewritten by a newer experiment that is not this caller's to revert. Either way this call is
// a no-op. Reports whether it reverted.
//
// A deliberate stop or promote, driven by the daemon, also discards or promotes the experiment
// configuration directory itself (RemoveConfigExperiment's i.config.RemoveExperiment, called
// after the package's own Stop hook). This path runs detached from the daemon and has no access
// to its installer/db state, but the directory swap it still owes is a plain filesystem
// operation -- restoring the experiment link to resting -- so revertExperiment does it directly
// (see discardExperimentConfig).
func revertExperimentIfStillPending(ctx context.Context, deadline launchd.Deadline, token string, reason string) (bool, error) {
	current, err := deadline.Read()
	if err != nil {
		return false, fmt.Errorf("watcher: could not read experiment deadline: %w", err)
	}
	if current != token {
		return false, nil
	}
	log.Warnf("watcher: reverting configuration experiment: %s", reason)
	if err := revertExperiment(ctx); err != nil {
		return false, fmt.Errorf("watcher: %w", err)
	}
	return true, nil
}

// revertExperiment hands the Agent back to the stable job set and discards the experiment
// configuration directory, leaving the host as if no experiment had ever been started.
func revertExperiment(ctx context.Context) error {
	if err := (configExperiment{jobs: agentJobSet()}).Stop(context.WithoutCancel(ctx)); err != nil {
		return fmt.Errorf("could not revert experiment: %w", err)
	}
	return discardExperimentConfig(ctx)
}
