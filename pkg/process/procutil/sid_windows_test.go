// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package procutil

import (
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGetSIDForPID_CurrentProcess exercises GetSIDForPID against a real, currently
// running process (this test binary itself). This package has no precedent for
// mocking Windows syscalls (OpenProcessHandle/GetUsernameForProcess are untested
// today for the same reason), so - matching TestWindowsProbe in
// process_windows_test.go - this asserts against the real OS instead.
func TestGetSIDForPID_CurrentProcess(t *testing.T) {
	sid, err := GetSIDForPID(int32(os.Getpid()))
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(sid, "S-1-"), "expected a well-formed Windows SID, got %q", sid)
}

// TestGetSIDForPID_NonexistentPID spawns and waits for a trivial child process to
// obtain a PID that is guaranteed to no longer be running, then asserts
// GetSIDForPID surfaces an error for it. The exact Win32 error code for a
// reused/nonexistent PID can vary slightly by Windows version, so this only
// asserts that an error is returned, not its classification (that classification
// is exercised against a stub in cmd/process-agent/api/pid_windows_test.go).
func TestGetSIDForPID_NonexistentPID(t *testing.T) {
	cmd := exec.Command("cmd.exe", "/c", "exit", "0")
	require.NoError(t, cmd.Start())
	pid := int32(cmd.Process.Pid)
	require.NoError(t, cmd.Wait())

	_, err := GetSIDForPID(pid)
	require.Error(t, err)
}
