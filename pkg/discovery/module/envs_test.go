// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

package module

import (
	"context"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/shirou/gopsutil/v4/process"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/discovery/envs"
)

// TestTargetEnvs it checks reading of target environment variables only from /proc/<pid>/environ.
func TestTargetEnvs(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(func() { cancel() })

	cmd := exec.CommandContext(ctx, "sleep", "1000")
	expectedEnvs := envs.GetExpectedEnvs()
	cmd.Env = append(cmd.Env, expectedEnvs...)
	err := cmd.Start()
	require.NoError(t, err)

	var proc *process.Process
	require.EventuallyWithT(t, func(collect *assert.CollectT) {
		proc, err = process.NewProcess(int32(cmd.Process.Pid))
		require.NoError(collect, err)

		// Check that envs are visible using the gopsutil library before we
		// attempt to read them using our own function under test to prevent
		// spurious failures, since the environ file can be empty if the process
		// is not fully execve(2)'d yet. See environ_read() in fs/proc/base.c in
		// the kernel.
		procEnv, err := proc.Environ()
		require.NoError(collect, err)
		// NotEmpty doesn't work because proc.Environ() returns a slice with one
		// empty string as the first element if environ is empty.
		assert.Greater(collect, len(procEnv), 1)
	}, 5*time.Second, 10*time.Millisecond)

	vars, err := GetTargetEnvs(proc)
	require.NoError(t, err)

	expectedMap := envs.GetExpectedMap()
	for k, v := range expectedMap {
		val, ok := vars.Get(k)
		require.True(t, ok)
		require.Equal(t, val, v)
	}

	// check unexpected env variables
	val, ok := vars.Get("HOME")
	require.Empty(t, val)
	require.False(t, ok)
	val, ok = vars.Get("PATH")
	require.Empty(t, val)
	require.False(t, ok)
	val, ok = vars.Get("SHELL")
	require.Empty(t, val)
	require.False(t, ok)

	// check that non-target variables return an empty map.
	vars = envs.NewVariables(map[string]string{
		"NON_TARGET1": "some",
		"NON_TARGET2": "some",
	})
	val, ok = vars.Get("NON_TARGET1")
	require.Empty(t, val)
	require.False(t, ok)
	val, ok = vars.Get("NON_TARGET2")
	require.Empty(t, val)
	require.False(t, ok)
}

// BenchmarkGetEnvs benchmarks reading of all environment variables from /proc/<pid>/environ.
func BenchmarkGetEnvs(b *testing.B) {
	proc := &process.Process{Pid: int32(os.Getpid())}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := getEnvs(proc); err != nil {
			return
		}
	}
}

// BenchmarkGetEnvsTarget benchmarks reading of target environment variables only from /proc/<pid>/environ.
func BenchmarkGetEnvsTarget(b *testing.B) {
	proc := &process.Process{Pid: int32(os.Getpid())}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := GetTargetEnvs(proc); err != nil {
			return
		}
	}
}
