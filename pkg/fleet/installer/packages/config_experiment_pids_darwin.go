// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

package packages

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/launchd"
)

// startupPollInterval and startupPollTimeout bound how long resolveExperimentPids waits for a job
// that Kickstart was already told to start, but that launchd has not yet reported a pid for.
const startupPollInterval = 100 * time.Millisecond

var startupPollTimeout = 5 * time.Second

// resolveExperimentPids waits for every job in the set to either report a running pid or a
// recorded exit, bounded by startupPollTimeout. Kickstart was already issued for every job in the
// set before this runs, so a healthy job should acquire a pid almost immediately; the bound only
// covers the gap between that request and this process actually getting to check.
//
// It returns the running jobs' pids, each mapped to its label without the variant suffix. A job
// still pending once startupPollTimeout is up is an error: it failed to start.
//
// A job that launchd recorded as having exited in a way that fails the experiment
// (exitFailsExperiment) stops the wait at once and is returned in exited: that is a durable fact
// recorded by launchd, not something more waiting would resolve. Any other job recorded as having
// exited cleanly (LastExitStatus zero) is not: a job whose feature is off exits that way the
// instant it is kickstarted, and this wait must tolerate it.
func resolveExperimentPids(ctx context.Context, jobs launchd.JobSet) (jobPIDs map[int]string, exited *launchd.JobStatus, err error) {
	giveUp := time.Now().Add(startupPollTimeout)
	for {
		statuses, err := jobs.Statuses(ctx, launchd.Experiment)
		if err != nil {
			return nil, nil, err
		}
		jobPIDs = make(map[int]string, len(statuses))
		var pending []string
		for i, status := range statuses {
			label := strings.TrimSuffix(status.Label, string(launchd.Experiment))
			if status.PID != 0 {
				jobPIDs[status.PID] = label
				continue
			}
			if status.HasExited && exitFailsExperiment(label, status.LastExitStatus == 0) {
				return nil, &statuses[i], nil
			}
			if status.HasExited {
				continue
			}
			pending = append(pending, status.Label)
		}
		if len(pending) == 0 {
			return jobPIDs, nil, nil
		}
		if time.Now().After(giveUp) {
			return nil, nil, fmt.Errorf("experiment jobs %s did not start within %s", strings.Join(pending, ", "), startupPollTimeout)
		}
		select {
		case <-ctx.Done():
			return jobPIDs, nil, nil
		case <-time.After(startupPollInterval):
		}
	}
}

// essentialJobs are the jobs that must keep running for as long as an experiment does: the Agent,
// and the installer daemon, the only process that can stop or promote the experiment. Either one
// exiting fails the experiment, even cleanly. The other jobs exit cleanly by design when their
// feature is off.
var essentialJobs = []string{agentJob, installerJob}

// exitFailsExperiment reports whether an experiment job exiting, cleanly or not, fails the
// experiment. label is the job's label without its variant suffix.
func exitFailsExperiment(label string, clean bool) bool {
	return !clean || slices.Contains(essentialJobs, label)
}

// exitedCleanly reports whether a wait(2) status word (as delivered by NOTE_EXITSTATUS) describes
// a normal, voluntary exit(0). For a job outside essentialJobs, whose feature may be off, that is
// expected lifecycle, not a crash, and must not cost the rest of the set an otherwise-healthy
// experiment.
//
// A pid that had already exited before ArmExitObserver could arm it also reports status 0 (its
// real exit status is unrecoverable by then, see ArmExitObserver's doc comment on that residual
// race): this treats that case the same as a clean exit rather than a crash, narrowing an already
// zero-width, documented race rather than introducing a new one. For an essential job either way
// is a failure.
func exitedCleanly(status int) bool {
	ws := unix.WaitStatus(status)
	return ws.Exited() && ws.ExitStatus() == 0
}
