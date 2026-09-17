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

// TestGetSIDForPID_CurrentProcess exercises GetSIDForPID against the real running test binary, since this package doesn't mock Windows syscalls.
func TestGetSIDForPID_CurrentProcess(t *testing.T) {
	sid, err := GetSIDForPID(int32(os.Getpid()))
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(sid, "S-1-"), "expected a well-formed Windows SID, got %q", sid)
}

// TestGetSIDForPID_NonexistentPID asserts GetSIDForPID errors on a PID guaranteed gone (only that it errors, not the exact code, which varies by Windows version).
func TestGetSIDForPID_NonexistentPID(t *testing.T) {
	cmd := exec.Command("cmd.exe", "/c", "exit", "0")
	require.NoError(t, cmd.Start())
	pid := int32(cmd.Process.Pid)
	require.NoError(t, cmd.Wait())

	_, err := GetSIDForPID(pid)
	require.Error(t, err)
}
