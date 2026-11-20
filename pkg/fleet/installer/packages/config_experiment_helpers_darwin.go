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
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// helperStopTimeout bounds how long stopHelpers waits for a helper process to exit after SIGTERM
// before it is killed.
const helperStopTimeout = 5 * time.Second

// experimentHelperCommands are the package commands of every detached process an experiment
// launches: the watchers, and the fallback hand-back.
var experimentHelperCommands = []string{watchConfigExperimentCommand, handBackInstallerCommand}

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

// stopSurvivingWatchers terminates the watcher processes an earlier experiment left behind, other
// than this one, and waits for them to exit. W1 calls it as it starts, so a watcher still around
// from an earlier experiment never acts on this experiment. A var so tests can observe when it is
// called.
var stopSurvivingWatchers = func(ctx context.Context) error {
	return stopHelpers(ctx, watchConfigExperimentCommand)
}

// watcherStopTimeout bounds how long W2 waits for the other watchers to exit after SIGTERM before it
// kills them: longer than the grace a watcher keeps supervising for after SIGTERM, so that a watcher
// in the middle of a revert or a hand-back finishes it. A var so tests can shorten it.
var watcherStopTimeout = 15 * time.Second

// stopOtherWatchers terminates every watcher process other than this one, and waits for them to
// exit. W2 calls it once it is armed on every experiment job: W1, and any watcher an earlier
// experiment left behind, stop supervising. A var so tests can observe when it is called.
var stopOtherWatchers = func(ctx context.Context) error {
	return stopHelpersWithin(ctx, watcherStopTimeout, watchConfigExperimentCommand)
}

// watcherRunning reports whether a watcher process other than this one runs. A watcher asks it to
// tell whether the other one owns a failure; a stop or promote hook asks it to tell whether a
// watcher will hand the installer daemon back. A var so tests can stub it.
var watcherRunning = func(ctx context.Context) (bool, error) {
	pids, err := listHelperPIDs(ctx, watchConfigExperimentCommand)
	return len(pids) > 0, err
}

// stopSurvivingHelpers terminates every detached experiment process a previous run left behind: the
// watchers, and any hand-back still pending. The post-install calls it before it discards an
// experiment: a surviving watcher would read the jobs going away as a crash and revert on top of the
// install, or swap the installer daemon under it. A var so tests can observe when it is called.
var stopSurvivingHelpers = func(ctx context.Context) error {
	return stopHelpers(ctx, experimentHelperCommands...)
}

// stopHelpers terminates the detached experiment processes running any of commands, and waits for
// them to exit, killing those that ignore SIGTERM.
func stopHelpers(ctx context.Context, commands ...string) error {
	return stopHelpersWithin(ctx, helperStopTimeout, commands...)
}

// stopHelpersWithin is stopHelpers with timeout as the wait after SIGTERM.
func stopHelpersWithin(ctx context.Context, timeout time.Duration, commands ...string) error {
	pids, err := listHelperPIDs(ctx, commands...)
	if err != nil {
		return err
	}
	for _, pid := range pids {
		log.Infof("stopping a previous configuration experiment process (pid %d)", pid)
		_ = syscall.Kill(pid, syscall.SIGTERM)
		stopped := time.Now().Add(timeout)
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
