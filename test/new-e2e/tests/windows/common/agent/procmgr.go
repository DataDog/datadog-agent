// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agent

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
)

// ProcmgrCLIPath returns the dd-procmgr CLI path for an Agent installed at installRoot.
func ProcmgrCLIPath(installRoot string) string {
	return filepath.Join(installRoot, "bin", "agent", "dd-procmgr.exe")
}

// ProcmgrProcessConfigPath returns the processes.d definition path for processName.
func ProcmgrProcessConfigPath(installRoot, processName string) string {
	return filepath.Join(installRoot, "processes.d", processName+".yaml")
}

// GetProcmgrProcessField runs `dd-procmgr describe <processName>` and returns the value of
// field, for example "State" or "PID".
//
// Workloads that dd-procmgr supervises have no SCM service to query, so this is the
// equivalent of GetServiceStatus for them.
func GetProcmgrProcessField(host *components.RemoteHost, cli, processName, field string) (string, error) {
	cmd := fmt.Sprintf(`& '%s' describe %s`, strings.ReplaceAll(cli, `'`, `''`), processName)
	out, err := host.Execute(cmd)
	if err != nil {
		return "", fmt.Errorf("dd-procmgr describe %s failed: %w, output: %s", processName, err, strings.TrimSpace(out))
	}
	for _, line := range strings.Split(out, "\n") {
		if value, ok := strings.CutPrefix(strings.TrimSpace(line), field+":"); ok {
			return strings.TrimSpace(value), nil
		}
	}
	return "", fmt.Errorf("no %s field in dd-procmgr describe %s output: %s", field, processName, strings.TrimSpace(out))
}

// GetProcmgrProcessState returns the dd-procmgrd-reported state of processName, for example
// "Running", "Stopped" or "Created".
func GetProcmgrProcessState(host *components.RemoteHost, installRoot, processName string) (string, error) {
	return GetProcmgrProcessField(host, ProcmgrCLIPath(installRoot), processName, "State")
}

// AssertProcmgrProcessRunning fails the test unless dd-procmgrd reports processName as
// stably Running.
//
// Stably matters: a workload that starts and exits shortly after would satisfy a plain
// "is it Running" check. system-probe does exactly that when it decides no module is
// enabled, sleeping 5 seconds before exiting 0. The window is several times that sleep so
// that neither poll jitter nor a lagging state update can land inside it.
func AssertProcmgrProcessRunning(t *testing.T, host *components.RemoteHost, processName string) {
	t.Helper()
	installRoot, err := GetInstallPathFromRegistry(host)
	require.NoError(t, err, "should find the Agent install path")

	var runningSince time.Time
	const minRunningDuration = 20 * time.Second
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		state, err := GetProcmgrProcessState(host, installRoot, processName)
		if !assert.NoError(c, err) ||
			!assert.Equal(c, "Running", state, "%s should be running under dd-procmgrd", processName) {
			runningSince = time.Time{}
			return
		}
		if runningSince.IsZero() {
			runningSince = time.Now()
		}
		// EventuallyWithT treats a tick with no recorded failures as an immediate success, so
		// the stability window has to be an assertion rather than a silent early return.
		assert.GreaterOrEqual(c, time.Since(runningSince), minRunningDuration,
			"%s has not been running long enough yet", processName)
	}, 3*time.Minute, 5*time.Second)
}
