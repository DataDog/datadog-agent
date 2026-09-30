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

// watchExperiment runs in a detached process, launched by postStartConfigExperimentDatadogAgent
// after a configuration experiment has started successfully. It supervises the experiment job
// set for as long as the deadline file (configExperimentDeadlinePath) exists, reverting to the
// stable set if every experiment job exits or if the deadline expires.
//
// A deliberate stop or promote (configExperiment.restoreStable) clears the deadline file before
// touching any job, which is this design's only signal between the two processes: there is no
// way to send this process a message, so every event handler below re-checks the file's
// presence immediately before acting, and treats an already-cleared file as "someone else is
// already handling this" rather than a crash to revert.
func watchExperiment(ctx context.Context) error {
	deadline := launchd.Deadline{Path: configExperimentDeadlinePath}
	jobs := agentJobSet()

	pids, err := jobs.Pids(ctx, launchd.Experiment)
	if err != nil {
		return fmt.Errorf("watcher: could not resolve experiment pids: %w", err)
	}
	if len(pids) == 0 {
		// Every job in the set had already exited by the time the watcher could resolve
		// pids -- as real a crash as one caught mid-watch. Revert immediately rather than
		// silently leaving an unsupervised, already-dead experiment in place.
		_, err := revertExperimentIfStillPending(ctx, deadline, "experiment exited before the watcher could observe it")
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
			_, err := revertExperimentIfStillPending(ctx, deadline, reason)
			if err != nil {
				return err
			}
			// Whether this call reverted or found the deadline already cleared by a
			// deliberate stop/promote, the experiment this watcher was supervising is no
			// longer pending -- staying alive any longer would leave an orphan process
			// whose later, delayed exit events could race a *different* experiment that
			// later reuses the same shared deadline file (the file carries no per-
			// experiment identity), reverting it by mistake.
			return nil
		case <-ticker.C:
			expired, err := deadline.Expired(configExperimentDeadlineWindow)
			if err != nil {
				log.Errorf("watcher: could not check experiment deadline: %v", err)
				continue
			}
			if !expired {
				continue
			}
			if _, err := revertExperimentIfStillPending(ctx, deadline, "experiment deadline expired"); err != nil {
				return err
			}
			return nil
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

// revertExperimentIfStillPending reverts to the stable job set unless the deadline file has
// already been cleared by a deliberate stop or promote, in which case that path owns the
// outcome and this call is a no-op. Reports whether it reverted.
func revertExperimentIfStillPending(ctx context.Context, deadline launchd.Deadline, reason string) (bool, error) {
	present, err := deadline.Present()
	if err != nil {
		return false, fmt.Errorf("watcher: could not check deadline presence: %w", err)
	}
	if !present {
		return false, nil
	}
	log.Warnf("watcher: reverting configuration experiment: %s", reason)
	if err := (configExperiment{jobs: agentJobSet()}).Stop(context.WithoutCancel(ctx)); err != nil {
		return false, fmt.Errorf("watcher: could not revert experiment: %w", err)
	}
	return true, nil
}
