// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !e2eunit

package installer

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
	windowsagent "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/agent"
)

const (
	// ddotProcmgrConfigName is the processes.d config filename dd-procmgrd uses for DDOT.
	ddotProcmgrConfigName = "datadog-agent-ddot.yaml"
	// ddotProcmgrProcessName is the process name dd-procmgrd uses for DDOT.
	ddotProcmgrProcessName = "datadog-agent-ddot"
	// legacyDDOTWindowsSCMService is the Windows SCM service for otel when not using dd-procmgr.
	legacyDDOTWindowsSCMService = "datadog-otel-agent"
)

// AssertDDOTManagedByProcmgrWindows verifies the OCI DDOT extension process is supervised
// by dd-procmgrd on a Windows host (processes.d + dd-procmgr describe), not only that
// dd-procmgr-service is running.
func AssertDDOTManagedByProcmgrWindows(t *testing.T, host *components.RemoteHost) {
	t.Helper()

	installRoot, err := windowsagent.GetInstallPathFromRegistry(host)
	require.NoError(t, err)

	cli := filepath.Join(installRoot, "bin", "agent", "dd-procmgr.exe")
	cfg := filepath.Join(installRoot, "processes.d", ddotProcmgrConfigName)

	requireRemoteLiteralPath(t, host, cli, "dd-procmgr CLI")
	requireRemoteLiteralPath(t, host, cfg, "DDOT procmgr config")

	waitForProcmgrCLIWindows(t, host, cli)
	waitProcmgrDDOTDescribeRunningStable(t, host, psProcmgr(cli, "describe "+ddotProcmgrProcessName))
}

// AssertNoFleetDDOTProcmgrConfigFileWindows asserts the fleet DDOT extension did not write
// processes.d/datadog-agent-ddot.yaml (e.g. when DD_PROCESS_MANAGER_ENABLED=false during install hooks).
func AssertNoFleetDDOTProcmgrConfigFileWindows(t *testing.T, host *components.RemoteHost) {
	t.Helper()
	installRoot, err := windowsagent.GetInstallPathFromRegistry(host)
	require.NoError(t, err)
	cfg := filepath.Join(installRoot, "processes.d", ddotProcmgrConfigName)
	// Normalize slashes for PowerShell -LiteralPath (tests may build paths on non-Windows).
	cfg = strings.ReplaceAll(cfg, "/", `\`)
	_, err = host.Execute(psLiteralPathNotExists(cfg))
	require.NoError(t, err, "fleet DDOT processes.d config should be absent when installer procmgr wiring is disabled (expected missing: %s)", cfg)
}

// AssertWindowsDDOTRunningLegacySCM waits until datadog-otel-agent is Running (DDOT on the legacy
// Windows SCM path rather than fleet processes.d under dd-procmgr).
func AssertWindowsDDOTRunningLegacySCM(t *testing.T, host *components.RemoteHost) {
	t.Helper()
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		_, err := host.Execute(
			`powershell -NoProfile -Command "$s = Get-Service -Name '` + legacyDDOTWindowsSCMService + `' -ErrorAction SilentlyContinue; if ($null -eq $s) { exit 1 }; if ($s.Status -ne 'Running') { exit 1 }; exit 0"`)
		assert.NoError(c, err, "%s should be Running when DDOT is not wired via fleet processes.d", legacyDDOTWindowsSCMService)
	}, 3*time.Minute, 3*time.Second)
}

// WindowsDescribeDDOTCommandLine runs dd-procmgr describe and returns the Command field from text output.
func WindowsDescribeDDOTCommandLine(host *components.RemoteHost, ddProcmgrCLI string) (string, error) {
	out, err := host.Execute(psProcmgr(ddProcmgrCLI, "describe "+ddotProcmgrProcessName))
	if err != nil {
		return "", err
	}
	out = strings.ReplaceAll(out, "\r", "")
	for _, line := range strings.Split(out, "\n") {
		trimmed := strings.TrimSpace(line)
		if after, ok := strings.CutPrefix(trimmed, "Command:"); ok {
			return strings.TrimSpace(after), nil
		}
	}
	return "", nil
}

func requireRemoteLiteralPath(t *testing.T, host *components.RemoteHost, path, description string) {
	t.Helper()
	_, err := host.Execute(psLiteralPathExists(path))
	require.NoError(t, err, "%s should exist at %s", description, path)
}

func psLiteralPathExists(path string) string {
	return fmt.Sprintf(
		`powershell -NoProfile -Command "if (-not (Test-Path -LiteralPath '%s')) { exit 1 }"`,
		psEscapeSingleQuoted(path),
	)
}

func psLiteralPathNotExists(path string) string {
	return fmt.Sprintf(
		`powershell -NoProfile -Command "if (Test-Path -LiteralPath '%s') { exit 1 }"`,
		psEscapeSingleQuoted(path),
	)
}

// psProcmgr runs a dd-procmgr subcommand (e.g. "status", "describe datadog-agent-ddot").
func psProcmgr(cliExe, invocation string) string {
	return fmt.Sprintf(
		`powershell -NoProfile -Command "$ErrorActionPreference='Stop'; & '%s' %s"`,
		psEscapeSingleQuoted(cliExe),
		invocation,
	)
}

func psEscapeSingleQuoted(s string) string {
	return strings.ReplaceAll(s, `'`, `''`)
}

func waitForProcmgrCLIWindows(t *testing.T, host *components.RemoteHost, cli string) {
	t.Helper()
	cmd := psProcmgr(cli, "status")
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		_, err := host.Execute(cmd)
		assert.NoError(c, err, "dd-procmgr CLI not reachable")
	}, 2*time.Minute, 2*time.Second)
}

// waitProcmgrDDOTDescribeRunningStable waits until dd-procmgr describe reports DDOT stably Running.
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
		if !assert.Equal(c, "Running", state,
			"DDOT should be running under dd-procmgrd; describe output:\n%s", strings.TrimSpace(out)) {
			runningSince = time.Time{}
			return
		}
		if runningSince.IsZero() {
			runningSince = time.Now()
		}
		// EventuallyWithT treats a tick with no recorded failures as an immediate success, so the
		// stability window must be enforced via an assertion rather than a silent early return.
		assert.GreaterOrEqual(c, time.Since(runningSince), minRunningDuration,
			"DDOT has not been running long enough yet")
	}, 2*time.Minute, 5*time.Second)
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
