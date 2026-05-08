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
	DDOTProcessName                         = "datadog-agent-ddot"
	CLIBinDefault                           = "/opt/datadog-agent/embedded/bin/dd-procmgr"
	CLIBinFleetStable                       = "/opt/datadog-packages/datadog-agent/stable/embedded/bin/dd-procmgr"
	waitForProcessTimeout                   = 90 * time.Second
	waitForProcessPollInterval              = 2 * time.Second
	ProcessStateRunning                     = "Running"
	DDOTOtelAgentFleetPackageBinary         = "/opt/datadog-packages/datadog-agent-ddot/stable/embedded/bin/otel-agent"
	DDOTOtelAgentFleetStableExtensionBinary = "/opt/datadog-packages/datadog-agent/stable/ext/ddot/embedded/bin/otel-agent"
	DDOTOtelAgentExtensionBinary            = "/opt/datadog-agent/ext/ddot/embedded/bin/otel-agent"
)

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
// PID/Restarts from that snapshot. For ProcessStateRunning it also validates Command,
// readlink -f on Command vs /proc/<pid>/exe, and optional ExpectedBinary resolution.
func WaitForProcess(t *testing.T, executor CommandExecutor, args WaitForProcessArgs) WaitForProcessResult {
	t.Helper()
	require.NotEmpty(t, args.ProcmgrCLIBin, "WaitForProcessArgs.ProcmgrCLIBin must be set")
	require.NotEmpty(t, args.DesiredState, "WaitForProcessArgs.DesiredState must be set")

	describeCmd := fmt.Sprintf(`sudo -u dd-agent -- %q describe %q`, args.ProcmgrCLIBin, args.ProcessName)
	desiredState := args.DesiredState

	var wantExeFromHint string
	hintResolves := false
	if desiredState == ProcessStateRunning && args.ExpectedBinary != "" {
		out, err := executor.ExecuteCommand(fmt.Sprintf("sudo readlink -f %q", args.ExpectedBinary))
		hintResolves = err == nil && strings.TrimSpace(out) != ""
		if hintResolves {
			wantExeFromHint = strings.TrimSpace(out)
		}
	}

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

		r, ok := runningSnapshotFromDescribe(t, executor, describeCmd, out, hintResolves, wantExeFromHint, args.ExpectedBinary)
		if !ok {
			return false
		}
		result = r
		return true
	}, waitForProcessTimeout, waitForProcessPollInterval, fmt.Sprintf("process %q should be %s via dd-procmgr describe", args.ProcessName, desiredState))
	return result
}

func runningSnapshotFromDescribe(
	t *testing.T,
	executor CommandExecutor,
	describeCmd, describeOut string,
	hintResolves bool,
	wantExeFromHint, expectedBinaryForLog string,
) (WaitForProcessResult, bool) {
	t.Helper()
	cmd := fieldValue(describeOut, "Command")
	cmdExe, err := executor.ExecuteCommand(fmt.Sprintf("sudo readlink -f %q", cmd))
	if err != nil {
		t.Logf("runningSnapshotFromDescribe: readlink -f %q (Command field from prior dd-procmgr describe) err=%v\n%s", cmd, err, cmdExe)
		return WaitForProcessResult{}, false
	}
	cmdExe = strings.TrimSpace(cmdExe)
	if cmdExe == "" {
		t.Logf("runningSnapshotFromDescribe: readlink -f %q returned empty path (Command field from prior dd-procmgr describe)", cmd)
		return WaitForProcessResult{}, false
	}
	if hintResolves && cmdExe != wantExeFromHint {
		t.Logf("runningSnapshotFromDescribe: dd-procmgr describe cmd=%q resolved exe got=%q want=%q (hint from ExpectedBinary %q)\noutput:\n%s", describeCmd, cmdExe, wantExeFromHint, expectedBinaryForLog, describeOut)
		return WaitForProcessResult{}, false
	}
	pid := fieldValue(describeOut, "PID")
	if pid == "" || pid == "-" {
		t.Logf("runningSnapshotFromDescribe: dd-procmgr describe cmd=%q missing PID (got %q)\noutput:\n%s", describeCmd, pid, describeOut)
		return WaitForProcessResult{}, false
	}
	exeOut, err := executor.ExecuteCommand("sudo readlink -f /proc/" + pid + "/exe")
	if err != nil {
		t.Logf("runningSnapshotFromDescribe: readlink -f /proc/%s/exe err=%v\n%s", pid, err, exeOut)
		return WaitForProcessResult{}, false
	}
	if strings.TrimSpace(exeOut) != cmdExe {
		t.Logf("runningSnapshotFromDescribe: readlink -f /proc/%s/exe got=%q want=%q (canonical Command %q from dd-procmgr describe)\ndd-procmgr describe output:\n%s", pid, strings.TrimSpace(exeOut), cmdExe, cmd, describeOut)
		return WaitForProcessResult{}, false
	}
	return WaitForProcessResult{
		PID:      pid,
		Restarts: restartsFromDescribe(describeOut),
	}, true
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
