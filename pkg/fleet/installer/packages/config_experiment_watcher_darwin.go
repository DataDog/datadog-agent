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
	defer observer.Disarm()

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
			reason := fmt.Sprintf("experiment pid %d exited (status %d)", ev.Pid, ev.Status)
			acted, err := revertExperimentIfStillPending(ctx, deadline, reason)
			if err != nil {
				return err
			}
			if acted {
				return nil
			}
		case <-ticker.C:
			expired, err := deadline.Expired(configExperimentDeadlineWindow)
			if err != nil {
				log.Errorf("watcher: could not check experiment deadline: %v", err)
				continue
			}
			if !expired {
				continue
			}
			acted, err := revertExperimentIfStillPending(ctx, deadline, "experiment deadline expired")
			if err != nil {
				return err
			}
			if acted {
				return nil
			}
		}
	}
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
