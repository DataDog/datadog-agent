// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ddot

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
	windowsagent "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/agent"
)

// WindowsLegacyDDOTSCMServiceName is the Windows SCM service for otel when not using dd-procmgr.
const WindowsLegacyDDOTSCMServiceName = "datadog-otel-agent"

// AssertWindowsDDOTSCMServiceNotRunningWhenProcmgr fails if the legacy Windows SCM otel service is
// Running while DDOT is expected under dd-procmgr.
func AssertWindowsDDOTSCMServiceNotRunningWhenProcmgr(t *testing.T, host *components.RemoteHost) {
	t.Helper()
	_, err := host.Execute(
		`powershell -NoProfile -Command "$s = Get-Service -Name '` + WindowsLegacyDDOTSCMServiceName + `' -ErrorAction SilentlyContinue; if ($null -eq $s) { exit 0 }; if ($s.Status -eq 'Running') { exit 1 }; exit 0"`)
	require.NoError(t, err, "%s Windows service must not be Running when DDOT is managed by dd-procmgr", WindowsLegacyDDOTSCMServiceName)
}

// AssertDDOTManagedByProcmgrWindows verifies the OCI DDOT extension process is supervised
// by dd-procmgrd on a Windows host (processes.d + dd-procmgr describe), not only that
// dd-procmgr-service is running.
func AssertDDOTManagedByProcmgrWindows(t *testing.T, host *components.RemoteHost) {
	t.Helper()

	installRoot, err := windowsagent.GetInstallPathFromRegistry(host)
	require.NoError(t, err)

	cli := filepath.Join(installRoot, "bin", "agent", "dd-procmgr.exe")
	cfg := filepath.Join(installRoot, "processes.d", procmgrConfigName)

	requireRemoteLiteralPath(t, host, cli, "dd-procmgr CLI")
	requireRemoteLiteralPath(t, host, cfg, "DDOT procmgr config")

	waitForProcmgrCLIWindows(t, host, cli)
	waitProcmgrDDOTDescribeRunningStable(t, host, psProcmgr(cli, "describe "+procmgrProcessName))
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

// AssertNoFleetDDOTProcmgrConfigFileWindows asserts the fleet DDOT extension did not write
// processes.d/datadog-agent-ddot.yaml (e.g. when DD_PROCESS_MANAGER_ENABLED=false during install hooks).
func AssertNoFleetDDOTProcmgrConfigFileWindows(t *testing.T, host *components.RemoteHost) {
	t.Helper()
	installRoot, err := windowsagent.GetInstallPathFromRegistry(host)
	require.NoError(t, err)
	cfg := filepath.Join(installRoot, "processes.d", procmgrConfigName)
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
			`powershell -NoProfile -Command "$s = Get-Service -Name '` + WindowsLegacyDDOTSCMServiceName + `' -ErrorAction SilentlyContinue; if ($null -eq $s) { exit 1 }; if ($s.Status -ne 'Running') { exit 1 }; exit 0"`)
		assert.NoError(c, err, "%s should be Running when DDOT is not wired via fleet processes.d", WindowsLegacyDDOTSCMServiceName)
	}, 3*time.Minute, 3*time.Second)
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

// normalizeWinPathForCompare lowercases and normalizes slashes for stable string compares of remote Windows paths.
func normalizeWinPathForCompare(p string) string {
	p = strings.TrimSpace(strings.ReplaceAll(p, `/`, `\`))
	for strings.Contains(p, `\\`) {
		p = strings.ReplaceAll(p, `\\`, `\`)
	}
	return strings.ToLower(p)
}

func waitForProcmgrCLIWindows(t *testing.T, host *components.RemoteHost, cli string) {
	t.Helper()
	cmd := psProcmgr(cli, "status")
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		_, err := host.Execute(cmd)
		assert.NoError(c, err, "dd-procmgr CLI not reachable")
	}, 2*time.Minute, 2*time.Second)
}

// WindowsDescribeDDOTCommandLine runs dd-procmgr describe and returns the Command field from text output.
func WindowsDescribeDDOTCommandLine(host *components.RemoteHost, ddProcmgrCLI string) (string, error) {
	out, err := host.Execute(psProcmgr(ddProcmgrCLI, "describe "+procmgrProcessName))
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

// AssertWindowsProcmgrDDOTMatchesInstalledBinary waits for dd-procmgr describe to show a stable
// Running DDOT process, then checks PID is set and the process image path matches the packaged otel-agent.exe.
func AssertWindowsProcmgrDDOTMatchesInstalledBinary(t *testing.T, host *components.RemoteHost) {
	t.Helper()

	installRoot, err := windowsagent.GetInstallPathFromRegistry(host)
	require.NoError(t, err)

	cli := filepath.Join(installRoot, "bin", "agent", "dd-procmgr.exe")
	waitForProcmgrCLIWindows(t, host, cli)

	describeCmd := psProcmgr(cli, "describe "+procmgrProcessName)
	waitProcmgrDDOTDescribeRunningStable(t, host, describeCmd)

	out, err := host.Execute(describeCmd)
	require.NoError(t, err, "dd-procmgr describe failed")

	pidStr := strings.TrimSpace(procmgrFieldValue(out, "PID"))
	require.NotEmpty(t, pidStr, "describe output should include PID:\n%s", out)
	require.NotEqual(t, "-", pidStr, "PID should be assigned when Running:\n%s", out)

	pid, err := strconv.ParseUint(pidStr, 10, 32)
	require.NoError(t, err, "PID should be numeric, got %q", pidStr)
	require.NotZero(t, pid, "PID should be non-zero")

	cmdPath := strings.TrimSpace(procmgrFieldValue(out, "Command"))
	require.NotEmpty(t, cmdPath, "describe output should include Command:\n%s", out)
	require.True(t, strings.EqualFold(filepath.Base(cmdPath), "otel-agent.exe"),
		"Command should point to otel-agent.exe, got %q", cmdPath)
	const wantSuffix = `\ext\ddot\embedded\bin\otel-agent.exe`
	require.True(t, strings.HasSuffix(normalizeWinPathForCompare(cmdPath), strings.ToLower(wantSuffix)),
		"Command %q should end with %s (fleet package or product install layout)", cmdPath, wantSuffix)

	wmiOut, err := host.Execute(fmt.Sprintf(
		`powershell -NoProfile -Command "(Get-CimInstance Win32_Process -Filter 'ProcessId=%d').ExecutablePath | Write-Output"`, pid))
	require.NoError(t, err, "WMI query for pid %d", pid)
	wmiPath := strings.TrimSpace(strings.ReplaceAll(wmiOut, "\r", ""))
	if strings.Contains(wmiPath, "\n") {
		for _, line := range strings.Split(wmiPath, "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "<") {
				continue
			}
			if strings.Contains(line, `:\`) {
				wmiPath = line
			}
		}
	}
	require.NotEmpty(t, wmiPath, "WMI should return ExecutablePath for pid %d", pid)
	require.Equal(t, normalizeWinPathForCompare(cmdPath), normalizeWinPathForCompare(wmiPath),
		"WMI image for pid %d should match describe Command (cmd=%q wmi=%q)", pid, cmdPath, wmiPath)
}
