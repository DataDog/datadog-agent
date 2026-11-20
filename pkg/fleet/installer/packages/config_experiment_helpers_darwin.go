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

// stopSurvivingWatchers terminates the watcher processes an earlier experiment left behind, and
// waits for them to exit. The hand-over calls it before it launches the watcher of the experiment
// it has just handed the installer daemon over to, so there is never more than one. Only watchers
// are stopped: the hand-over is itself one of the other helpers. A var so tests can observe when it
// is called.
var stopSurvivingWatchers = func(ctx context.Context) error {
	return stopHelpers(ctx, watchConfigExperimentCommand)
}

// stopSurvivingHelpers terminates every detached experiment process a previous run left behind:
// the watcher, and any hand-over or hand-back still pending. The post-install calls it before it
// discards an experiment: a surviving watcher would read the jobs going away as a crash and revert
// on top of the install, and a surviving hand-over would swap the installer daemon under it. A var
// so tests can observe when it is called.
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
