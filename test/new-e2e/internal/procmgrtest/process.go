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
	DDOTProcessName = "datadog-agent-ddot"

	ddotProcmgrYAMLFileName = "datadog-agent-ddot.yaml"
	// StableDDOTProcmgrYAMLDeb is the stable processes.d DDOT file on classic deb/rpm installs.
	StableDDOTProcmgrYAMLDeb = "/opt/datadog-agent/processes.d/" + ddotProcmgrYAMLFileName
	// StableDDOTProcmgrYAMLOCI is the stable processes.d DDOT file on fleet OCI agent installs.
	StableDDOTProcmgrYAMLOCI = "/opt/datadog-packages/datadog-agent/stable/processes.d/" + ddotProcmgrYAMLFileName

	DDOTOtelAgentExtensionBinary            = "/opt/datadog-agent/ext/ddot/embedded/bin/otel-agent"
	DDOTOtelAgentFleetStableExtensionBinary = "/opt/datadog-packages/datadog-agent/stable/ext/ddot/embedded/bin/otel-agent"
	DDOTOtelAgentFleetPackageBinary         = "/opt/datadog-packages/datadog-agent-ddot/stable/embedded/bin/otel-agent"

	CLIBinDefault     = "/opt/datadog-agent/embedded/bin/dd-procmgr"
	CLIBinFleetStable = "/opt/datadog-packages/datadog-agent/stable/embedded/bin/dd-procmgr"

	ProcessStateRunning               = "Running"
	ProcessStateStarting              = "Starting"
	ProcessStateStopped               = "Stopped"
	waitForProcessTimeout             = 90 * time.Second
	waitForProcessNotRunningTimeout   = 30 * time.Second
	waitForProcessPollInterval        = 2 * time.Second
	waitForProcessRunningStableWindow = 5 * time.Second
	waitForProcessRunningStablePoll   = 500 * time.Millisecond
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
// ProcessStateRunning. For ProcessStateRunning, when ExpectedBinary is set, describe
// Command must equal it. The describe Command path is canonicalized with readlink -f and
// must match readlink -f /proc/<pid>/exe (PID from describe).
// For Running, success additionally requires State and PID to stay unchanged for
// waitForProcessRunningStableWindow inside one Eventually callback (otherwise the callback
// returns false and Eventually retries—e.g. a brief Running before a crash does not pass).
func WaitForProcess(t *testing.T, executor CommandExecutor, args WaitForProcessArgs) WaitForProcessResult {
	t.Helper()
	require.NotEmpty(t, args.ProcmgrCLIBin, "WaitForProcessArgs.ProcmgrCLIBin must be set")
	require.NotEmpty(t, args.DesiredState, "WaitForProcessArgs.DesiredState must be set")

	describeCmd := fmt.Sprintf(`sudo -u dd-agent -- %q describe %q`, args.ProcmgrCLIBin, args.ProcessName)
	desiredState := args.DesiredState

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
		descCmd := fieldValue(out, "Command")
		if args.ExpectedBinary != "" {
			if descCmd != args.ExpectedBinary {
				t.Logf("WaitForProcess: dd-procmgr describe cmd=%q unexpected Command field got=%q want=%q\noutput:\n%s", describeCmd, descCmd, args.ExpectedBinary, out)
				return false
			}
		}
		if desiredState != ProcessStateRunning {
			result = WaitForProcessResult{
				Restarts: restartsFromDescribe(out),
			}
			return true
		}

		r, ok := resolveRunningPIDFromDescribe(t, executor, describeCmd, out)
		if !ok {
			return false
		}
		if !confirmStableRunningPID(t, executor, describeCmd, desiredState, r.PID) {
			return false
		}
		result = r
		return true
	}, waitForProcessTimeout, waitForProcessPollInterval, fmt.Sprintf("process %q should be %s via dd-procmgr describe", args.ProcessName, desiredState))
	return result
}

// WaitForDDOTRunning polls until process datadog-agent-ddot is Running, using CLIBinForLinuxHost
// and validating describe Command against expectedBinary (e.g. DDOTOtelAgentExtensionBinary vs
// DDOTOtelAgentFleetPackageBinary for extension vs standalone ddot-package installs).
func WaitForDDOTRunning(t *testing.T, executor CommandExecutor, expectedBinary string) WaitForProcessResult {
	t.Helper()
	cli := CLIBinForLinuxHost(t, executor)
	require.NotEmpty(t, expectedBinary,
		"expectedBinary must be set (use DDOTOtelAgentExtensionBinary, DDOTOtelAgentFleetStableExtensionBinary, DDOTOtelAgentFleetPackageBinary)")
	return WaitForProcess(t, executor, WaitForProcessArgs{
		ProcmgrCLIBin:  cli,
		ProcessName:    DDOTProcessName,
		ExpectedBinary: expectedBinary,
		DesiredState:   ProcessStateRunning,
	})
}

// AssertDDOTNotRunning polls dd-procmgr until datadog-agent-ddot is not Running: describe
// returns process not found, or State is Stopped/Failed/etc. (not Running or Starting).
func AssertDDOTNotRunning(t *testing.T, executor CommandExecutor) {
	t.Helper()
	cli := CLIBinForLinuxHost(t, executor)
	describeCmd := fmt.Sprintf(`sudo -u dd-agent -- %q describe %q`, cli, DDOTProcessName)
	require.Eventually(t, func() bool {
		out, err := executor.ExecuteCommand(describeCmd)
		if ddotDescribeShowsNotRunning(out, err) {
			return true
		}
		t.Logf("AssertDDOTNotRunning: dd-procmgr describe cmd=%q err=%v\noutput:\n%s", describeCmd, err, out)
		return false
	}, waitForProcessNotRunningTimeout, waitForProcessPollInterval,
		fmt.Sprintf("process %q must not be Running via dd-procmgr describe", DDOTProcessName))
}

func ddotDescribeShowsNotRunning(out string, err error) bool {
	msg := out
	if err != nil {
		msg += " " + err.Error()
	}
	if strings.Contains(msg, "not found") {
		return true
	}
	st := fieldValue(out, "State")
	if st == "" {
		return false
	}
	return st != ProcessStateRunning && st != ProcessStateStarting
}

// confirmStableRunningPID returns true iff describe shows wantState and wantPID on every
// poll for waitForProcessRunningStableWindow. Any drift returns false so the caller can
// retry (e.g. DDOT may briefly report Running before a failed restart settles on a stable PID).
func confirmStableRunningPID(t *testing.T, executor CommandExecutor, describeCmd, wantState, wantPID string) bool {
	t.Helper()
	start := time.Now()
	for time.Since(start) < waitForProcessRunningStableWindow {
		out, err := executor.ExecuteCommand(describeCmd)
		if err != nil {
			t.Logf("confirmStableRunningPID: describe err=%v\n%s", err, out)
			return false
		}
		if st := fieldValue(out, "State"); st != wantState {
			t.Logf("confirmStableRunningPID: State=%q want %q", st, wantState)
			return false
		}
		if pid := fieldValue(out, "PID"); pid != wantPID {
			t.Logf("confirmStableRunningPID: PID=%q want %q", pid, wantPID)
			return false
		}
		time.Sleep(waitForProcessRunningStablePoll)
	}
	return true
}

func resolveRunningPIDFromDescribe(
	t *testing.T,
	executor CommandExecutor,
	describeCmd, describeOut string,
) (WaitForProcessResult, bool) {
	t.Helper()
	cmd, cmdExe, ok := resolveCommandExeFromDescribe(t, executor, describeCmd, describeOut)
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
) (string, string, bool) {
	t.Helper()
	cmd := fieldValue(describeOut, "Command")
	cmdExe, err := executor.ExecuteCommand(fmt.Sprintf("sudo readlink -f %q", cmd))
	if err != nil {
		t.Logf("resolveCommandExeFromDescribe: describe cmd=%q readlink -f %q (Command from describe) err=%v\n%s", describeCmd, cmd, err, cmdExe)
		return "", "", false
	}
	cmdExe = strings.TrimSpace(cmdExe)
	if cmdExe == "" {
		t.Logf("resolveCommandExeFromDescribe: describe cmd=%q readlink -f %q returned empty path (Command from describe)", describeCmd, cmd)
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
