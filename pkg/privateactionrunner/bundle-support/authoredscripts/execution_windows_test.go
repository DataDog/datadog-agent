// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package authoredscripts

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows"
)

func TestExecuteCommandWindowsSuccess(t *testing.T) {
	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", `[Console]::Out.Write("hello"); [Console]::Error.Write("world")`)

	result, err := ExecuteCommand(context.Background(), cmd)

	require.NoError(t, err)
	assert.Equal(t, 0, result.ExitCode)
	assert.Equal(t, "hello", result.Stdout)
	assert.Equal(t, "world", result.Stderr)
}

func TestExecuteCommandWindowsNonZeroExit(t *testing.T) {
	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", `[Console]::Error.Write("boom"); exit 3`)

	result, err := ExecuteCommand(context.Background(), cmd)

	require.Error(t, err)
	assert.Equal(t, 3, result.ExitCode)
	assert.ErrorContains(t, err, "exit code 3")
	assert.ErrorContains(t, err, "boom")
}

func TestExecuteCommandWindowsContextTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", "Start-Sleep -Seconds 30")

	_, err := ExecuteCommand(ctx, cmd)

	require.ErrorIs(t, err, context.DeadlineExceeded)
	assert.ErrorContains(t, err, "timed out")
}

func TestExecuteCommandWindowsContextCancellationTerminatesDescendants(t *testing.T) {
	const testTimeout = 30 * time.Second

	readyEventName := fmt.Sprintf(`Local\DatadogAuthoredScriptsTest-%d-%d`, os.Getpid(), time.Now().UnixNano())
	readyEvent, err := windows.CreateEvent(nil, 1, 0, windows.StringToUTF16Ptr(readyEventName))
	require.NoError(t, err)
	defer windows.CloseHandle(readyEvent)

	testCtx, cancelTest := context.WithTimeout(context.Background(), testTimeout)
	defer cancelTest()
	executionCtx, cancelExecution := context.WithCancel(context.Background())
	defer cancelExecution()
	cmd := exec.Command(
		"powershell.exe",
		"-NoProfile",
		"-NonInteractive",
		"-Command",
		fmt.Sprintf(`$child = Start-Process powershell.exe -ArgumentList '-NoProfile','-NonInteractive','-Command','Start-Sleep -Seconds 30' -PassThru; [Console]::Out.Write($child.Id); $ready = [System.Threading.EventWaitHandle]::OpenExisting('%s'); $ready.Set() | Out-Null; Start-Sleep -Seconds 30`, readyEventName),
	)

	type commandOutcome struct {
		result Result
		err    error
	}
	outcomeCh := make(chan commandOutcome, 1)
	go func() {
		result, err := ExecuteCommand(executionCtx, cmd)
		outcomeCh <- commandOutcome{result: result, err: err}
	}()

	status, waitErr := windows.WaitForSingleObject(readyEvent, uint32(testTimeout/time.Millisecond))
	require.NoError(t, waitErr)
	require.Equal(t, uint32(windows.WAIT_OBJECT_0), status, "child process did not become ready before the test timeout")
	cancelExecution()

	var outcome commandOutcome
	select {
	case outcome = <-outcomeCh:
	case <-testCtx.Done():
		t.Fatal("command did not stop after its execution context was canceled")
	}

	require.ErrorIs(t, outcome.err, context.Canceled)
	childPID, parseErr := strconv.ParseUint(strings.TrimSpace(outcome.result.Stdout), 10, 32)
	require.NoError(t, parseErr, "child PID was not captured before cancellation: %q", outcome.result.Stdout)
	process, openErr := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(childPID))
	if openErr != nil {
		require.ErrorIs(t, openErr, windows.ERROR_INVALID_PARAMETER)
		return
	}
	defer windows.CloseHandle(process)
	status, waitErr := windows.WaitForSingleObject(process, 5_000)
	require.NoError(t, waitErr)
	assert.Equal(t, uint32(windows.WAIT_OBJECT_0), status, "descendant process was not terminated with its Windows job")
}

func TestExecuteCommandWindowsOutputLimit(t *testing.T) {
	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", `while ($true) { [Console]::Out.Write("xxxxxxxxxxxxxxxx") }`)

	result, err := executeCommand(context.Background(), cmd, 100)

	require.ErrorIs(t, err, errOutputLimitExceeded)
	assert.LessOrEqual(t, len(result.Stdout)+len(result.Stderr), 100)
}
