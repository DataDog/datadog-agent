// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

package packages

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/launchd"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// resumeConfigExperimentDatadogAgent is run once when the daemon starts, to recover a
// configuration experiment left running unsupervised because the detached watcher process
// (config_experiment_watcher_darwin.go) that was tracking it died along with everything else in
// an abrupt shutdown -- a crash, a kill, a reboot -- before it could revert or hand off.
//
// The deadline file is the only durable signal of this, since the watcher is a bare process with
// no launchd job and nothing else re-spawns it: a present, unexpired deadline means an experiment
// was mid-flight with nothing currently watching it, so the experiment job set and its watcher
// are re-established here. A present, expired deadline means the unsupervised window was already
// exceeded before the daemon got a chance to resume it, so the experiment is reverted instead.
// No deadline at all, but an experiment configuration directory still deployed, is reverted too
// (see revertOrphanedExperiment).
func resumeConfigExperimentDatadogAgent(ctx HookContext) error {
	deadline := launchd.Deadline{Path: configExperimentDeadlinePath}
	token, err := deadline.Read()
	if err != nil {
		return fmt.Errorf("resume: could not read experiment deadline: %w", err)
	}
	if token == "" {
		return revertOrphanedExperiment(ctx)
	}

	expired, err := deadline.Expired(configExperimentDeadlineWindow)
	if err != nil {
		return fmt.Errorf("resume: could not check experiment deadline: %w", err)
	}
	if expired {
		log.Warnf("resume: configuration experiment deadline already expired before the daemon could resume it, reverting")
		_, err := revertExperimentIfStillPending(ctx, deadline, token, "experiment deadline expired before the daemon could resume it")
		return err
	}

	log.Infof("resume: configuration experiment still within its deadline, resuming supervision")
	// A daemon restart does not take the previous watcher with it: it is a detached process. It is
	// still armed on the pids Resume is about to bounce, so it would read their termination as a
	// crash and revert the experiment under the resume. Stop it first; a new one is launched below.
	if err := stopSurvivingWatchers(ctx); err != nil {
		log.Warnf("resume: could not stop the previous watcher, resuming anyway: %v", err)
	}
	if err := (configExperiment{jobs: agentJobSet()}).Resume(ctx); err != nil {
		log.Errorf("resume: could not resume the configuration experiment job set, reverting: %v", err)
		if _, revertErr := revertExperimentIfStillPending(ctx, deadline, token, "experiment could not be resumed"); revertErr != nil {
			return fmt.Errorf("resume failed (%w) and the experiment could not be reverted: %w", err, revertErr)
		}
		return fmt.Errorf("resume failed, experiment reverted: %w", err)
	}
	if err := launchConfigExperimentWatcher(ctx); err != nil {
		log.Errorf("resume: could not relaunch the configuration experiment watcher, reverting: %v", err)
		if revertErr := (configExperiment{jobs: agentJobSet()}).Stop(context.WithoutCancel(ctx)); revertErr != nil {
			return fmt.Errorf("watcher failed to relaunch (%w) and the experiment could not be reverted: %w", err, revertErr)
		}
		return fmt.Errorf("watcher failed to relaunch, experiment reverted: %w", err)
	}
	return nil
}

// revertOrphanedExperiment reverts an experiment configuration directory that is deployed with no
// deadline beside it. Start writes the deadline before touching any job, and restoreStable clears
// it before anything else, so this is a host whose previous daemon died either after the installer
// published the directory but before the experiment job set ever started, or partway through a
// deliberate stop. Either way nothing will ever resume the experiment, yet the directory alone
// would have the daemon report it as the running configuration indefinitely.
func revertOrphanedExperiment(ctx HookContext) error {
	state, err := configExperimentDirs.GetState()
	if err != nil {
		return fmt.Errorf("resume: could not read the configuration state: %w", err)
	}
	if state.ExperimentDeploymentID == "" {
		return nil
	}
	log.Warnf("resume: configuration experiment %s is deployed with no deadline, reverting", state.ExperimentDeploymentID)
	if err := revertExperiment(ctx); err != nil {
		return fmt.Errorf("resume: %w", err)
	}
	return nil
}

// watcherStopTimeout bounds how long stopSurvivingWatchers waits for a watcher to exit after
// SIGTERM before it is killed.
const watcherStopTimeout = 5 * time.Second

// stopSurvivingWatchers terminates the watcher processes a previous run of the daemon left
// behind, and waits for them to exit. A var so tests can observe when resume calls it.
//
// The watchers are found by listing processes, and only root-owned processes running exactly the
// watcher command are matched.
var stopSurvivingWatchers = func(ctx context.Context) error {
	out, err := exec.CommandContext(ctx, "/bin/ps", "-ax", "-o", "pid=,uid=,command=").Output()
	if err != nil {
		return fmt.Errorf("could not list processes: %w", err)
	}
	for _, pid := range parseWatcherPIDs(string(out), os.Getpid()) {
		log.Infof("resume: stopping the previous experiment watcher (pid %d)", pid)
		_ = syscall.Kill(pid, syscall.SIGTERM)
		stopped := time.Now().Add(watcherStopTimeout)
		for time.Now().Before(stopped) && syscall.Kill(pid, 0) == nil {
			time.Sleep(100 * time.Millisecond)
		}
		if syscall.Kill(pid, 0) == nil {
			log.Warnf("resume: the previous experiment watcher (pid %d) ignored SIGTERM, killing it", pid)
			_ = syscall.Kill(pid, syscall.SIGKILL)
		}
	}
	return nil
}

// parseWatcherPIDs returns the pids of the root-owned experiment watcher processes in the output
// of `ps -ax -o pid=,uid=,command=`, other than self.
//
// The command line must be exactly the installer binary followed by the three arguments
// launchConfigExperimentWatcher passes it, so another process whose command line merely contains
// that text -- a shell or pgrep looking for the watcher -- is never matched.
func parseWatcherPIDs(ps string, self int) []int {
	var pids []int
	for _, line := range strings.Split(ps, "\n") {
		fields := strings.Fields(line)
		// pid, uid, then the installer binary and its three arguments.
		if len(fields) != 6 {
			continue
		}
		pid, err := strconv.Atoi(fields[0])
		if err != nil || pid == self {
			continue
		}
		if fields[1] != "0" || filepath.Base(fields[2]) != "installer" {
			continue
		}
		if fields[3] != "package-command" || fields[4] != agentPackage || fields[5] != watchConfigExperimentCommand {
			continue
		}
		pids = append(pids, pid)
	}
	return pids
}
