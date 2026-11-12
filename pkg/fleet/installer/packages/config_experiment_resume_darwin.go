// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

package packages

import (
	"context"
	"errors"
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

// resumeConfigExperimentDatadogAgent is run once each time a daemon starts. What it does depends on
// which daemon that is, and in both cases it keeps a configuration experiment from running without
// a supervisor.
//
// The stable daemon recovers an experiment left running unsupervised because the detached watcher
// process (config_experiment_watcher_darwin.go) that was tracking it died along with everything
// else in an abrupt shutdown -- a crash, a kill, a reboot -- before it could revert or hand off.
// The deadline file is the only durable signal of this, since the watcher is a bare process with
// no launchd job and nothing else re-spawns it: a present, unexpired deadline means an experiment
// was mid-flight with nothing currently watching it, so the experiment job set is re-established
// here, and the installer daemon handed over to the experiment, whose daemon launches the watcher.
// A present, expired deadline means the unsupervised window was already exceeded before the daemon
// got a chance to resume it, so the experiment is reverted instead. No deadline at all, but an
// experiment configuration directory still deployed, is reverted too (see
// revertOrphanedExperiment).
//
// The stable daemon drives the agent jobs only: it must not swap itself (see installerJob). The
// hand-over process it launches does, from outside its job.
//
// The experiment daemon launches the watcher (superviseFromExperimentDaemon). It does not resume
// or revert anything: starting is the experiment running as intended, and resuming would bounce
// the experiment the watcher is about to supervise.
func resumeConfigExperimentDatadogAgent(ctx HookContext) error {
	if runByExperimentDaemon() {
		return superviseFromExperimentDaemon(ctx)
	}
	deadline := experimentDeadline()
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
		_, err := revertFromResume(ctx, deadline, token, "experiment deadline expired before the daemon could resume it")
		return err
	}

	// The installer publishes the experiment configuration before the experiment starts, and only
	// promotes or discards it once a stop or promote is under way. A deadline still in force over a
	// configuration that is no longer deployed is a promote whose hook never got to hand the jobs
	// back -- the previous daemon died in between. Resuming would run the experiment job set on the
	// stable configuration until the deadline; finish the hand-back instead.
	if state, err := configExperimentDirs.GetState(); err != nil {
		log.Warnf("resume: could not read the configuration state, resuming anyway: %v", err)
	} else if state.ExperimentDeploymentID == "" {
		log.Warnf("resume: the experiment configuration is no longer deployed, handing the jobs back to the stable set")
		_, err := revertFromResume(ctx, deadline, token, "experiment configuration no longer deployed")
		return err
	}

	log.Infof("resume: configuration experiment still within its deadline, resuming supervision")
	// A daemon restart does not take the previous watcher with it: it is a detached process. It is
	// still armed on the pids Resume is about to bounce, so it would read their termination as a
	// crash and revert the experiment under the resume. Stop it first; the experiment daemon
	// launches a new one once the hand-over below has started it.
	if err := stopSurvivingWatchers(ctx); err != nil {
		log.Warnf("resume: could not stop the previous watcher, resuming anyway: %v", err)
	}
	if err := (configExperiment{jobs: agentJobSet()}).Resume(ctx); err != nil {
		log.Errorf("resume: could not resume the configuration experiment job set, reverting: %v", err)
		if _, revertErr := revertFromResume(ctx, deadline, token, "experiment could not be resumed"); revertErr != nil {
			return fmt.Errorf("resume failed (%w) and the experiment could not be reverted: %w", err, revertErr)
		}
		return fmt.Errorf("resume failed, experiment reverted: %w", err)
	}
	if err := launchInstallerHandOver(ctx); err != nil {
		log.Errorf("resume: could not relaunch the installer daemon hand-over, reverting: %v", err)
		if revertErr := (configExperiment{jobs: agentJobSet()}).Stop(context.WithoutCancel(ctx)); revertErr != nil {
			return fmt.Errorf("hand-over failed to relaunch (%w) and the experiment could not be reverted: %w", err, revertErr)
		}
		return fmt.Errorf("hand-over failed to relaunch, experiment reverted: %w", err)
	}
	return nil
}

// superviseFromExperimentDaemon is the resume hook as the experiment daemon runs it, each time it
// starts: it launches the watcher that supervises the experiment, so the watcher is the code the
// experiment runs under (see installerJob). The hand-over process that started this daemon waits
// for that watcher, and reverts the experiment if none appears.
//
// With no experiment pending the daemon has nothing to run under -- the experiment ended while it
// started -- so it has itself handed back. A deadline it cannot read does not stop the watcher from
// being launched: the watcher then reverts the experiment, as it does for any experiment nobody can
// tell the state of.
//
// A previous watcher is stopped first, so a restarted experiment daemon never leaves two. A watcher
// that cannot be launched fails the experiment: the agent jobs are reverted from here, and the
// daemon handed back, since a hook cannot swap the daemon it runs in.
func superviseFromExperimentDaemon(ctx HookContext) error {
	token, err := experimentDeadline().Read()
	switch {
	case err != nil:
		log.Warnf("resume: could not read experiment deadline, leaving the watcher to revert: %v", err)
	case !experimentPending(token):
		log.Infof("resume: no configuration experiment is pending, handing the installer daemon back")
		return handBackInstallerAfterHook(ctx)
	}
	if err := stopSurvivingWatchers(ctx); err != nil {
		log.Warnf("resume: could not stop the previous watcher, launching a new one anyway: %v", err)
	}
	if err := launchConfigExperimentWatcher(ctx); err != nil {
		log.Errorf("resume: could not launch the configuration experiment watcher, reverting: %v", err)
		revertErr := revertExperiment(ctx, agentJobSet())
		handBackErr := handBackInstallerAfterHook(ctx)
		if joined := errors.Join(revertErr, handBackErr); joined != nil {
			return fmt.Errorf("watcher failed to launch (%w) and the experiment could not be reverted: %w", err, joined)
		}
		return fmt.Errorf("watcher failed to launch, experiment reverted: %w", err)
	}
	return nil
}

// revertOrphanedExperiment reverts an experiment configuration directory that is deployed with no
// deadline beside it. Start writes the deadline before touching any job, and restoreStable only
// clears it once the stable set is back, so this is a host whose previous daemon died either after
// the installer published the directory but before the experiment job set ever started, or
// between a deliberate stop's job hand-back and its discarding of the directory. Either way nothing will ever resume the experiment, yet the directory alone
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
	if err := revertExperiment(ctx, agentJobSet()); err != nil {
		return fmt.Errorf("resume: %w", err)
	}
	removeInstallerExperimentJob()
	return nil
}

// revertFromResume is revertExperimentIfStillPending for the resume hook: it reverts the agent
// jobs, and removes a leftover definition of the experiment daemon's job.
func revertFromResume(ctx context.Context, deadline launchd.Deadline, token string, reason string) (bool, error) {
	reverted, err := revertExperimentIfStillPending(ctx, agentJobSet(), deadline, token, reason)
	if reverted {
		removeInstallerExperimentJob()
	}
	return reverted, err
}

// removeInstallerExperimentJob removes the experiment daemon's job definition, left on disk when
// the host rebooted mid-experiment. The stable daemon is running, so that job is not: it only has
// to go so the host looks as if no experiment had ever run. Failing to remove it is logged only:
// with no RunAtLoad, a leftover definition never starts on its own.
func removeInstallerExperimentJob() {
	if err := installerJobSet().Remove(launchd.Experiment); err != nil {
		log.Warnf("resume: could not remove the experiment daemon's job definition: %v", err)
	}
}

// watcherStopTimeout bounds how long stopSurvivingWatchers waits for a watcher to exit after
// SIGTERM before it is killed.
const watcherStopTimeout = 5 * time.Second

// listWatcherPIDs returns the pids of the experiment watcher processes running, other than this
// one. The watchers are found by listing processes, and only root-owned processes running exactly
// the watcher command are matched (parseWatcherPIDs).
func listWatcherPIDs(ctx context.Context) ([]int, error) {
	out, err := exec.CommandContext(ctx, "/bin/ps", "-ax", "-o", "pid=,uid=,command=").Output()
	if err != nil {
		return nil, fmt.Errorf("could not list processes: %w", err)
	}
	return parseWatcherPIDs(string(out), os.Getpid()), nil
}

// watcherRunning reports whether an experiment watcher process is running. A var so tests can stub
// it.
var watcherRunning = func(ctx context.Context) (bool, error) {
	pids, err := listWatcherPIDs(ctx)
	return len(pids) > 0, err
}

// stopSurvivingWatchers terminates the watcher processes a previous run of the daemon left
// behind, and waits for them to exit. A var so tests can observe when resume calls it.
var stopSurvivingWatchers = func(ctx context.Context) error {
	pids, err := listWatcherPIDs(ctx)
	if err != nil {
		return err
	}
	for _, pid := range pids {
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
