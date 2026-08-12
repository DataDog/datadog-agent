// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package ddot provides shared E2E helpers for DDOT extension tests.
package ddot

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	e2eos "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
)

const (
	fleetAgentStableDir     = "/opt/datadog-packages/datadog-agent/stable"
	fleetAgentExperimentDir = "/opt/datadog-packages/datadog-agent/experiment"
	debAgentDir             = "/opt/datadog-agent"
	procmgrProcessName      = "datadog-agent-ddot"
	procmgrConfigName       = "datadog-agent-ddot.yaml"
	procmgrSocket           = "/var/run/datadog-procmgrd/dd-procmgrd.sock"
	ddotSystemdUnit         = "datadog-agent-ddot.service"
	ddotSystemdUnitExp      = "datadog-agent-ddot-exp.service"
)

// AssertDDOTSystemdUnitsNotActive fails if datadog-agent-ddot systemd units are active.
// DDOT must run under dd-procmgrd, not the legacy systemd units.
func AssertDDOTSystemdUnitsNotActive(t *testing.T, host *components.RemoteHost) {
	t.Helper()
	assertSystemdUnitsNotActive(t, host)
}

// AssertDDOTManagedByProcmgr verifies extension DDOT is supervised by dd-procmgrd.
// Call AssertDDOTSystemdUnitsNotActive first so legacy datadog-agent-ddot systemd units are not active.
func AssertDDOTManagedByProcmgr(t *testing.T, host *components.RemoteHost) {
	t.Helper()

	installRoot := resolveAgentInstallRoot(host)
	_, err := host.Execute("test -x " + procmgrCLIBin(installRoot))
	require.NoError(t, err, "dd-procmgr CLI should exist at %s", procmgrCLIBin(installRoot))

	_, err = host.Execute("test -f " + procmgrConfigPath(installRoot))
	require.NoError(t, err, "procmgr config should exist at %s", procmgrConfigPath(installRoot))

	assertManagedByProcmgr(t, host, installRoot)
}

// AssertDDOTNotManagedByProcmgr verifies extension DDOT is no longer supervised by dd-procmgrd.
func AssertDDOTNotManagedByProcmgr(t *testing.T, host *components.RemoteHost) {
	t.Helper()
	assertNotManagedByProcmgr(t, host, resolveAgentInstallRoot(host))
}

// AssertDDOTAutoInstallUnderProcmgr verifies DDOT after an env-driven install is not on the legacy
// supervisor path and is stable under dd-procmgr (Windows: SCM + describe/binary; Linux: systemd + describe).
func AssertDDOTAutoInstallUnderProcmgr(t *testing.T, host *components.RemoteHost) {
	t.Helper()
	switch host.OSFamily {
	case e2eos.WindowsFamily:
		AssertWindowsDDOTSCMServiceNotRunningWhenProcmgr(t, host)
		AssertWindowsProcmgrDDOTMatchesInstalledBinary(t, host)
	case e2eos.LinuxFamily:
		AssertDDOTSystemdUnitsNotActive(t, host)
		AssertDDOTManagedByProcmgr(t, host)
	default:
		t.Skipf("DDOT procmgr post-install checks are not defined for OS family %v", host.OSFamily)
	}
}

func procmgrCLIBin(installRoot string) string {
	return installRoot + "/embedded/bin/dd-procmgr"
}

func procmgrConfigPath(installRoot string) string {
	return installRoot + "/processes.d/" + procmgrConfigName
}

func procmgrCLICmd(installRoot, args string) string {
	return "sudo -u dd-agent " + procmgrCLIBin(installRoot) + " " + args
}

func resolveAgentInstallRoot(host *components.RemoteHost) string {
	candidates := []string{fleetAgentStableDir, debAgentDir}
	out, err := host.Execute("systemctl is-active datadog-agent-exp.service 2>/dev/null || true")
	if err == nil && strings.TrimSpace(out) == "active" {
		candidates = append([]string{fleetAgentExperimentDir}, candidates...)
	}
	for _, root := range candidates {
		if _, err := host.Execute("test -x " + procmgrCLIBin(root)); err == nil {
			return root
		}
	}
	return debAgentDir
}

func waitForProcmgrCLI(t *testing.T, host *components.RemoteHost, installRoot string) {
	t.Helper()
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		_, err := host.Execute(procmgrCLICmd(installRoot, "status"))
		assert.NoError(c, err, "procmgr not reachable via %s", procmgrSocket)
	}, 2*time.Minute, 2*time.Second)
}

func waitProcmgrDDOTDescribeRunningStable(t *testing.T, host *components.RemoteHost, describeCmd string) {
	t.Helper()
	var runningSince time.Time
	const minRunningDuration = 5 * time.Second
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		out, err := host.Execute(describeCmd)
		if err != nil {
			assert.Failf(c, "dd-procmgr describe failed", "err: %v\noutput:\n%s",
				err, strings.TrimSpace(out))
			return
		}
		state := procmgrFieldValue(out, "State")
		if state != "Running" {
			runningSince = time.Time{}
			assert.Equal(c, "Running", state,
				"DDOT should be running under dd-procmgrd; describe output:\n%s", strings.TrimSpace(out))
			return
		}
		if runningSince.IsZero() {
			runningSince = time.Now()
			return
		}
		if time.Since(runningSince) < minRunningDuration {
			return
		}
	}, 2*time.Minute, 5*time.Second)
}

func assertManagedByProcmgr(t *testing.T, host *components.RemoteHost, installRoot string) {
	t.Helper()
	waitForProcmgrCLI(t, host, installRoot)
	describeCmd := procmgrCLICmd(installRoot, "describe "+procmgrProcessName)
	waitProcmgrDDOTDescribeRunningStable(t, host, describeCmd)
}

func assertNotManagedByProcmgr(t *testing.T, host *components.RemoteHost, installRoot string) {
	t.Helper()

	_, err := host.Execute("test ! -f " + procmgrConfigPath(installRoot))
	require.NoError(t, err, "procmgr config should be removed at %s", procmgrConfigPath(installRoot))

	waitForProcmgrCLI(t, host, installRoot)

	describeCmd := procmgrCLICmd(installRoot, "describe "+procmgrProcessName)
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		_, err := host.Execute(describeCmd)
		assert.Error(c, err, "dd-procmgr describe should fail when DDOT is not managed")
	}, 2*time.Minute, 5*time.Second)
}

func assertSystemdUnitsNotActive(t *testing.T, host *components.RemoteHost) {
	t.Helper()
	for _, unit := range []string{ddotSystemdUnit, ddotSystemdUnitExp} {
		out, err := host.Execute("systemctl is-active " + unit + " 2>/dev/null || true")
		require.NoError(t, err)
		assert.NotEqual(t, "active", strings.TrimSpace(out),
			"%s should not be active when DDOT is managed by procmgr", unit)
	}
}

func procmgrFieldValue(output, label string) string {
	needle := label + ":"
	for _, line := range strings.Split(output, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, needle) {
			return strings.TrimSpace(trimmed[len(needle):])
		}
	}
	return ""
}
