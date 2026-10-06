// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agent

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
	windowsCommon "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common"
)

// ProcmgrCLIPath returns the dd-procmgr CLI path for an Agent installed at installRoot.
func ProcmgrCLIPath(installRoot string) string {
	return filepath.Join(installRoot, "bin", "agent", "dd-procmgr.exe")
}

// ProcmgrProcessConfigPath returns the processes.d definition path for processName.
func ProcmgrProcessConfigPath(installRoot, processName string) string {
	return filepath.Join(installRoot, "processes.d", processName+".yaml")
}

// GetProcmgrProcessState returns the dd-procmgrd-reported state of processName, for example
// "Running", "Stopped" or "Created".
//
// Workloads that dd-procmgr supervises have no SCM service to query, so this is the
// equivalent of GetServiceStatus for them.
func GetProcmgrProcessState(host *components.RemoteHost, installRoot, processName string) (string, error) {
	return windowsCommon.ProcmgrDescribeField(host, ProcmgrCLIPath(installRoot), processName, "State")
}

// AssertProcmgrProcessRunning fails the test unless dd-procmgrd reports processName as stably
// Running as one unchanging OS process.
//
// Both halves are needed. A workload that starts and exits shortly after would satisfy a plain
// "is it Running" check, so the state has to hold for a window several times longer than the 5
// second sleep system-probe takes before exiting when it decides no module is enabled.
//
// The PID covers what the window alone cannot. A process crashing more slowly than its restart
// burst limit is respawned indefinitely rather than left in Failed, and reads as Running on
// nearly every poll, with a different PID each time. Restarting resets the window instead of
// failing outright, so a single restart while an install settles still passes.
func AssertProcmgrProcessRunning(t *testing.T, host *components.RemoteHost, processName string) {
	t.Helper()
	installRoot, err := GetInstallPathFromRegistry(host)
	require.NoError(t, err, "should find the Agent install path")
	cliPath := ProcmgrCLIPath(installRoot)

	var (
		runningSince time.Time
		pid          string
	)
	const minRunningDuration = 20 * time.Second
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		fields, out, err := windowsCommon.ProcmgrDescribe(host, cliPath, processName)
		if !assert.NoError(c, err) ||
			!assert.Equal(c, "Running", fields["State"], "%s should be running under dd-procmgrd: %s", processName, out) {
			runningSince, pid = time.Time{}, ""
			return
		}
		current := fields["PID"]
		if !assert.NotEmpty(c, current, "%s is Running but reports no PID: %s", processName, out) ||
			!assert.NotEqual(c, "-", current, "%s is Running but reports no PID: %s", processName, out) {
			runningSince, pid = time.Time{}, ""
			return
		}
		if current != pid {
			runningSince, pid = time.Now(), current
		}
		// EventuallyWithT treats a tick with no recorded failures as an immediate success, so
		// the stability window has to be an assertion rather than a silent early return.
		assert.GreaterOrEqual(c, time.Since(runningSince), minRunningDuration,
			"%s has not been running as PID %s long enough yet", processName, current)
	}, 3*time.Minute, 5*time.Second)
}
