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
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestProcessesByPIDReusedPID exercises the PID reuse path in ProcessesByPID by
// pre-populating the probe's cache with a wrong createTime for a real process.
// When ProcessesByPID runs, it sees the same PID with a different createTime and
// replaces the cached process (reuse branch).
func TestProcessesByPIDReusedPID(t *testing.T) {
	now := time.Now()
	cmd := exec.Command("powershell.exe", "-c", "sleep 60; foo bar baz")
	err := cmd.Start()
	require.NoError(t, err)
	defer cmd.Process.Kill()

	pid := uint32(cmd.Process.Pid)

	probe := NewWindowsToolhelpProbe()
	wp, ok := probe.(*windowsToolhelpProbe)
	require.True(t, ok, "probe must be *windowsToolhelpProbe")

	// Pre-populate cache with wrong createTime to run the reused PID logic.
	// createTime 1 will never equal the real process's createTime.
	const wrongCreateTime = 1
	wp.cachedProcesses[pid] = &cachedProcess{createTime: wrongCreateTime}

	procs, err := probe.ProcessesByPID(time.Now(), true)
	require.NoError(t, err)

	p, found := procs[int32(pid)]
	require.True(t, found, "process with reused PID should be in result")

	// Reuse path should have replaced the cache and filled with real createTime.
	assert.NotEqual(t, wrongCreateTime, p.Stats.CreateTime,
		"ProcessesByPID should have replaced cached process with real createTime (PID reuse path)")

	// Sanity: process metadata should match the process we started.
	assert.True(t, strings.HasSuffix(p.Exe, "powershell.exe"), "Exe should be powershell.exe")
	assert.Equal(t, []string{"powershell.exe", "-c", `"sleep 60; foo bar baz"`}, p.Cmdline)
	assert.Equal(t, int32(os.Getpid()), p.Ppid)
	assert.Equal(t, int32(pid), p.Pid)
	assert.Equal(t, "Windows PowerShell", p.Comm)

	// Real createTime should be in a plausible range (process started around now).
	assert.WithinRange(t, time.Unix(0, p.Stats.CreateTime*1000_000), now, now.Add(5*time.Second),
		"CreateTime should reflect actual process start time")
}
