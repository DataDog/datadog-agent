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
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/launchd"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// resumeConfigExperimentDatadogAgent is run once each time an installer daemon starts. What it does
// depends on which daemon that is.
//
// The experiment daemon launches the watcher that supervises the experiment it was handed over to
// (superviseFromExperimentDaemon).
//
// The stable daemon settles the host (settleConfigExperiment): it never brings an experiment back.
// A stable daemon starting with an experiment still unfinished means the previous one did not end
// it -- a reboot, a crash, a kill -- and the watcher that was supervising it, a detached process
// with no launchd job, is gone with it. Bringing the experiment back would need a new watcher, a new
// hand-over and a deadline that still means something; failing it puts the host in the state Fleet
// already handles for any experiment that fails on the host.
func resumeConfigExperimentDatadogAgent(ctx HookContext) error {
	if runByExperimentDaemon() {
		return superviseFromExperimentDaemon(ctx)
	}
	return settleConfigExperiment(ctx)
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
		revertErr := revertExperiment(ctx, configExperiment{jobs: agentJobSet()})
		handBackErr := handBackInstallerAfterHook(ctx)
		if joined := errors.Join(revertErr, handBackErr); joined != nil {
			return fmt.Errorf("watcher failed to launch (%w) and the experiment could not be reverted: %w", err, joined)
		}
		return fmt.Errorf("watcher failed to launch, experiment reverted: %w", err)
	}
	return nil
}

// settleConfigExperiment is the resume hook as the stable daemon runs it. It leaves the host on the
// stable job set, in one of the end states the rest of the experiment lifecycle already knows,
// whatever the previous daemon was doing when it went away:
//
//   - An experiment still pending or being reverted (a deadline, or the reverting marker), with its
//     configuration deployed, fails: the agent jobs go back to the stable set, the configuration is
//     kept and the deadline replaced by the failed marker (configExperiment.Fail), for Fleet to stop.
//   - The same with the configuration no longer deployed is a promote whose hook did not finish: it
//     is finished as a promote (revertExperiment).
//   - A failed experiment is left for Fleet to stop. A failed marker with no configuration behind it
//     is cleared.
//   - A configuration deployed with no deadline is discarded (revertOrphanedExperiment).
//
// After a reboot every stable job has just been started by launchd, so the hand-back leaves alone
// any job already on its stable variant (configExperiment.keepRunningStable) instead of restarting
// it. The stable daemon drives the agent jobs only: it must not swap itself (see installerJob).
// Whatever is left of the experiment daemon's job is dealt with last
// (settleInstallerExperimentJob).
func settleConfigExperiment(ctx HookContext) error {
	err := settleAgentJobs(ctx)
	settleInstallerExperimentJob(ctx)
	return err
}

// settleAgentJobs is settleConfigExperiment's decision, for the agent jobs and the deadline.
func settleAgentJobs(ctx HookContext) error {
	deadline := experimentDeadline()
	token, err := deadline.Read()
	switch {
	case err != nil:
		log.Warnf("settle: could not read the experiment deadline, failing any configuration experiment left behind: %v", err)
		return settleUnfinishedExperiment(ctx)
	case token == launchd.DeadlineFailed:
		if experimentConfigDeployed() {
			log.Infof("settle: the configuration experiment failed on the host and waits for Fleet to stop it")
			return nil
		}
		return deadline.Clear()
	case token == "":
		return revertOrphanedExperiment(ctx)
	default:
		return settleUnfinishedExperiment(ctx)
	}
}

// settleUnfinishedExperiment hands the agent jobs of an experiment the previous daemon left
// unfinished back to the stable set: failed, configuration kept, or finished as a promote when the
// configuration was already promoted (revertExperiment).
func settleUnfinishedExperiment(ctx HookContext) error {
	log.Warnf("settle: the previous installer daemon left a configuration experiment unfinished, handing every job back to the stable set")
	stopHelpersBeforeSettling(ctx)
	if err := revertExperiment(ctx, configExperiment{jobs: agentJobSet(), keepRunningStable: true}); err != nil {
		return fmt.Errorf("settle: %w", err)
	}
	return nil
}

// revertOrphanedExperiment reverts an experiment configuration directory that is deployed with no
// deadline beside it. Start writes the deadline before touching any job, and restoreStable only
// clears it once the stable set is back, so this is a host whose previous daemon died either after
// the installer published the directory but before the experiment job set ever started, or between
// a deliberate stop's job hand-back and its discarding of the directory. Either way nothing will
// ever end the experiment, yet the directory alone would have the daemon report it as the running
// configuration indefinitely.
func revertOrphanedExperiment(ctx HookContext) error {
	state, err := configExperimentDirs.GetState()
	if err != nil {
		return fmt.Errorf("settle: could not read the configuration state: %w", err)
	}
	if state.ExperimentDeploymentID == "" {
		return nil
	}
	log.Warnf("settle: configuration experiment %s is deployed with no deadline, reverting", state.ExperimentDeploymentID)
	stopHelpersBeforeSettling(ctx)
	if err := revertAndDiscardExperiment(ctx, configExperiment{jobs: agentJobSet(), keepRunningStable: true}); err != nil {
		return fmt.Errorf("settle: %w", err)
	}
	return nil
}

// stopHelpersBeforeSettling stops the detached experiment processes a previous daemon left behind,
// before the settle touches any job: a surviving watcher would read the jobs it was armed on going
// away as a crash and revert on top of the settle, and a surviving hand-over would swap the
// installer daemon under it. Failing to stop them does not stop the settle: the host must end up on
// the stable set either way.
func stopHelpersBeforeSettling(ctx context.Context) {
	if err := stopSurvivingHelpers(ctx); err != nil {
		log.Warnf("settle: could not stop the previous configuration experiment processes, settling anyway: %v", err)
	}
}

// settleInstallerExperimentJob takes the experiment daemon's job out of the stable daemon's way.
//
// After a reboot mid-experiment its definition is still on disk, and launchd loads it at boot
// without starting it (no RunAtLoad): it is unloaded and removed, so the host looks as if no
// experiment had ever run. An experiment daemon actually running beside this one -- started by
// hand -- is handed back by the hand-back process instead, as a stop or promote does: a hook cannot
// swap the daemon it runs in. Failures are logged only: a leftover definition never starts on its
// own.
func settleInstallerExperimentJob(ctx context.Context) {
	jobs := installerJobSet()
	if _, err := os.Stat(jobs.Job(installerJob, launchd.Experiment).Path()); errors.Is(err, os.ErrNotExist) {
		return
	}
	label := installerJob + string(launchd.Experiment)
	status, err := jobs.Client.Print(ctx, label)
	switch {
	case err != nil:
		log.Warnf("settle: could not look at the experiment daemon's job: %v", err)
	case status.PID != 0:
		log.Warnf("settle: the experiment daemon is running beside the stable one, handing it back")
		if err := launchInstallerHandBack(ctx); err != nil {
			log.Errorf("settle: could not launch the installer daemon hand-back: %v", err)
		}
		return
	case status.Loaded:
		if err := jobs.Client.Bootout(ctx, label); err != nil {
			log.Warnf("settle: could not unload the experiment daemon's job: %v", err)
		}
	}
	removeInstallerExperimentJob()
}

// removeInstallerExperimentJob removes the experiment daemon's job definition. Failing to remove it
// is logged only: with no RunAtLoad, a leftover definition never starts on its own.
func removeInstallerExperimentJob() {
	if err := installerJobSet().Remove(launchd.Experiment); err != nil {
		log.Warnf("could not remove the experiment daemon's job definition: %v", err)
	}
}

// helperStopTimeout bounds how long stopHelpers waits for a helper process to exit after SIGTERM
// before it is killed.
const helperStopTimeout = 5 * time.Second

// experimentHelperCommands are the package commands of every detached process an experiment
// launches: the watcher, and the processes that hand the installer daemon over and back.
var experimentHelperCommands = []string{watchConfigExperimentCommand, handOverInstallerCommand, handBackInstallerCommand}

// listHelperPIDs returns the pids of the detached experiment processes running any of commands,
// other than this one. They are found by listing processes, and only root-owned processes running
// exactly one of those commands are matched (parseHelperPIDs).
func listHelperPIDs(ctx context.Context, commands ...string) ([]int, error) {
	out, err := exec.CommandContext(ctx, "/bin/ps", "-ax", "-o", "pid=,uid=,command=").Output()
	if err != nil {
		return nil, fmt.Errorf("could not list processes: %w", err)
	}
	return parseHelperPIDs(string(out), os.Getpid(), commands...), nil
}

// watcherRunning reports whether an experiment watcher process is running. A var so tests can stub
// it.
var watcherRunning = func(ctx context.Context) (bool, error) {
	pids, err := listHelperPIDs(ctx, watchConfigExperimentCommand)
	return len(pids) > 0, err
}

// stopSurvivingWatchers terminates the watcher processes a previous run of the daemon left
// behind, and waits for them to exit. The experiment daemon calls it: the hand-over that started
// it is still waiting for the watcher it launches, so only watchers are stopped. A var so tests
// can observe when it is called.
var stopSurvivingWatchers = func(ctx context.Context) error {
	return stopHelpers(ctx, watchConfigExperimentCommand)
}

// stopSurvivingHelpers terminates every detached experiment process a previous run left behind:
// the watcher, and any hand-over or hand-back still pending. The stable daemon calls it before it
// settles the host, and the post-install before it discards an experiment: a surviving watcher
// would read the jobs it was armed on going away as a crash and revert on top of them, and a
// surviving hand-over would swap the installer daemon under them. A var so tests can observe when
// it is called.
var stopSurvivingHelpers = func(ctx context.Context) error {
	return stopHelpers(ctx, experimentHelperCommands...)
}

// stopHelpers terminates the detached experiment processes running any of commands, and waits for
// them to exit, killing those that ignore SIGTERM.
func stopHelpers(ctx context.Context, commands ...string) error {
	pids, err := listHelperPIDs(ctx, commands...)
	if err != nil {
		return err
	}
	for _, pid := range pids {
		log.Infof("stopping a previous configuration experiment process (pid %d)", pid)
		_ = syscall.Kill(pid, syscall.SIGTERM)
		stopped := time.Now().Add(helperStopTimeout)
		for time.Now().Before(stopped) && syscall.Kill(pid, 0) == nil {
			time.Sleep(100 * time.Millisecond)
		}
		if syscall.Kill(pid, 0) == nil {
			log.Warnf("the previous configuration experiment process (pid %d) ignored SIGTERM, killing it", pid)
			_ = syscall.Kill(pid, syscall.SIGKILL)
		}
	}
	return nil
}

// parseHelperPIDs returns the pids of the root-owned detached experiment processes running any of
// commands in the output of `ps -ax -o pid=,uid=,command=`, other than self.
//
// The command line must be exactly the installer binary followed by the three arguments the
// launchers pass it, so another process whose command line merely contains that text -- a shell
// or pgrep looking for the watcher -- is never matched.
func parseHelperPIDs(ps string, self int, commands ...string) []int {
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
		if fields[3] != "package-command" || fields[4] != agentPackage || !slices.Contains(commands, fields[5]) {
			continue
		}
		pids = append(pids, pid)
	}
	return pids
}
