// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package procmgrtest provides new-e2e helpers for asserting processes managed by
// dd-procmgr (describe output, /proc/<pid>/exe, optional expected binary paths).
package procmgrtest

import (
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const (
	ddotProcmgrYAMLFileName = "datadog-agent-ddot.yaml"
	// StableDDOTProcmgrYAMLDeb is the stable processes.d DDOT file on classic deb/rpm installs.
	StableDDOTProcmgrYAMLDeb = "/opt/datadog-agent/processes.d/" + ddotProcmgrYAMLFileName
	// StableDDOTProcmgrYAMLOCI is the stable processes.d DDOT file on fleet OCI agent installs.
	StableDDOTProcmgrYAMLOCI = "/opt/datadog-packages/datadog-agent/stable/processes.d/" + ddotProcmgrYAMLFileName
)

const (
	DDOTProcessName                         = "datadog-agent-ddot"
	CLIBinDefault                           = "/opt/datadog-agent/embedded/bin/dd-procmgr"
	CLIBinFleetStable                       = "/opt/datadog-packages/datadog-agent/stable/embedded/bin/dd-procmgr"
	waitForProcessTimeout                   = 90 * time.Second
	waitForProcessPollInterval              = 2 * time.Second
	waitForProcessRunningStableWindow       = 5 * time.Second
	waitForProcessRunningStablePoll         = 500 * time.Millisecond
	ProcessStateRunning                     = "Running"
	DDOTOtelAgentFleetPackageBinary         = "/opt/datadog-packages/datadog-agent-ddot/stable/embedded/bin/otel-agent"
	DDOTOtelAgentFleetStableExtensionBinary = "/opt/datadog-packages/datadog-agent/stable/ext/ddot/embedded/bin/otel-agent"
	DDOTOtelAgentExtensionBinary            = "/opt/datadog-agent/ext/ddot/embedded/bin/otel-agent"
)

// CLIBinForLinuxHost returns CLIBinFleetStable when that binary is executable on the host,
// otherwise CLIBinDefault (classic deb/rpm layout). Staging install-script suites often use
// /opt/datadog-agent only until an OCI experiment is promoted.
func CLIBinForLinuxHost(t *testing.T, executor CommandExecutor) string {
	t.Helper()
	cmd := fmt.Sprintf(`if sudo test -x %q; then echo %q; elif sudo test -x %q; then echo %q; else echo ""; fi`,
		CLIBinFleetStable, CLIBinFleetStable, CLIBinDefault, CLIBinDefault)
	out, err := executor.ExecuteCommand(cmd)
	require.NoError(t, err)
	path := strings.TrimSpace(out)
	require.NotEmpty(t, path, "dd-procmgr CLI not found (checked %s and %s)", CLIBinFleetStable, CLIBinDefault)
	return path
}

// StableDDOTProcmgrYAMLPath returns the on-disk path to stable DDOT procmgr YAML, preferring
// the fleet OCI layout when present, otherwise the classic deb/rpm path.
func StableDDOTProcmgrYAMLPath(t *testing.T, executor CommandExecutor) string {
	t.Helper()
	cmd := fmt.Sprintf(`if sudo test -f %q; then echo %q; elif sudo test -f %q; then echo %q; else echo ""; fi`,
		StableDDOTProcmgrYAMLOCI, StableDDOTProcmgrYAMLOCI, StableDDOTProcmgrYAMLDeb, StableDDOTProcmgrYAMLDeb)
	out, err := executor.ExecuteCommand(cmd)
	require.NoError(t, err)
	path := strings.TrimSpace(out)
	require.NotEmpty(t, path, "datadog-agent-ddot.yaml not found (checked %s and %s)", StableDDOTProcmgrYAMLOCI, StableDDOTProcmgrYAMLDeb)
	return path
}

// CommandExecutor executes a command on the remote host.
type CommandExecutor interface {
	ExecuteCommand(command string) (string, error)
}

type WaitForProcessArgs struct {
	ProcmgrCLIBin  string
	ProcessName    string
	ExpectedBinary string
	DesiredState   string
}

type WaitForProcessResult struct {
	PID      string
	Restarts int
}

// WaitForProcess polls dd-procmgr describe until State matches DesiredState and returns
// a snapshot result. Restarts is always populated; PID is only populated for
// ProcessStateRunning. For ProcessStateRunning it also validates Command,
// readlink -f on Command vs /proc/<pid>/exe, and optional ExpectedBinary resolution.
// When DesiredState is Running, it then polls for waitForProcessRunningStableWindow
// to assert State stays Running and PID does not change.
func WaitForProcess(t *testing.T, executor CommandExecutor, args WaitForProcessArgs) WaitForProcessResult {
	t.Helper()
	require.NotEmpty(t, args.ProcmgrCLIBin, "WaitForProcessArgs.ProcmgrCLIBin must be set")
	require.NotEmpty(t, args.DesiredState, "WaitForProcessArgs.DesiredState must be set")

	describeCmd := fmt.Sprintf(`sudo -u dd-agent -- %q describe %q`, args.ProcmgrCLIBin, args.ProcessName)
	desiredState := args.DesiredState

	wantExeFromHint, hintResolves := resolveExpectedBinaryHint(executor, desiredState, args.ExpectedBinary)

	var result WaitForProcessResult
	require.Eventually(t, func() bool {
		out, err := executor.ExecuteCommand(describeCmd)
		if err != nil {
			t.Logf("WaitForProcess: dd-procmgr describe cmd=%q err=%v\noutput:\n%s", describeCmd, err, out)
			return false
		}
		if st := fieldValue(out, "State"); st != desiredState {
			t.Logf("WaitForProcess: dd-procmgr describe cmd=%q State=%q (want %s)\noutput:\n%s", describeCmd, st, desiredState, out)
			return false
		}
		if args.ExpectedBinary != "" {
			if cmd := fieldValue(out, "Command"); cmd != args.ExpectedBinary {
				t.Logf("WaitForProcess: dd-procmgr describe cmd=%q unexpected Command field got=%q want=%q\noutput:\n%s", describeCmd, cmd, args.ExpectedBinary, out)
				return false
			}
		}
		if desiredState != ProcessStateRunning {
			result = WaitForProcessResult{
				Restarts: restartsFromDescribe(out),
			}
			return true
		}

		r, ok := resolveRunningPIDFromDescribe(t, executor, describeCmd, out, hintResolves, wantExeFromHint, args.ExpectedBinary)
		if !ok {
			return false
		}
		result = r
		return true
	}, waitForProcessTimeout, waitForProcessPollInterval, fmt.Sprintf("process %q should be %s via dd-procmgr describe", args.ProcessName, desiredState))
	if desiredState == ProcessStateRunning && result.PID != "" {
		requireStableRunningPID(t, executor, describeCmd, desiredState, result.PID)
	}
	return result
}

// requireStableRunningPID polls describe for stableWindow and fails if State or PID drifts.
func requireStableRunningPID(t *testing.T, executor CommandExecutor, describeCmd, wantState, wantPID string) {
	t.Helper()
	start := time.Now()
	for {
		out, err := executor.ExecuteCommand(describeCmd)
		require.NoError(t, err, "dd-procmgr describe during stable window: %s", describeCmd)
		st := fieldValue(out, "State")
		require.Equal(t, wantState, st, "State changed during stable window (elapsed %s)\ndescribe:\n%s", time.Since(start), out)
		pid := fieldValue(out, "PID")
		require.Equal(t, wantPID, pid, "PID changed during stable window (elapsed %s)\ndescribe:\n%s", time.Since(start), out)
		if time.Since(start) >= waitForProcessRunningStableWindow {
			return
		}
		time.Sleep(waitForProcessRunningStablePoll)
	}
}

func resolveExpectedBinaryHint(
	executor CommandExecutor,
	desiredState, expectedBinary string,
) (string, bool) {
	if desiredState != ProcessStateRunning || expectedBinary == "" {
		return "", false
	}
	out, err := executor.ExecuteCommand(fmt.Sprintf("sudo readlink -f %q", expectedBinary))
	if err != nil {
		return "", false
	}
	trimmedOut := strings.TrimSpace(out)
	if trimmedOut == "" {
		return "", false
	}
	return trimmedOut, true
}

func resolveRunningPIDFromDescribe(
	t *testing.T,
	executor CommandExecutor,
	describeCmd, describeOut string,
	hintResolves bool,
	wantExeFromHint, expectedBinaryForLog string,
) (WaitForProcessResult, bool) {
	t.Helper()
	cmd, cmdExe, ok := resolveCommandExeFromDescribe(t, executor, describeCmd, describeOut, hintResolves, wantExeFromHint, expectedBinaryForLog)
	if !ok {
		return WaitForProcessResult{}, false
	}
	pid, ok := resolveRunningPIDFromProc(t, executor, describeCmd, describeOut, cmd, cmdExe)
	if !ok {
		return WaitForProcessResult{}, false
	}
	return WaitForProcessResult{
		PID:      pid,
		Restarts: restartsFromDescribe(describeOut),
	}, true
}

func resolveCommandExeFromDescribe(
	t *testing.T,
	executor CommandExecutor,
	describeCmd, describeOut string,
	hintResolves bool,
	wantExeFromHint, expectedBinaryForLog string,
) (string, string, bool) {
	t.Helper()
	cmd := fieldValue(describeOut, "Command")
	cmdExe, err := executor.ExecuteCommand(fmt.Sprintf("sudo readlink -f %q", cmd))
	if err != nil {
		t.Logf("resolveCommandExeFromDescribe: readlink -f %q (Command field from prior dd-procmgr describe) err=%v\n%s", cmd, err, cmdExe)
		return "", "", false
	}
	cmdExe = strings.TrimSpace(cmdExe)
	if cmdExe == "" {
		t.Logf("resolveCommandExeFromDescribe: readlink -f %q returned empty path (Command field from prior dd-procmgr describe)", cmd)
		return "", "", false
	}
	if hintResolves && cmdExe != wantExeFromHint {
		t.Logf("resolveCommandExeFromDescribe: dd-procmgr describe cmd=%q resolved exe got=%q want=%q (hint from ExpectedBinary %q)\noutput:\n%s", describeCmd, cmdExe, wantExeFromHint, expectedBinaryForLog, describeOut)
		return "", "", false
	}
	return cmd, cmdExe, true
}

func resolveRunningPIDFromProc(
	t *testing.T,
	executor CommandExecutor,
	describeCmd, describeOut, cmd, cmdExe string,
) (string, bool) {
	t.Helper()
	pid := fieldValue(describeOut, "PID")
	if pid == "" || pid == "-" {
		t.Logf("resolveRunningPIDFromProc: dd-procmgr describe cmd=%q missing PID (got %q)\noutput:\n%s", describeCmd, pid, describeOut)
		return "", false
	}
	exeOut, err := executor.ExecuteCommand("sudo readlink -f /proc/" + pid + "/exe")
	if err != nil {
		t.Logf("resolveRunningPIDFromProc: readlink -f /proc/%s/exe err=%v\n%s", pid, err, exeOut)
		return "", false
	}
	if strings.TrimSpace(exeOut) != cmdExe {
		t.Logf("resolveRunningPIDFromProc: readlink -f /proc/%s/exe got=%q want=%q (canonical Command %q from dd-procmgr describe)\ndd-procmgr describe output:\n%s", pid, strings.TrimSpace(exeOut), cmdExe, cmd, describeOut)
		return "", false
	}
	return pid, true
}

func restartsFromDescribe(describeOut string) int {
	n, err := strconv.Atoi(fieldValue(describeOut, "Restarts"))
	if err != nil {
		return 0
	}
	return n
}

func fieldValue(output, label string) string {
	needle := label + ":"
	for _, line := range strings.Split(output, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, needle) {
			return strings.TrimSpace(trimmed[len(needle):])
		}
	}
	return ""
}
