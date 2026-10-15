// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package authoredscripts

import (
	"context"
	"os/exec"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestExecuteCommandWindowsOutputLimit(t *testing.T) {
	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", `while ($true) { [Console]::Out.Write("xxxxxxxxxxxxxxxx") }`)

	result, err := executeCommand(context.Background(), cmd, 100)

	require.ErrorIs(t, err, errOutputLimitExceeded)
	assert.LessOrEqual(t, len(result.Stdout)+len(result.Stderr), 100)
}
