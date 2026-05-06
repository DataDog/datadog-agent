// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package procmgrtest holds shared E2E helpers for asserting a process is running
// under dd-procmgr (describe + /proc/<pid>/exe checks).
package procmgrtest

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// CLIBin is the standard dd-procmgr CLI path on Unix test hosts.
const CLIBin = "/opt/datadog-agent/embedded/bin/dd-procmgr"

// DDOTProcessName is the dd-procmgr process name for the DDOT collector.
const DDOTProcessName = "datadog-agent-ddot"

// CommandExecutor executes a command on the remote host.
type CommandExecutor interface {
	ExecuteCommand(command string) (string, error)
}

// WaitForRunningProcess polls `dd-procmgr describe <processName>` until describe reports State=Running,
// Command matches expectedBinary, and /proc/<pid>/exe resolves to the same path
// as sudo readlink -f expectedBinary (so stable paths match versioned binaries).
func WaitForRunningProcess(t *testing.T, executor CommandExecutor, processName, expectedBinary string, timeout time.Duration) string {
	t.Helper()
	describeCmd := fmt.Sprintf(`sudo -u dd-agent -- %q describe %q`, CLIBin, processName)
	wantExe, err := executor.ExecuteCommand(fmt.Sprintf("sudo readlink -f %q", expectedBinary))
	require.NoError(t, err, "resolve expected binary %q", expectedBinary)
	wantExe = strings.TrimSpace(wantExe)
	require.NotEmpty(t, wantExe, "resolved expected binary for %q", expectedBinary)

	var pid string
	require.Eventually(t, func() bool {
		out, err := executor.ExecuteCommand(describeCmd)
		if err != nil {
			t.Logf("WaitForRunningProcess: describe command=%q err=%v\noutput:\n%s", describeCmd, err, out)
			return false
		}
		st := fieldValue(out, "State")
		if st != "Running" {
			t.Logf("WaitForRunningProcess: describe command=%q State=%q (want Running)\noutput:\n%s", describeCmd, st, out)
			return false
		}
		if cmd := fieldValue(out, "Command"); cmd != expectedBinary {
			t.Logf("WaitForRunningProcess: describe command=%q unexpected Command got=%q want=%q\noutput:\n%s", describeCmd, cmd, expectedBinary, out)
			return false
		}
		p := fieldValue(out, "PID")
		if p == "" || p == "-" {
			t.Logf("WaitForRunningProcess: describe command=%q missing PID (got %q)\noutput:\n%s", describeCmd, p, out)
			return false
		}
		exeOut, err := executor.ExecuteCommand("sudo readlink -f /proc/" + p + "/exe")
		if err != nil {
			t.Logf("WaitForRunningProcess: readlink /proc/%s/exe failed: %v\n%s", p, err, exeOut)
			return false
		}
		if strings.TrimSpace(exeOut) != wantExe {
			t.Logf("WaitForRunningProcess: unexpected executable got=%q want=%q (resolved from %q, pid=%s)\ndescribe output:\n%s", strings.TrimSpace(exeOut), wantExe, expectedBinary, p, out)
			return false
		}
		pid = p
		return true
	}, timeout, 2*time.Second, fmt.Sprintf("process %q should be running via dd-procmgr describe", processName))
	return pid
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
