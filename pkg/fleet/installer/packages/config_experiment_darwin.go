// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

package packages

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/launchd"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/paths"
)

const (
	// configExperimentDeadlineWindow bounds how long an unsupervised configuration experiment
	// may run before the watcher (config_experiment_watcher_darwin.go) treats it as hung and
	// reverts it. There is no existing signal in this codebase to size the window from, so it
	// shares Windows's default experiment watchdog timeout (getWatchdogTimeout in
	// datadog_agent_windows.go, 60 minutes) rather than inventing an unrelated number.
	configExperimentDeadlineWindow = 60 * time.Minute

	// configExperimentRestoreRetries bounds restoreStableJobs's retries of the two steps PR
	// #55770's review flagged as unrecoverable: rewriting and restarting the stable job set. A
	// transient disk or launchd hiccup is the most plausible failure here; anything that still
	// fails after this many attempts is reported, not retried forever.
	configExperimentRestoreRetries = 3
)

// configExperimentRestoreBackoff is the fixed delay between restoreStableJobs attempts. A
// package-level var, not a constant, so tests can shrink it instead of a retrying test taking
// several real seconds.
var configExperimentRestoreBackoff = 2 * time.Second

// configExperimentDeadlinePath is where the watcher's deadline is persisted. A package-level var,
// not a constant, so tests can point it at a temporary file instead of the real, root-owned run
// directory — the same indirection launchdJobDir uses for job definitions.
var configExperimentDeadlinePath = filepath.Join(paths.RunPath, "experiment-deadline")

// configExperiment swaps the Agent between its stable and its experiment launchd job set.
//
// It owns the jobs and the deadline file that bounds how long an experiment may run
// unsupervised (config_experiment_watcher_darwin.go watches the jobs and the deadline in a
// separate, detached process; this type never talks to that process, it only leaves and clears
// the file the watcher reads). The configuration directories are the installer's own business:
// it publishes the experiment directory before calling the post-start hook, and it discards or
// promotes it around the stop and promote hooks. So by the time any method here runs, the
// directory the incoming job set is about to read is already in place.
//
// The installer daemon is deliberately not part of the set. It is the process performing the
// swap, and a set that contained it would stop itself halfway through.
type configExperiment struct {
	jobs launchd.JobSet
}

// Start hands the Agent over to the experiment job set.
//
// The deadline is written before anything else is touched: an orphaned deadline with no
// experiment is harmless, but an experiment with no deadline is unbounded and silent. If the
// handover fails partway — the case PR #55770's review flagged, where the stable set had
// already been stopped — the stable set is put back rather than left down with nothing in its
// place.
func (e configExperiment) Start(ctx context.Context) error {
	deadline := launchd.Deadline{Path: configExperimentDeadlinePath}
	if err := deadline.Write(configExperimentDeadlineWindow); err != nil {
		return err
	}
	if err := e.jobs.Stop(ctx, launchd.Stable); err != nil {
		_ = deadline.Clear()
		return err
	}
	if err := e.jobs.Write(launchd.Experiment); err != nil {
		return e.abortStart(ctx, err)
	}
	if err := e.jobs.Start(ctx, launchd.Experiment); err != nil {
		return e.abortStart(ctx, err)
	}
	return nil
}

// Resume re-establishes the experiment job set after an unsupervised restart (daemon restart,
// crash, reboot) finds the deadline still valid. Unlike Start, it does not touch the deadline
// file: the deadline bounds time since the experiment originally started, and rewriting it here
// would let a resumed experiment outlive that original bound.
func (e configExperiment) Resume(ctx context.Context) error {
	if err := e.jobs.Stop(ctx, launchd.Stable); err != nil {
		return err
	}
	if err := e.jobs.Write(launchd.Experiment); err != nil {
		return e.abortStart(ctx, err)
	}
	if err := e.jobs.Start(ctx, launchd.Experiment); err != nil {
		return e.abortStart(ctx, err)
	}
	return nil
}

// abortStart puts the stable jobs back after a failed handover to the experiment set. It
// reports the original failure, not whatever the rollback itself returns, unless the rollback's
// own retries are exhausted — in which case that error better describes the host's actual
// state.
func (e configExperiment) abortStart(ctx context.Context, startErr error) error {
	if err := e.restoreStable(context.WithoutCancel(ctx)); err != nil {
		return fmt.Errorf("experiment failed to start (%w) and could not be rolled back: %w", startErr, err)
	}
	return startErr
}

// Stop hands the Agent back to the stable job set, abandoning the experiment.
func (e configExperiment) Stop(ctx context.Context) error {
	return e.restoreStable(ctx)
}

// Promote hands the Agent back to the stable job set, which now reads the promoted configuration.
//
// The work is the same as Stop's: what makes this a promotion rather than a rollback happened in
// the configuration directories before the hook ran. The two are kept apart because they are
// distinct outcomes to a reader of the code and of the traces, and because anything that later
// times an experiment out has to distinguish them.
func (e configExperiment) Promote(ctx context.Context) error {
	return e.restoreStable(ctx)
}

// restoreStable unloads the experiment set, removes its definitions and loads the stable set.
//
// The definitions are removed, not just unloaded, so a host that has finished with an experiment
// is indistinguishable from one that never ran one — including to an operator reading
// /Library/LaunchDaemons.
//
// The deadline is cleared first, not last: nothing signals the detached watcher
// (config_experiment_watcher_darwin.go) that a deliberate stop is starting, so the cleared
// deadline file is itself that signal. Clearing it after jobs.Stop below would race the
// watcher's own exit observer, which fires the instant that bootout kills the experiment jobs —
// both this method and the watcher would then try to swap the job sets at once.
func (e configExperiment) restoreStable(ctx context.Context) error {
	if err := (launchd.Deadline{Path: configExperimentDeadlinePath}).Clear(); err != nil {
		return err
	}
	if err := e.jobs.Stop(ctx, launchd.Experiment); err != nil {
		return err
	}
	if err := e.jobs.Remove(launchd.Experiment); err != nil {
		return err
	}
	return e.restoreStableJobs(ctx)
}

// restoreStableJobs rewrites and restarts the stable job set, retrying a bounded number of times
// on failure before giving up. This is the second gap PR #55770's review flagged: previously, a
// failure here left the host with neither job set running, and nothing retried. Once the
// retries are exhausted, the error is reported and there is no further automated recovery — the
// deadline is already cleared by this point, so the watcher does not act as a backstop for a
// failed rollback, only for a hung experiment.
func (e configExperiment) restoreStableJobs(ctx context.Context) error {
	var lastErr error
	for attempt := 0; attempt < configExperimentRestoreRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(configExperimentRestoreBackoff)
		}
		// The stable definitions are rewritten rather than assumed present: bootstrap needs
		// the file on disk, and this is the path a host takes back to a working state.
		if err := e.jobs.Write(launchd.Stable); err != nil {
			lastErr = err
			continue
		}
		if err := e.jobs.Start(ctx, launchd.Stable); err != nil {
			lastErr = err
			continue
		}
		return nil
	}
	return fmt.Errorf("could not restore the stable job set after %d attempts: %w", configExperimentRestoreRetries, lastErr)
}
